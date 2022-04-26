package main

import (
	"hermes/analysis"
	"hermes/order"
	"hermes/position"
	"hermes/telegram"
	"hermes/utils"

	"context"
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

// CLI flags
var notifyOnSignals, simulatePositions, tradeSignals bool
var interval string
var maxPositions int

var alerts []utils.Alert
var alertSymbols []string
var bot telegram.Bot
var log zerolog.Logger = utils.InitLogging()
var futuresClient *futures.Client
var simulatedPositions = make(map[string]position.Position)
var sentSignals = make(map[string]string) // {"BTCUSDT": "bullish|bearish", ...}
var symbolAssets = make(map[string]analysis.Asset)
var symbolCloses = make(map[string][]float64) // {"BTCUSDT": [40004.75, ...], ...}
var symbolPrices = make(map[string]float64)   // {"BTCUSDT": 40004.75, ...}

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
			log.Fatal().Str(key, value).Msg("Crashed parsing klines")
		}

		parsedCandle[key] = parsedValue
	}

	price := parsedCandle["Close"]

	// NOTE: currently, only closes are updated (there may be TA indicators using other OHLC values)
	closes := symbolCloses[symbol]
	closes[LIMIT-1] = price // Update the last candle

	// Rotate all candles but the last one (already set above).
	if k.IsFinal {
		text := "Updating candles for " + symbol
		bot.SendMessage(&text)

		// close[0] = close[1], ..., close[198] = close[199]
		for i := 0; i < LIMIT-1; i++ {
			closes[i] = closes[i+1]
		}
	}

	// Update global maps
	symbolCloses[symbol] = closes
	symbolPrices[symbol] = price

	asset := symbolAssets[symbol]

	// NOTE: may not need to pass LIMIT (it is len(closes))... might be interesting performance-wise(?)
	a := analysis.New(&asset, closes, LIMIT-1)

	sublogger := log.With().
		Float64("Price", a.Price).
		Float64("RSI", a.RSI).
		Str("Symbol", a.Asset.BaseAsset).
		Str("Trend", a.Trend).
		Logger()

	// TODO: first, check if symbol has alert.
	if triggersAlert, target := a.TriggersAlert(&alerts); triggersAlert {
		sublogger.Info().Float64("Target", target).Msg("🔔")

		bot.SendAlert(&a, target)
	}

	if a.TriggersSignal(sentSignals) {
		sublogger.Info().
			Str("EMA_Cross", a.EMA_Cross).
			Uint("Signal_Count", a.Signal_Count).
			Str("Side", a.Side).
			Msg("⚡")

		if notifyOnSignals {
			bot.SendSignal(&a)
		}

		if tradeSignals {
			order.New(futuresClient, log, &a)
		}

		_, pExists := simulatedPositions[symbol]

		// Only open a simulated position if we want to, a position for the symbol has not been opened,
		// and we haven't reached the limit of positions.
		if simulatePositions && !pExists && len(simulatedPositions) < maxPositions {
			p := position.New(&a)
			simulatedPositions[symbol] = p

			bot.SendPosition(&p)

			log.Info().
				Float64("EntryPrice", p.EntryPrice).
				Str("EntrySignal", p.EntrySignal).
				Str("Symbol", p.Symbol).
				Int("Free slots", maxPositions-len(simulatedPositions)).
				Msg("Opened position")
		}

		sentSignals[a.Symbol] = a.Side
	}
}

func init() {
	maxPositions, notifyOnSignals, simulatePositions, tradeSignals, interval = utils.ParseFlags(log)

	apiKey, secretKey := utils.LoadEnvFile(log)

	bot = telegram.NewBot(&log)

	futuresClient = binance.NewFuturesClient(apiKey, secretKey)
}

func main() {
	var wg sync.WaitGroup

	// Handle CTRL-C (may want to do something on exit)
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	go func() {
		for sig := range c {
			log.Warn().Str("sig", sig.String()).Msg("Received CTRL-C. Exiting...")
			if notifyOnSignals || len(alerts) >= 1 {
				bot.SendFinish()
			}
			close(c)
			os.Exit(1)
		}
	}()

	log.Info().Str("interval", interval).Msg("💡 Fetching symbols...")

	symbolIntervalPair := fetchAssets(futuresClient, &wg)

	alerts, alertSymbols = utils.LoadAlerts(log, interval, symbolIntervalPair)
	log.Info().Int("count", len(alerts)).Msg("⚙️  Loaded alerts")

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
		Int("max-positions", maxPositions).
		Bool("simulate", simulatePositions).
		Bool("signals", notifyOnSignals).
		Bool("trade", tradeSignals).
		Msg("🔌 WebSocket initialised!")

	if notifyOnSignals || len(alerts) >= 1 {
		bot.SendInit(interval, maxPositions, simulatePositions, len(symbolIntervalPair))
	}

	bot.Listen(symbolPrices)

	<-doneC
}
