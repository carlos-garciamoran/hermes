package main

import (
	"hermes/pair"

	"context"
	"fmt"
	"math"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/adshao/go-binance/v2"
	"github.com/adshao/go-binance/v2/futures"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
	"github.com/markcheno/go-talib"
	"github.com/rs/zerolog"
)

const ID int64 = 445996511
const INTERVAL string = "1m"
const LIMIT int = 200

var bot *tgbotapi.BotAPI

var alerts = make(map[string]string)          // {"BTCUSDT": "bullish|bearish", ...}
var symbolCloses = make(map[string][]float64) // {"BTCUSDT": [40004.75, ...], ...}

var emoji = map[string]string{
	"bullish": "🐗",
	"bearish": "🐻",
}

func initLogging() zerolog.Logger {
	zerolog.TimeFieldFormat = time.RFC3339Nano // time.RFC3339, time.RFC822, zerolog.TimeFormatUnix
	output := zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339Nano}

	output.FormatLevel = func(i interface{}) string {
		return strings.ToUpper(fmt.Sprintf("| %-5s|", i))
	}

	return zerolog.New(output).With().Timestamp().Logger()
}

func loadEnvFile() (string, string, string) {
	err := godotenv.Load()

	if err != nil {
		panic("Error loading .env file")
	}

	return os.Getenv("BINANCE_APIKEY"), os.Getenv("BINANCE_SECRETKEY"), os.Getenv("TELEGRAM_APITOKEN")
}

func fetchSymbols(wg sync.WaitGroup, futuresClient *futures.Client) map[string]string {
	symbolIntervalPair := make(map[string]string)

	exchangeInfo, err := futuresClient.NewExchangeInfoService().Do(context.Background())
	if err != nil {
		panic(err)
	}

	// Filter unwanted symbols (non-USDT, quarterlies, indexes, unactive, and 1000BTTC)
	for _, asset := range exchangeInfo.Symbols {
		if asset.QuoteAsset == "USDT" && asset.ContractType == "PERPETUAL" && asset.UnderlyingType == "COIN" &&
			asset.Status == "TRADING" && asset.BaseAsset != "1000BTTC" {
			symbol := asset.Symbol
			symbolIntervalPair[symbol] = INTERVAL

			wg.Add(1)

			go fetchInitialCloses(symbol, wg, futuresClient)
		}
	}

	return symbolIntervalPair
}

func fetchInitialCloses(symbol string, wg sync.WaitGroup, futuresClient *futures.Client) {
	defer wg.Done()

	klines, err := futuresClient.NewKlinesService().
		Symbol(symbol).Interval(INTERVAL).Limit(LIMIT).
		Do(context.Background())

	if err != nil {
		panic(err)
	}

	// NOTE: can't use limit because asset may be too new (num of candles < 200)
	kline_count := len(klines)

	for i := 0; i < kline_count; i++ {
		if close, err := strconv.ParseFloat(klines[i].Close, 64); err == nil {
			symbolCloses[symbol] = append(symbolCloses[symbol], close)
		}
	}

	fmt.Printf("💡 %-12s: downloaded %d candles\n", symbol, kline_count)
}

func sendTelegramAlert(bot *tgbotapi.BotAPI, symbol string, side string, RSI float64) {
	text := fmt.Sprintf("%s *%s*: %s cross ⚡️\n"+
		"    — RSI: %.2f",
		emoji[side], symbol[:len(symbol)-4], side, RSI,
	)

	msg := tgbotapi.MessageConfig{
		BaseChat: tgbotapi.BaseChat{
			ChatID: ID,
		},
		Text:      text,
		ParseMode: tgbotapi.ModeMarkdown,
	}

	if _, err := bot.Send(msg); err != nil {
		panic(err)
	}
}

func main() {
	// Used for building initial candles cache async & quickly
	var err error
	var wg sync.WaitGroup

	log := initLogging()

	apiKey, secretKey, telegramToken := loadEnvFile()

	bot, err = tgbotapi.NewBotAPI(telegramToken)
	if err != nil {
		log.Fatal().Msg(err.Error())
	}

	futuresClient := binance.NewFuturesClient(apiKey, secretKey)

	symbols := fetchSymbols(wg, futuresClient)

	// Handle CTRL-C (may want to print something)
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	go func() {
		for sig := range c {
			fmt.Println(sig)
			os.Exit(1)
		}
	}()

	// TODO: move into dedicated function: need to pass log object
	wsKlineHandler := func(event *futures.WsKlineEvent) {
		k := event.Kline
		symbol := event.Symbol

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
				log.Fatal().Str(key, value).
					Msg(fmt.Sprintf("Could not parse %s", key))
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

		// Round the RSI to 2 digits
		RSI := math.Round(talib.Rsi(closes, 14)[lastCloseIndex]*100) / 100
		EMA_09, EMA_21 := talib.Ema(closes, 9)[lastCloseIndex-2:], talib.Ema(closes, 21)[lastCloseIndex-2:]

		p := pair.New(EMA_09, EMA_21, parsedCandle["Close"], RSI, symbol)

		// Only send alert if there's a cross and we haven't alerted yet.
		if p.Bias != "NA" && alerts[symbol] != p.Bias {
			alerts[symbol] = p.Bias

			sendTelegramAlert(bot, symbol, p.Bias, RSI)

			log.Info().
				Float64("Price", p.Price).
				Float64("EMA_09", EMA_09[2]).
				Float64("EMA_21", EMA_21[2]).
				Float64("RSI", RSI).
				Str("_Cross", p.Bias).
				Msg(symbol[:len(symbol)-4]) // No need to print "USDT"
		}
	}

	errHandler := func(err error) { log.Fatal().Msg(err.Error()) }

	wg.Wait()

	// TODO: find better way to wait for the cache to be built before starting the WS
	time.Sleep(2 * time.Second)

	log.Info().Int("count", len(symbols)).Msg("🪙 Fetched symbols")

	doneC, _, err := futures.WsCombinedKlineServe(symbols, wsKlineHandler, errHandler)
	if err != nil {
		log.Fatal().Msg(err.Error())
	}

	log.Debug().Msg("🔌 Socket initialised")

	<-doneC
}
