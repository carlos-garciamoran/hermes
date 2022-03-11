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

const INTERVAL string = "1m"
const LIMIT int = 200

var apiKey, secretKey, telegramToken string
var bot *tgbotapi.BotAPI
var chatID int64

var alerts = make(map[string]string)          // {"BTCUSDT": "bullish|bearish", ...}
var symbolCloses = make(map[string][]float64) // {"BTCUSDT": [40004.75, ...], ...}

func initLogging() zerolog.Logger {
	zerolog.TimeFieldFormat = time.RFC3339Nano // time.RFC3339, time.RFC822, zerolog.TimeFormatUnix
	output := zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339Nano}

	output.FormatLevel = func(i interface{}) string {
		return strings.ToUpper(fmt.Sprintf("| %-5s|", i))
	}

	return zerolog.New(output).With().Timestamp().Logger()
}

func loadEnvFile() {
	var err error

	err = godotenv.Load()

	if err != nil {
		fmt.Println("Error loading .env file:", err)
		os.Exit(1)
	}

	apiKey, secretKey = os.Getenv("BINANCE_APIKEY"), os.Getenv("BINANCE_SECRETKEY")
	telegramToken = os.Getenv("TELEGRAM_APITOKEN")

	chatID, err = strconv.ParseInt(os.Getenv("TELEGRAM_CHATID"), 10, 64)
	if err != nil {
		fmt.Println("Error parsing TELEGRAM_CHATID:", err)
		os.Exit(1)
	}
}

func fetchSymbols(wg sync.WaitGroup, futuresClient *futures.Client) map[string]string {
	symbolIntervalPair := make(map[string]string)

	exchangeInfo, err := futuresClient.NewExchangeInfoService().Do(context.Background())
	if err != nil {
		fmt.Println("Crashed getting exchange info:", err)
		os.Exit(1)
	}

	// Filter unwanted symbols (non-USDT, quarterlies, indexes, unactive, and 1000BTTC)
	for _, asset := range exchangeInfo.Symbols {
		if asset.QuoteAsset == "USDT" && asset.ContractType == "PERPETUAL" && asset.UnderlyingType == "COIN" &&
			asset.Status == "TRADING" && asset.BaseAsset != "1000BTTC" {
			symbol := asset.Symbol
			symbolIntervalPair[symbol] = INTERVAL

			wg.Add(1)

			go fetchInitialCloses(futuresClient, symbol, wg)
		}
	}

	return symbolIntervalPair
}

func fetchInitialCloses(futuresClient *futures.Client, symbol string, wg sync.WaitGroup) {
	defer wg.Done()

	klines, err := futuresClient.NewKlinesService().
		Symbol(symbol).Interval(INTERVAL).Limit(LIMIT).
		Do(context.Background())
	if err != nil {
		fmt.Println("Crashed fetching klines:", err)
		os.Exit(1)
	}

	// NOTE: we don't use LIMIT because asset may be too new, so len(klines) < 200
	kline_count := len(klines)

	for i := 0; i < kline_count; i++ {
		if close, err := strconv.ParseFloat(klines[i].Close, 64); err == nil {
			symbolCloses[symbol] = append(symbolCloses[symbol], close)
		}
	}

	fmt.Printf("💡 %-12s: downloaded %d candles\n", symbol, kline_count)
}

func sendTelegramAlert(bot *tgbotapi.BotAPI, p *pair.Pair) {
	text := fmt.Sprintf("%s *%s*: %s cross ⚡️\n"+
		"    — RSI: %.2f",
		pair.Emoji[p.Bias], p.Symbol, p.Bias, p.RSI,
	)

	msg := tgbotapi.MessageConfig{
		BaseChat: tgbotapi.BaseChat{
			ChatID: chatID,
		},
		Text:      text,
		ParseMode: tgbotapi.ModeMarkdown,
	}

	// NOTE: may want to continue running instead of doing os.Exit()
	if _, err := bot.Send(msg); err != nil {
		fmt.Println("Crashed sending Telegram alert:", err)
		os.Exit(1)
	}
}

func main() {
	var err error
	var wg sync.WaitGroup

	log := initLogging()

	loadEnvFile()

	bot, err = tgbotapi.NewBotAPI(telegramToken)
	if err != nil {
		log.Fatal().Msg(err.Error())
	}

	futuresClient := binance.NewFuturesClient(apiKey, secretKey)

	symbols := fetchSymbols(wg, futuresClient)

	// Handle CTRL-C (may want to do something on exit)
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

		EMA_09 := talib.Ema(closes, 9)[lastCloseIndex-2:]
		EMA_21 := talib.Ema(closes, 21)[lastCloseIndex-2:]
		EMA_100 := talib.Ema(closes, 100)[lastCloseIndex]
		EMA_200 := talib.Ema(closes, 200)[lastCloseIndex]

		// Round to 2 digits.
		RSI := math.Round(talib.Rsi(closes, 14)[lastCloseIndex]*100) / 100

		p := pair.New(EMA_09, EMA_21, parsedCandle["Close"], RSI, symbol)

		// Only send alert if there's a cross and we haven't alerted yet.
		if p.Bias != "NA" && alerts[symbol] != p.Bias {
			alerts[symbol] = p.Bias

			sendTelegramAlert(bot, &p)

			log.Info().
				Float64("Price", p.Price).
				Float64("EMA_09", EMA_09[2]).
				Float64("EMA_21", EMA_21[2]).
				Float64("EMA_100", EMA_100).
				Float64("EMA_200", EMA_200).
				Float64("RSI", RSI).
				Str("_Cross", p.Bias).
				Msg(p.Symbol)
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

	log.Debug().Msg("🔌 WebSocket initialised")

	<-doneC
}
