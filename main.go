package main

import (
	"hermes/order"
	"hermes/pair"
	"hermes/utils"

	"context"
	"flag"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"time"

	"github.com/adshao/go-binance/v2"
	"github.com/adshao/go-binance/v2/futures"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/rs/zerolog"
)

const LIMIT int = 200

var log zerolog.Logger = utils.InitLogging()

var alertOnSignals *bool
var bot *tgbotapi.BotAPI
var futuresClient *futures.Client
var interval string
var sentAlerts = make(map[string]string) // {"BTCUSDT": "bullish|bearish", ...}
var symbolAssets = make(map[string]pair.Asset)
var symbolCloses = make(map[string][]float64) // {"BTCUSDT": [40004.75, ...], ...}
var tradeSignals *bool

func fetchAssets(futuresClient *futures.Client, wg *sync.WaitGroup) map[string]string {
	symbolIntervalPair := make(map[string]string)

	exchangeInfo, err := futuresClient.NewExchangeInfoService().Do(context.Background())
	if err != nil {
		log.Fatal().Str("err", err.Error()).Msg("Crashed getting exchange info")
	}

	// Filter unwanted symbols (non-USDT, quarterlies, indexes, unactive, and 1000BTTC)
	for _, rawAsset := range exchangeInfo.Symbols {
		if rawAsset.QuoteAsset == "USDT" && rawAsset.ContractType == "PERPETUAL" && rawAsset.UnderlyingType == "COIN" &&
			rawAsset.Status == "TRADING" && rawAsset.BaseAsset != "1000BTTC" {

			symbol := rawAsset.Symbol
			maxQuantity, _ := strconv.ParseFloat(rawAsset.LotSizeFilter().MaxQuantity, 64)
			minQuantity, _ := strconv.ParseFloat(rawAsset.LotSizeFilter().MinQuantity, 64)

			symbolAssets[symbol] = pair.Asset{
				BaseAsset:         rawAsset.BaseAsset,
				MaxQuantity:       maxQuantity,
				MinQuantity:       minQuantity,
				PricePrecision:    rawAsset.PricePrecision,
				QuantityPrecision: rawAsset.QuantityPrecision,
				Symbol:            symbol,
			}

			symbolIntervalPair[symbol] = interval

			wg.Add(1)

			defer wg.Done()
			go fetchInitialCloses(futuresClient, symbol, symbolIntervalPair, wg)
		}
	}

	return symbolIntervalPair
}

func fetchInitialCloses(futuresClient *futures.Client, symbol string, symbolIntervalPair map[string]string, wg *sync.WaitGroup) {
	klines, err := futuresClient.NewKlinesService().
		Symbol(symbol).Interval(interval).Limit(LIMIT).Do(context.Background())
	if err != nil {
		log.Fatal().Str("err", err.Error()).Msg("Crashed fetching klines")
	}

	// Discard assets with less than LIMIT candles due to impossibility of computing EMA LIMIT.
	if len(klines) == LIMIT {
		for i := 0; i < LIMIT; i++ {
			if close, err := strconv.ParseFloat(klines[i].Close, 64); err == nil {
				symbolCloses[symbol] = append(symbolCloses[symbol], close)
			}
		}
	} else {
		delete(symbolIntervalPair, symbol)
	}
}

func wsKlineHandler(event *futures.WsKlineEvent) {
	k, symbol := event.Kline, event.Symbol

	parsedCandle := make(map[string]float64)
	rawCandle := map[string]string{
		"Open":  k.Open,
		"High":  k.High,
		"Low":   k.Low,
		"Close": k.Close,
	}

	for key, value := range rawCandle {
		parsedValue, err := strconv.ParseFloat(value, 64)
		if err != nil {
			log.Fatal().Str(key, value).Msg("Crashed fetching klines")
		}

		parsedCandle[key] = parsedValue
	}

	// NOTE: currently, only closes are updated (there may be TA indicators that use other OHLC values)
	// HACK: may want to hardcode the length (to find last close) for optimizing performance
	closes := symbolCloses[symbol]
	lastCloseIndex := len(symbolCloses[symbol]) - 1
	closes[lastCloseIndex] = parsedCandle["Close"] // Update the last candle

	// Rotate all candles but the last one (already set above).
	if k.IsFinal {
		// i.e., close[0] = close[1], ..., close[199] = parsedCandle["Close"]
		for i := 0; i < lastCloseIndex; i++ {
			closes[i] = closes[i+1]
		}
	}

	symbolCloses[symbol] = closes // Update the global map
	asset := symbolAssets[symbol]

	p := pair.New(closes, lastCloseIndex, &asset)

	// TODO: implement correct alert storage logic.
	// notAlerted := !(sentAlerts[symbol] != p.Side)

	// Only trade or send alert if there's a signal, a side, and no alert has been sent.
	if p.Signal_Count >= 1 && p.Side != pair.NA { // && notAlerted {
		log.Info().
			Str("EMA_Cross", p.EMA_Cross).
			Float64("Price", p.Price).
			Float64("RSI", p.RSI).
			Str("RSI_Signal", p.RSI_Signal).
			Uint("Signal_Count", p.Signal_Count).
			Str("Trend", p.Trend).
			Str("Side", p.Side).
			Str("Symbol", symbol).
			Msg("⚡")

		if *alertOnSignals {
			utils.SendTelegramAlert(bot, log, &p)
		}

		if *tradeSignals {
			order.New(futuresClient, log, &p)
		}

		// TODO: store triggered alerts based on signal+symbol, not just symbol.
		sentAlerts[symbol] = p.Side
	}
}

func parseFlags() {
	alertOnSignals = flag.Bool("alert", false, "send signal alerts on Telegram")
	tradeSignals = flag.Bool("trade", false, "trade signals on Binance USD-M")
	flag.StringVar(&interval, "interval", "", "interval to scan for: 1m, 3m, 5m, 15m, 30m, 1h, 4h, 1d")

	flag.Parse()

	intervalIsValid := false
	validIntervals := []string{"1m", "3m", "5m", "15m", "30m", "1h", "4h", "1d"}
	for _, valid_interval := range validIntervals {
		if interval == valid_interval {
			intervalIsValid = true
		}
	}

	if !intervalIsValid {
		log.Error().Msg("Please specify a valid interval")
		os.Exit(2)
	}
}

func init() {
	parseFlags()

	apiKey, secretKey := utils.LoadEnvFile(log)

	alerts := utils.LoadAlerts(log)

	log.Info().Int("count", len(alerts)).Msg("⚙️  Loaded alerts")

	bot = utils.NewTelegramBot(log)

	futuresClient = binance.NewFuturesClient(apiKey, secretKey)
}

func main() {
	var wg sync.WaitGroup

	// Handle CTRL-C (may want to do something on exit)
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	go func() {
		for sig := range c {
			log.Warn().Str("sig", sig.String()).Msg("Received CTRL-C")
			close(c)
			os.Exit(1)
		}
	}()

	log.Info().Str("interval", interval).Msg("💡 Fetching symbols")

	symbolIntervalPair := fetchAssets(futuresClient, &wg)

	wg.Wait()

	// TODO: find better way to wait for the cache to be built before starting the WS
	time.Sleep(time.Second)

	log.Info().Int("count", len(symbolIntervalPair)).Msg("🪙  Fetched symbols!")

	errHandler := func(err error) { log.Fatal().Msg(err.Error()) }

	doneC, _, err := futures.WsCombinedKlineServe(symbolIntervalPair, wsKlineHandler, errHandler)
	if err != nil {
		log.Fatal().Msg(err.Error())
	}

	log.Info().
		Bool("alert", *alertOnSignals).
		Bool("trade", *tradeSignals).
		Msg("🔌 WebSocket initialised!")

	if *alertOnSignals {
		utils.SendTelegramInit(bot, interval, log, len(symbolIntervalPair))
	}

	<-doneC
}
