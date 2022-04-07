package main

import (
	"hermes/analysis"
	"hermes/order"
	"hermes/telegram"
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
	"github.com/rs/zerolog"
)

const LIMIT int = 200

var log zerolog.Logger = utils.InitLogging()

var alerts []utils.Alert
var alertOnSignals *bool
var bot telegram.Bot
var futuresClient *futures.Client
var interval string
var sentAlerts = make(map[string]string) // {"BTCUSDT": "bullish|bearish", ...}
var symbolAssets = make(map[string]analysis.Asset)
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

			symbolAssets[symbol] = analysis.Asset{
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

func checkAlerts(a *analysis.Analysis) {
	price, symbol := a.Price, a.Symbol

	// HACK: using a pre-built symbol map (of alerts) may improve performance: O(1) beats O(n)
	for i, alert := range alerts {
		if alert.Symbol == symbol && !alert.Notified && alert.Type == "price" {
			// TODO: check if parentheses are actually needed.
			alertTriggered := (alert.Condition == ">=" && price >= alert.Price) ||
				(alert.Condition == "<=" && price <= alert.Price) ||
				(alert.Condition == "<" && price < alert.Price) ||
				(alert.Condition == ">" && price > alert.Price)

			if alertTriggered {
				log.Info().Str("symbol", symbol).Float64("price", price).Msg("Alert triggered!")
				// bot.SendAlert(log, a)
				alerts[i].Notified = true
			}
		}
	}
}

func checkSignals(a *analysis.Analysis) {
	// Only trade or send alert if there's a signal, a side, and no alert has been sent.
	if a.Signal_Count >= 1 && a.Side != analysis.NA && sentAlerts[a.Symbol] != a.Side {
		log.Info().
			Str("EMA_Cross", a.EMA_Cross).
			Float64("Price", a.Price).
			Float64("RSI", a.RSI).
			Str("RSI_Signal", a.RSI_Signal).
			Uint("Signal_Count", a.Signal_Count).
			Str("Trend", a.Trend).
			Str("Side", a.Side).
			Str("Symbol", a.Asset.BaseAsset).
			Msg("⚡")

		if *alertOnSignals {
			bot.SendAlert(log, a)
		}

		if *tradeSignals {
			order.New(futuresClient, log, a)
		}

		// TODO: store triggered alerts based on signal+symbol, not just symbol.
		sentAlerts[a.Symbol] = a.Side
	}
}

func wsKlineHandler(event *futures.WsKlineEvent) {
	k, symbol := event.Kline, event.Symbol

	parsedCandle := make(map[string]float64, 4)
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

	price := parsedCandle["Close"]

	// NOTE: currently, only closes are updated (there may be TA indicators using other OHLC values)
	closes := symbolCloses[symbol]
	lastCloseIndex := LIMIT - 1

	closes[lastCloseIndex] = price // Update the last candle

	// Rotate all candles but the last one (already set above).
	if k.IsFinal {
		// close[0] = close[1], ..., close[199] = parsedCandle["Close"]
		for i := 0; i < lastCloseIndex; i++ {
			closes[i] = closes[i+1]
		}
	}

	symbolCloses[symbol] = closes // Update the global map
	asset := symbolAssets[symbol]

	a := analysis.New(closes, lastCloseIndex, &asset)

	checkAlerts(&a)

	checkSignals(&a)
}

func parseFlags() {
	alertOnSignals = flag.Bool("signals", false, "send signal alerts on Telegram")
	tradeSignals = flag.Bool("trade", false, "trade signals on Binance USD-M")
	flag.StringVar(&interval, "interval", "", "interval to perform TA: 1m, 3m, 5m, 15m, 30m, 1h, 4h, 1d")

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

	alerts = utils.LoadAlerts(log)

	log.Info().Int("count", len(alerts)).Msg("⚙️  Loaded alerts")

	bot = telegram.NewBot(log)

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

	log.Info().Str("interval", interval).Msg("💡 Fetching symbols...")

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
		Bool("signals", *alertOnSignals).
		Bool("trade", *tradeSignals).
		Msg("🔌 WebSocket initialised!")

	if *alertOnSignals || len(alerts) >= 1 {
		bot.SendInit(interval, log, len(symbolIntervalPair))
	}

	<-doneC
}
