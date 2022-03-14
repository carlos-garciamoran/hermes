package utils

import (
	"hermes/pair"
	"os"
	"strconv"
	"strings"
	"time"

	"fmt"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
)

var bot *tgbotapi.BotAPI
var chatID int64
var telegramToken string

func buildMessage(text string) tgbotapi.MessageConfig {
	return tgbotapi.MessageConfig{
		BaseChat: tgbotapi.BaseChat{
			ChatID: chatID,
		},
		Text:      text,
		ParseMode: tgbotapi.ModeMarkdown,
	}
}

func InitLogging() zerolog.Logger {
	zerolog.TimeFieldFormat = time.RFC3339Nano // time.RFC3339, time.RFC822, zerolog.TimeFormatUnix
	output := zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339Nano}

	output.FormatLevel = func(i interface{}) string {
		return strings.ToUpper(fmt.Sprintf("|%-5s|", i))
	}

	return zerolog.New(output).With().Timestamp().Logger()
}

func LoadEnvFile() (string, string) {
	var err error

	err = godotenv.Load()

	if err != nil {
		fmt.Println("Error loading .env file:", err)
		os.Exit(1)
	}

	telegramToken = os.Getenv("TELEGRAM_APITOKEN")

	chatID, err = strconv.ParseInt(os.Getenv("TELEGRAM_CHATID"), 10, 64)
	if err != nil {
		fmt.Println("Error parsing TELEGRAM_CHATID:", err)
		os.Exit(1)
	}

	return os.Getenv("BINANCE_APIKEY"), os.Getenv("BINANCE_SECRETKEY")
}

func NewTelegramBot() {
	var err error

	bot, err = tgbotapi.NewBotAPI(telegramToken)

	if err != nil {
		fmt.Println("Crashed creating Telegram bot:", err)
		os.Exit(1)
	}
}

func SendTelegramInit(interval string, symbol_count int) {
	text := "🔔🔔 *NEW SESSION STARTED* 🔔🔔\n\n" +
		fmt.Sprintf("    ⏱ interval: >>>*%s*<<<\n", interval) +
		fmt.Sprintf("    🪙 symbols: >>>*%d*<<<", symbol_count)

	if _, err := bot.Send(buildMessage(text)); err != nil {
		fmt.Println("Crashed sending Telegram init:", err)
		os.Exit(1)
	}
}

func SendTelegramAlert(p *pair.Pair) {
	text := fmt.Sprintf("⚡️ %s", p.Symbol)

	if p.EMA_Cross != "NA" {
		text += fmt.Sprintf(" | _%s EMA cross_ %s", p.EMA_Cross, pair.Emojis[p.EMA_Cross])
	}

	if p.RSI_Signal != "NA" {
		text += fmt.Sprintf(" | _RSI %s_ %s", p.RSI_Signal, pair.Emojis[p.RSI_Signal])
	}

	text += fmt.Sprintf("\n"+
		"    — Trend: _%s_ %s\n"+
		"    — RSI: %.2f\n\n"+
		"    🔮 Side: *%s* %s",
		p.Trend, pair.Emojis[p.Trend], p.RSI, p.Side, pair.Emojis[p.Side],
	)

	// NOTE: may want to continue running instead of doing os.Exit()
	if _, err := bot.Send(buildMessage(text)); err != nil {
		fmt.Println("Crashed sending Telegram alert:", err)
		os.Exit(1)
	}
}
