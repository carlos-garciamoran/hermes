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

func NewBot(log zerolog.Logger) Bot {
	bot, err := tgbotapi.NewBotAPI(os.Getenv("TELEGRAM_APITOKEN"))
	if err != nil {
		log.Fatal().Str("err", err.Error()).Msg("Crashed creating Telegram bot")
	}

	chatID, err = strconv.ParseInt(os.Getenv("TELEGRAM_CHATID"), 10, 64)
	if err != nil {
		log.Fatal().Str("err", err.Error()).Msg("Error parsing TELEGRAM_CHATID")
	}

	return Bot{bot}
}

func (bot *Bot) SendInit(interval string, log zerolog.Logger, symbolCount int) {
	text := fmt.Sprintf(
		"🍾🍾 *NEW SESSION STARTED* 🍾🍾\n\n"+
			"    ⏱ interval: >*%s*<\n"+
			"    🪙 symbols: >*%d*<",
		interval, symbolCount,
	)

	if _, err := bot.Send(buildMessage(text)); err != nil {
		log.Fatal().Str("err", err.Error()).Msg("Crashed sending Telegram init")
	}
}

func (bot *Bot) SendAlert(log zerolog.Logger, a *analysis.Analysis) {
	text := fmt.Sprintf("🔔 %s\n\n"+
		"    — Price: *%.3f*\n"+
		"    — Trend: _%s_ %s\n"+
		"    — RSI: %.2f",
		a.Asset.BaseAsset, a.Price, a.Trend, analysis.Emojis[a.Trend], a.RSI,
	)

	// NOTE: may want to continue running instead of doing os.Exit()
	if _, err := bot.Send(buildMessage(text)); err != nil {
		log.Fatal().Str("err", err.Error()).Msg("Crashed sending Telegram alert")
	}
}

func (bot *Bot) SendSignal(log zerolog.Logger, a *analysis.Analysis) {
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
		"    — RSI: %.2f",
		a.Price, a.Trend, analysis.Emojis[a.Trend], a.RSI,
	)

	if a.Side != "NA" {
		text += fmt.Sprintf("\n\n "+
			"    🔮 Side: *%s* %s",
			a.Side, analysis.Emojis[a.Side],
		)
	}

	// NOTE: may want to continue running instead of doing os.Exit()
	if _, err := bot.Send(buildMessage(text)); err != nil {
		log.Fatal().Str("err", err.Error()).Msg("Crashed sending Telegram alert")
	}
}
