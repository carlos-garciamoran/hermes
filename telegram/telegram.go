package telegram

import (
	"hermes/analysis"

	"fmt"
	"os"
	"strconv"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/rs/zerolog"
)

type Bot struct {
	*tgbotapi.BotAPI
	*zerolog.Logger
}

var chatID int64

func (bot *Bot) sendMessage(text *string) {
	message := tgbotapi.MessageConfig{
		BaseChat: tgbotapi.BaseChat{
			ChatID: chatID,
		},
		Text:      *text,
		ParseMode: tgbotapi.ModeMarkdown,
	}

	// NOTE: may want to continue running instead of doing os.Exit()
	if _, err := bot.Send(message); err != nil {
		bot.Fatal().
			Str("err", err.Error()).
			Str("text", *text).
			Msg("Crashed sending Telegram message")
	}
}

func NewBot(log *zerolog.Logger) Bot {
	bot, err := tgbotapi.NewBotAPI(os.Getenv("TELEGRAM_APITOKEN"))
	if err != nil {
		log.Fatal().Str("err", err.Error()).Msg("Crashed creating Telegram bot")
	}

	chatID, err = strconv.ParseInt(os.Getenv("TELEGRAM_CHATID"), 10, 64)
	if err != nil {
		log.Fatal().Str("err", err.Error()).Msg("Error parsing TELEGRAM_CHATID")
	}

	return Bot{bot, log}
}

func (bot *Bot) SendInit(interval string, symbolCount int) {
	text := fmt.Sprintf(
		"🍾 *NEW SESSION STARTED* 🍾\n\n"+
			"    ⏱ interval: >*%s*<\n"+
			"    🪙 symbols: >*%d*<",
		interval, symbolCount,
	)

	bot.sendMessage(&text)
}

// TODO: set float precision based on p.Asset.PricePrecision
func (bot *Bot) SendAlert(a *analysis.Analysis, target float64) {
	text := fmt.Sprintf("🔔 *%s* crossed %.3f\n\n"+
		"    — Price: *%.3f*\n"+
		"    — Trend: _%s_ %s\n"+
		"    — RSI: %.2f",
		a.Asset.BaseAsset, target, a.Price, a.Trend, analysis.Emojis[a.Trend], a.RSI,
	)

	bot.sendMessage(&text)
}

func (bot *Bot) SendSignal(a *analysis.Analysis) {
	text := fmt.Sprintf("⚡️ %s", a.Asset.BaseAsset)

	if a.EMA_Cross != "NA" {
		text += fmt.Sprintf(" | _%s EMA cross_ %s", a.EMA_Cross, analysis.Emojis[a.EMA_Cross])
	}

	if a.RSI_Signal != "NA" {
		text += fmt.Sprintf(" | _RSI %s_ %s", a.RSI_Signal, analysis.Emojis[a.RSI_Signal])
	}

	// TODO: set float precision based on p.Asset.PricePrecision
	text += fmt.Sprintf("\n"+
		"    — Price: %.3f\n"+
		"    — Trend: _%s_ %s\n"+
		"    — RSI: %.2f\n\n"+
		"    🔮 Side: *%s* %s",
		a.Price, a.Trend, analysis.Emojis[a.Trend], a.RSI, a.Side, analysis.Emojis[a.Side],
	)

	bot.sendMessage(&text)
}

func (bot *Bot) SendFinish() {
	text := "⛔️ *SESSION ENDED* ⛔️"

	bot.sendMessage(&text)
}
