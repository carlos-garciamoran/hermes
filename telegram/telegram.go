package telegram

import (
	"fmt"
	"hermes/pair"
	"os"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/rs/zerolog"
)

type TelegramBot struct {
	*tgbotapi.BotAPI
}

var chatID int64

func buildMessage(text string) tgbotapi.MessageConfig {
	return tgbotapi.MessageConfig{
		BaseChat: tgbotapi.BaseChat{
			ChatID: chatID,
		},
		Text:      text,
		ParseMode: tgbotapi.ModeMarkdown,
	}
}

func NewTelegramBot(log zerolog.Logger) *tgbotapi.BotAPI {
	bot, err := tgbotapi.NewBotAPI(os.Getenv("TELEGRAM_APITOKEN"))

	if err != nil {
		log.Fatal().Str("err", err.Error()).Msg("Crashed creating Telegram bot")
	}

	return bot
}

func (bot *TelegramBot) SendTelegramInit(interval string, symbol_count int) {
	text := "🔔🔔 *NEW SESSION STARTED* 🔔🔔\n\n" +
		fmt.Sprintf("    ⏱ interval: >>>*%s*<<<\n", interval) +
		fmt.Sprintf("    🪙 symbols: >>>*%d*<<<", symbol_count)

	if _, err := bot.Send(buildMessage(text)); err != nil {
		fmt.Println("Crashed sending Telegram init:", err)
		os.Exit(1)
	}
}

func (bot *TelegramBot) SendTelegramAlert(p *pair.Pair) {
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
