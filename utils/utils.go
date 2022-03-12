package utils

import (
	"hermes/pair"
	"os"
	"strconv"

	"fmt"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/joho/godotenv"
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
	text := "🔔🔔🔔 *NEW SESSION STARTED* 🔔🔔🔔\n\n" +
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
		text += fmt.Sprintf(" | *%s EMA cross* %s", p.EMA_Cross, pair.Emojis[p.EMA_Cross])
	}

	if p.RSI_Signal != "NA" {
		text += fmt.Sprintf(" | *RSI %s* %s", p.RSI_Signal, pair.Emojis[p.RSI_Signal])
	}

	text += fmt.Sprintf("\n"+
		"    — Trend: _%s_ %s\n"+
		"    — RSI: %.2f",
		p.Trend, pair.Emojis[p.Trend], p.RSI,
	)

	// NOTE: may want to continue running instead of doing os.Exit()
	if _, err := bot.Send(buildMessage(text)); err != nil {
		fmt.Println("Crashed sending Telegram alert:", err)
		os.Exit(1)
	}
}
