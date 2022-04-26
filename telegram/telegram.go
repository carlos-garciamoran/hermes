package telegram

import (
	"hermes/analysis"
	"hermes/position"

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

func (bot *Bot) SendMessage(text *string) {
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
		log.Fatal().Str("err", err.Error()).Msg("Crashed parsing TELEGRAM_CHATID")
	}

	return Bot{bot, log}
}

func (bot *Bot) Listen(log *zerolog.Logger, symbolPrices map[string]float64) {
	updateConfig := tgbotapi.NewUpdate(0)
	updateConfig.Timeout = 30

	updates := bot.GetUpdatesChan(updateConfig)

	log.Info().
		Int64("chatID", chatID).
		Msg("📡 Listening for commands")

	// Go through each Telegram update.
	for update := range updates {
		if update.Message == nil {
			continue
		}

		message := update.Message
		chat := message.Chat

		// Make it private: ignore messages not coming from chatID.
		if chat.ID != chatID {
			log.Error().
				Int64("chat.ID", chat.ID).
				Msg("Unauthorised access")
			continue
		}

		// Only respond to commands
		if len(message.Entities) == 1 && message.Entities[0].Type == "bot_command" {
			command := message.Text
			log.Debug().
				Str("command", command).
				Msg("Command received")

			if command == "/pnl" {
				emoji := "💸💸💸"
				totalPNL := position.CalculateTotalPNL(symbolPrices)
				if totalPNL < 0 {
					emoji = "⚰️⚰️⚰️"
				}

				resp := fmt.Sprintf("PNL: *$%.2f* %s", totalPNL, emoji)
				msg := tgbotapi.NewMessage(chatID, resp)
				msg.ReplyToMessageID = update.Message.MessageID // Reply to the previous message

				if _, err := bot.Send(msg); err != nil {
					log.Error().
						Str("err", err.Error()).
						Msg("Could not send message")
				}
			}
		}
	}
}

func (bot *Bot) SendInit(interval string, maxPositions int, simulatePositions bool, symbolCount int) {
	text := fmt.Sprintf(
		"🍾 *NEW SESSION STARTED* 🍾\n\n"+
			"    ⏱ interval: >*%s*<\n"+
			"    🔝 max positions: >*%d*<\n"+
			"    📟 simulate: >*%t*<\n"+
			"    🪙 symbols: >*%d*<",
		interval, maxPositions, simulatePositions, symbolCount,
	)

	bot.SendMessage(&text)
}

// TODO: set float precision based on p.Asset.PricePrecision
func (bot *Bot) SendAlert(a *analysis.Analysis, target float64) {
	text := fmt.Sprintf(
		"🔔 *%s* crossed %.3f\n\n"+
			"    — Price: *%.3f*\n"+
			"    — Trend: _%s_ %s\n"+
			"    — RSI: %.2f",
		a.Asset.BaseAsset, target, a.Price, a.Trend, analysis.Emojis[a.Trend], a.RSI,
	)

	bot.SendMessage(&text)
}

func (bot *Bot) SendSignal(a *analysis.Analysis) {
	text := fmt.Sprintf("⚡️ %s", a.Asset.BaseAsset)

	if a.EMA_Cross != "NA" {
		text += fmt.Sprintf(" | _%s 5/9 EMA cross_ %s", a.EMA_Cross, analysis.Emojis[a.EMA_Cross])
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

	bot.SendMessage(&text)
}

func (bot *Bot) SendPosition(p *position.Position) {
	text := fmt.Sprintf("💰 Opened *%s* position\n\n"+
		"    — Entry price: %.3f\n"+
		"    — Entry signal: %s\n"+
		"    — Side: *%s* %s\n"+
		"    — Size: $%.2f\n",
		p.Symbol, p.EntryPrice, p.EntrySignal, p.Side, analysis.Emojis[p.Side], p.Size,
	)

	bot.SendMessage(&text)
}

func (bot *Bot) SendFinish() {
	text := "⛔️ *SESSION ENDED* ⛔️"

	bot.SendMessage(&text)
}
