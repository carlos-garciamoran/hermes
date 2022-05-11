package main

import (
	"os"
	"os/signal"
	"strconv"
	"sync"

	"hermes/analysis"
	"hermes/exchange"
	"hermes/order"
	"hermes/position"
	"hermes/telegram"
	"hermes/utils"

	"github.com/adshao/go-binance/v2"
	"github.com/adshao/go-binance/v2/futures"
	"github.com/rs/zerolog"
)

const LIMIT int = 200

// CLI flags
var interval string
var maxPositions int
var notifyOnSignals, simulatePositions, tradeSignals bool

var alerts []utils.Alert
var alertSymbols []string
var bot telegram.Bot
var futuresClient *futures.Client
var log zerolog.Logger = utils.InitLogging()
var openPositions = make(map[string]position.Position)
var sentSignals = make(map[string]string) // {"BTCUSDT": "bullish|bearish", ...}
var symbolAssets = make(map[string]analysis.Asset)
var symbolCloses = make(map[string][]float64) // {"BTCUSDT": [40004.75, ...], ...}
var symbolPrices = make(map[string]float64)   // {"BTCUSDT": 40004.75, ...}

func wsKlineHandler(event *futures.WsKlineEvent) {
	k, symbol := event.Kline, event.Symbol

	parsedCandle := make(map[string]float64, 4)
	rawCandle := map[string]string{
		"Open": k.Open, "High": k.High, "Low": k.Low, "Close": k.Close,
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

	// NOTE: declaration not inlined in `if` so variable is accessible afterwards.
	p, positionExists := openPositions[symbol]
	if positionExists {
		closed := false

		if price <= p.SL {
			p.Close(price, "SL")
			closed = true
		} else if price > p.TP {
			p.Close(price, "TP")
			closed = true
		}

		if closed {
			delete(openPositions, symbol)
			bot.SendClosedPosition(&p)
			sublogger.Info().
				Str("ExitSignal", p.ExitSignal).
				Float64("NetPNL", p.NetPNL).
				Float64("PNL", p.PNL).
				Int("Slots", maxPositions-len(openPositions)).
				Msg(telegram.GetPNLEmoji(p.PNL) + " closed position")
		}
	}

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

		// Only open a simulated position if we want to, a position for the symbol has not been opened,
		// and we haven't reached the limit of positions.
		if simulatePositions && !positionExists && len(openPositions) < maxPositions {
			p := position.New(&a)
			openPositions[symbol] = p

			bot.SendNewPosition(&p)

			log.Info().
				Float64("EntryPrice", p.EntryPrice).
				Str("EntrySignal", p.EntrySignal).
				Int("Slots", maxPositions-len(openPositions)).
				Str("Symbol", p.Symbol).
				Float64("SL", p.SL).
				Float64("TP", p.TP).
				Msg("Opened position")
		}

		sentSignals[a.Symbol] = a.Side
	}
}

func init() {
	maxPositions, notifyOnSignals, simulatePositions, tradeSignals, interval = utils.ParseFlags(log)

	apiKey, secretKey := utils.LoadEnvFile(log)

	bot = telegram.New(&log)

	futuresClient = binance.NewFuturesClient(apiKey, secretKey)
}

func main() {
	var wg sync.WaitGroup

	// Handle CTRL-C.
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	go func() {
		for sig := range c {
			log.Warn().Str("sig", sig.String()).Msg("Received CTRL-C. Exiting...")
			if notifyOnSignals || simulatePositions || len(alerts) >= 1 {
				bot.SendFinish(symbolPrices)
			}
			close(c)
			os.Exit(1)
		}
	}()

	log.Info().Str("interval", interval).Msg("💡 Fetching symbols...")

	symbolIntervalPair := exchange.FetchAssets(
		futuresClient, interval, LIMIT, &log, symbolAssets, symbolCloses, &wg,
	)

	wg.Wait()

	log.Info().Int("count", len(symbolIntervalPair)).Msg("🪙  Fetched symbols!")

	alerts, alertSymbols = utils.LoadAlerts(log, interval, symbolIntervalPair)
	log.Info().Int("count", len(alerts)).Msg("⚙️  Loaded alerts")

	errHandler := func(err error) {
		msg := "WebSocket stream crashed 🧨"
		bot.SendMessage(msg)
		log.Fatal().Str("err", err.Error()).Msg(msg)
	}

	doneC, _, err := futures.WsCombinedKlineServe(symbolIntervalPair, wsKlineHandler, errHandler)
	if err != nil {
		log.Fatal().Str("err", err.Error()).Msg("Crashed calling WsCombinedKlineServe")
	}

	log.Info().
		Int("max-positions", maxPositions).
		Bool("simulate", simulatePositions).
		Bool("signals", notifyOnSignals).
		Bool("trade", tradeSignals).
		Msg("🔌 WebSocket initialised!")

	if notifyOnSignals || simulatePositions || len(alerts) >= 1 {
		bot.SendInit(interval, maxPositions, simulatePositions, len(symbolIntervalPair))
	}

	bot.Listen(symbolPrices)

	<-doneC
}
