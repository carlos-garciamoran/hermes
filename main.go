package main

import (
	"os"
	"os/signal"
	"strconv"
	"sync"

	"hermes/account"
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
var initialBalance float64
var interval string
var maxPositions int
var notifyOnSignals, simulatePositions, tradeSignals bool

var acct account.Account
var alerts []utils.Alert
var alertSymbols []string
var bot telegram.Bot
var futuresClient *futures.Client
var log zerolog.Logger = utils.InitLogging()
var openPositions = make(map[string]*position.Position) // Used to easily add/delete open positions.
var sentSignals = make(map[string]string)               // {"BTCUSDT": "bullish|bearish", ...}
var symbolAssets = make(map[string]analysis.Asset)      // Maps a symbol's string to its asset
var symbolCloses = make(map[string][]float64)           // {"BTCUSDT": [40004.75, ...], ...}
var symbolPrices = make(map[string]float64)             // {"BTCUSDT": 40004.75, ...}

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
	p, hasPositionWithSymbol := openPositions[symbol]
	if hasPositionWithSymbol {
		closed := false

		// Check if position should be closed according to side and SL/TP.
		if p.Side == analysis.BUY && price <= p.SL || p.Side == analysis.SELL && price >= p.SL {
			closed = true
			p.Close(price, "SL")
		} else if p.Side == analysis.BUY && price >= p.TP || p.Side == analysis.SELL && price <= p.TP {
			closed = true
			p.Close(price, "TP")
		}

		if closed {
			acct.LogClosedPosition(p)

			delete(openPositions, symbol)

			bot.SendClosedPosition(p)

			sublogger.Info().
				Str("ExitSignal", p.ExitSignal).
				Float64("NetPNL", p.NetPNL).
				Float64("PNL", p.PNL).
				Int("Slots", maxPositions-len(openPositions)).
				Msg(telegram.GetPNLEmoji(p.PNL) + " closed position")

			log.Info().
				Float64("AllocatedBalance", acct.AllocatedBalance).
				Float64("AvailableBalance", acct.AvailableBalance).
				Float64("NetPNL", acct.NetPNL).
				Int("Loses", acct.Loses).
				Int("Wins", acct.Wins).
				Msg("📄")
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

		targetSize := acct.TotalBalance / float64(maxPositions)
		hasAvailableBalance := acct.AvailableBalance >= targetSize
		hasFreeSlot := len(openPositions) < maxPositions

		if tradeSignals {
			order.New(futuresClient, log, &a)
		}

		if !hasPositionWithSymbol && hasAvailableBalance && hasFreeSlot && simulatePositions {
			p := position.New(&a, targetSize)

			acct.LogNewPosition(p)

			openPositions[symbol] = p

			bot.SendNewPosition(p)

			log.Info().
				Float64("EntryPrice", p.EntryPrice).
				Float64("Size", p.Size).
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
	initialBalance, interval, maxPositions, notifyOnSignals, simulatePositions, tradeSignals = utils.ParseFlags(log)

	utils.LoadEnvFile(log)

	bot = telegram.New(&log)

	futuresClient = binance.NewFuturesClient(os.Getenv("BINANCE_APIKEY"), os.Getenv("BINANCE_SECRETKEY"))

	// If on prod, use the exchange's account real balance.
	if tradeSignals {
		initialBalance = exchange.FetchBalance()
	}

	acct = account.New(initialBalance)
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
				bot.SendFinish(&acct, symbolPrices)
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
		Float64("balance", initialBalance).
		Int("max-positions", maxPositions).
		Bool("signals", notifyOnSignals).
		Bool("simulate", simulatePositions).
		Bool("trade", tradeSignals).
		Msg("🔌 WebSocket initialised!")

	if notifyOnSignals || simulatePositions || len(alerts) >= 1 {
		bot.SendInit(initialBalance, interval, maxPositions, simulatePositions)
	}

	bot.Listen(&acct, symbolPrices)

	<-doneC
}
