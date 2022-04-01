package utils

import (
	"encoding/json"
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

type alert struct {
	Type      string
	Price     int
	Condition string
	Symbol    string
}

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

func LoadAlerts(log zerolog.Logger) []alert {
	dat, err := os.ReadFile("./alerts.json")
	if err != nil {
		log.Fatal().Msg(err.Error())
	}

	alerts := []alert{}
	json.Unmarshal(dat, &alerts)

	return alerts
}

func LoadEnvFile(log zerolog.Logger) (string, string) {
	// TODO: remove this declaration in favor of :=
	var err error

	err = godotenv.Load()
	if err != nil {
		log.Fatal().Str("err", err.Error()).Msg("Crashed loading .env file")
	}

	chatID, err = strconv.ParseInt(os.Getenv("TELEGRAM_CHATID"), 10, 64)
	if err != nil {
		log.Fatal().Str("err", err.Error()).Msg("Error parsing TELEGRAM_CHATID")
	}

	return os.Getenv("BINANCE_APIKEY"), os.Getenv("BINANCE_SECRETKEY")
}

func NewTelegramBot(log zerolog.Logger) {
	var err error

	bot, err = tgbotapi.NewBotAPI(os.Getenv("TELEGRAM_APITOKEN"))

	if err != nil {
		log.Fatal().Str("err", err.Error()).Msg("Crashed creating Telegram bot")
	}
}

func SendTelegramInit(interval string, log zerolog.Logger, symbol_count int) {
	text := "🔔🔔 *NEW SESSION STARTED* 🔔🔔\n\n" +
		fmt.Sprintf("    ⏱ interval: >>>*%s*<<<\n", interval) +
		fmt.Sprintf("    🪙 symbols: >>>*%d*<<<", symbol_count)

	if _, err := bot.Send(buildMessage(text)); err != nil {
		log.Fatal().Str("err", err.Error()).Msg("Crashed sending Telegram init")
	}
}

func SendTelegramAlert(log zerolog.Logger, p *pair.Pair) {
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
		log.Fatal().Str("err", err.Error()).Msg("Crashed sending Telegram alert")
	}
}
