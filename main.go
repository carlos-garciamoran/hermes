package main

import (
	"flag"
	"hermes/order"
	"hermes/pair"
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

var log zerolog.Logger = utils.InitLogging()

var alertOnSignals *bool
var futuresClient *futures.Client
var interval string
var sentAlerts = make(map[string]string)      // {"BTCUSDT": "bullish|bearish", ...}
var symbolCloses = make(map[string][]float64) // {"BTCUSDT": [40004.75, ...], ...}
var tradeSignals *bool

func fetchSymbols(futuresClient *futures.Client, wg *sync.WaitGroup) map[string]string {
	symbolIntervalPair := make(map[string]string)

	exchangeInfo, err := futuresClient.NewExchangeInfoService().Do(context.Background())
	if err != nil {
		log.Fatal().Str("err", err.Error()).Msg("Crashed getting exchange info")
	}

	// Filter unwanted symbols (non-USDT, quarterlies, indexes, unactive, and 1000BTTC)
	for _, asset := range exchangeInfo.Symbols {
		if asset.QuoteAsset == "USDT" && asset.ContractType == "PERPETUAL" && asset.UnderlyingType == "COIN" &&
			asset.Status == "TRADING" && asset.BaseAsset != "1000BTTC" {
			symbol := asset.Symbol
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

	// NOTE: we don't use LIMIT because asset may be too new, so len(klines) < 200
	kline_count := len(klines)

	// Discard assets with few candles due to impossibility of doing TA.
	if kline_count >= 40 {
		for i := 0; i < kline_count; i++ {
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

	p := pair.New(closes, lastCloseIndex, symbol)

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
			Str("Symbol", p.Symbol).
			Msg("⚡")

		if *alertOnSignals {
			utils.SendTelegramAlert(log, &p)
		}

		if *tradeSignals {
			order.New(futuresClient, log, &p)
		}

		// TODO: store triggered alerts based on signal+symbol, not just symbol.
		sentAlerts[symbol] = p.Side
	}
}

func main() {
	var err error
	var wg sync.WaitGroup

	apiKey, secretKey := utils.LoadEnvFile(log)

	// TODO: implement alerts handling.
	// alerts := utils.LoadAlerts(log)

	alertOnSignals = flag.Bool("signals", false, "send signal alerts on Telegram")
	tradeSignals = flag.Bool("trade", false, "trade signals on Binance USD-M")
	flag.StringVar(&interval, "interval", "4h", "interval to scan for")

	flag.Parse()

	// TODO: change to bot instance >>> bot := telegram.NewTelegramBot(log)
	// bot := telegram.NewTelegramBot(log)
	utils.NewTelegramBot(log)

	futuresClient := binance.NewFuturesClient(apiKey, secretKey)

	log.Debug().Str("interval", interval).Msg("💡 Fetching symbols")

	symbols := fetchSymbols(futuresClient, &wg)

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

	wg.Wait()

	// TODO: find better way to wait for the cache to be built before starting the WS
	time.Sleep(2 * time.Second)

	log.Info().Int("count", len(symbols)).Msg("🪙  Fetched symbols")

	errHandler := func(err error) { log.Fatal().Msg(err.Error()) }

	doneC, _, err := futures.WsCombinedKlineServe(symbols, wsKlineHandler, errHandler)
	if err != nil {
		log.Fatal().Msg(err.Error())
	}

	log.Debug().Msg("🔌 WebSocket initialised")

	utils.SendTelegramInit(interval, log, len(symbols))

	<-doneC
}
