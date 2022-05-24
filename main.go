package main

import (
	"math"
	"os"
	"os/signal"
	"strconv"
	"sync"

	"hermes/account"
	"hermes/analysis"
	"hermes/exchange"
	"hermes/position"
	"hermes/telegram"
	"hermes/utils"

	"github.com/adshao/go-binance/v2/futures"
	"github.com/rs/zerolog"
)

const LIMIT int = 200

// CLI flags
var initialBalance float64
var interval string
var maxPositions int
var onDev, trackPositions, isReal, sendSignals bool

var acct account.Account
var alerts []analysis.Alert
var alertSymbols []string
var bot telegram.Bot
var excg exchange.Exchange
var log zerolog.Logger = utils.InitLogging()
var openPositions = make(map[string]*position.Position) // Used to easily add/delete open positions.
var triggeredSignals = make(map[string]string)          // {"BTCUSDT": "bullish|bearish", ...}
var symbolAssets = make(map[string]analysis.Asset)      // Symbol-to-asset mapping.
var symbolCloses = make(map[string][]float64)           // {"BTCUSDT": [40004.75, ...], ...}
var symbolPrices = make(map[string]float64)             // {"BTCUSDT": 40004.75, ...}

// wsKlineHandler is called on every price update. It parses the passed kline, checks if a position
// needs to be closed or opened, and if an alert or a signal is triggered.
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
			if isReal {
				excg.CloseOrder(p)
			}

			acct.LogClosedPosition(p)

			delete(openPositions, symbol)

			bot.SendClosedPosition(p)

			sublogger.Info().
				Str("ExitSignal", p.ExitSignal).
				Float64("NetPNL", p.NetPNL).
				Float64("PNL", p.PNL).
				Int("Slots", maxPositions-len(openPositions)).
				Msg(telegram.GetPNLEmoji(p.PNL) + " closed")

			log.Info().
				Float64("AllocatedBalance", acct.AllocatedBalance).
				Float64("AvailableBalance", acct.AvailableBalance).
				Float64("TotalBalance", acct.TotalBalance).
				Float64("NetPNL", acct.NetPNL).
				Float64("PNL", acct.PNL).
				Int("Loses", acct.Loses).
				Int("Wins", acct.Wins).
				Msg("📄")
		}
	}

	// TODO: first, check if symbol has alert.
	if triggersAlert, targetPrice := a.TriggersAlert(&alerts); triggersAlert {
		sublogger.Info().Float64("TargetPrice", targetPrice).Msg("🔔")

		bot.SendAlert(&a, targetPrice)
	}

	if a.TriggersSignal(triggeredSignals) {
		if sendSignals {
			bot.SendSignal(&a)

			sublogger.Info().
				Str("EMA_Cross", a.EMACross).
				Uint("Signal_Count", a.SignalCount).
				Str("Side", a.Side).
				Msg("⚡")
		}

		// NOTE: to be safer, may want to factor in unrealized PNL ([TotalBalance+uPNL] / maxPositions)
		// Round size to 2 digits
		targetSize := math.Floor((acct.TotalBalance/float64(maxPositions))*100) / 100
		targetQuantity := targetSize / price

		hasEnoughBalance := acct.AvailableBalance >= targetSize
		hasAFreeSlot := len(openPositions) < maxPositions
		hasValidQuantity := targetQuantity >= asset.MinQuantity && targetQuantity <= asset.MaxQuantity

		if !hasPositionWithSymbol && hasEnoughBalance && hasAFreeSlot && hasValidQuantity && trackPositions {
			p := position.New(&a, isReal, targetQuantity, targetSize)

			if isReal {
				excg.NewOrder(p)
			}

			openPositions[symbol] = p

			acct.LogNewPosition(p)
			bot.SendNewPosition(p)

			log.Info().
				Float64("EntryPrice", p.EntryPrice).
				Float64("Quantity", p.Quantity).
				Float64("Size", p.Size).
				Int("Slots", maxPositions-len(openPositions)).
				Str("Symbol", p.Symbol).
				Float64("SL", p.SL).
				Float64("TP", p.TP).
				Msg("💡")

			log.Info().
				Float64("AllocatedBalance", acct.AllocatedBalance).
				Float64("AvailableBalance", acct.AvailableBalance).
				Msg("📄")
		}

		triggeredSignals[a.Symbol] = a.Side
	}
}

func init() {
	initialBalance, onDev, interval, maxPositions, trackPositions, isReal, sendSignals = utils.ParseFlags(&log)

	utils.LoadEnvFile(&log)

	bot = telegram.New(&log, onDev)

	excg = exchange.New(&bot, &log)

	if isReal {
		initialBalance = excg.FetchBalance()
	}

	if initialBalance < 5 {
		log.Fatal().
			Float64("initialBalance", initialBalance).
			Msg("Initial balance should be at least 5")
	}

	acct = account.New(initialBalance, !trackPositions)
}

func main() {
	var wg sync.WaitGroup

	usesTelegramBot := len(alerts) >= 1 || trackPositions || sendSignals

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt) // Listen for CTRL-C.

	go func() {
		utils.HandleCTRLC(&acct, &bot, c, &excg, isReal, &log, symbolPrices, usesTelegramBot)
	}()

	log.Info().Str("interval", interval).Msg("📡 Fetching symbols...")

	symbolIntervalPair := excg.FetchAssets(interval, LIMIT, symbolAssets, symbolCloses, &wg)

	wg.Wait()

	log.Info().Int("count", len(symbolIntervalPair)).Msg("🪙  Fetched symbols!")

	alerts, alertSymbols = utils.LoadAlerts(&log, interval, symbolIntervalPair)
	log.Info().Int("count", len(alerts)).Msg("⚙️  Loaded alerts")

	errHandler := func(err error) {
		msg := "💥 WebSocket stream crashed"
		bot.SendMessage(msg)
		log.Fatal().Str("err", err.Error()).Msg(msg)
	}

	doneC, _, err := futures.WsCombinedKlineServe(symbolIntervalPair, wsKlineHandler, errHandler)
	if err != nil {
		log.Fatal().Str("err", err.Error()).Msg("💥 Crashed on futures.WsCombinedKlineServe")
	}

	log.Info().
		Float64("balance", initialBalance).
		Bool("dev", onDev).
		Int("max-positions", maxPositions).
		Bool("positions", trackPositions).
		Bool("real", isReal).
		Bool("signals", sendSignals).
		Msg("🔌 WebSocket initialised!")

	if usesTelegramBot {
		bot.SendInit(initialBalance, interval, maxPositions, trackPositions, isReal)
	}

	bot.Listen(&acct, symbolPrices)

	<-doneC
}
