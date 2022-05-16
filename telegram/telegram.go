package telegram

import (
	"fmt"
	"os"
	"strconv"

	"hermes/account"
	"hermes/analysis"
	"hermes/position"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/rs/zerolog"
)

type Bot struct {
	*tgbotapi.BotAPI
	*zerolog.Logger
}

var chatID int64

func New(log *zerolog.Logger) Bot {
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

func (bot *Bot) Listen(acct *account.Account, symbolPrices map[string]float64) {
	updateConfig := tgbotapi.NewUpdate(0)
	updateConfig.Timeout = 30

	updates := bot.GetUpdatesChan(updateConfig)

	bot.Info().Int64("chatID", chatID).Msg("📡 Listening for commands")

	for update := range updates {
		message := update.Message

		if update.Message == nil { // Ignore any non-Message updates
			continue
		}

		chat := message.Chat

		if chat.ID != chatID { // Make it private: ignore messages not coming from chatID.
			bot.Error().
				Int64("ID", chat.ID).
				Str("UserName", chat.UserName).
				Msg("Unauthorised access")
			continue
		}

		if !message.IsCommand() { // Ignore any non-command Messages
			continue
		}

		bot.Info().Str("text", message.Text).Msg("Received command")

		// Extract the command.
		switch message.Command() {
		case "briefing":
			bot.reportBriefing(acct, symbolPrices, update)
		case "pnl":
			bot.reportNetPNL(acct.NetPNL, update)
		case "upnl":
			bot.reportUnrealizedPNL(acct, symbolPrices, update)
		}
	}
}

func (bot *Bot) SendInit(initialBalance float64, interval string, maxPositions int, simulatePositions bool) {
	bot.SendMessage(fmt.Sprintf(
		"🍾 *NEW SESSION STARTED* 🍾\n\n"+
			"    💰 initial balance: >*%.2f*<\n"+
			"    ⏱ interval: >*%s*<\n"+
			"    🔝 max positions: >*%d*<\n"+
			"    📟 simulate: >*%t*<\n",
		initialBalance, interval, maxPositions, simulatePositions,
	))
}

// TODO: set float precision based on p.Asset.PricePrecision
func (bot *Bot) SendAlert(a *analysis.Analysis, target float64) {
	bot.SendMessage(fmt.Sprintf(
		"🔔 *%s* crossed %.3f\n\n"+
			"    — Price: *%.3f*\n"+
			"    — Trend: _%s_ %s\n"+
			"    — RSI: %.2f",
		a.Asset.BaseAsset, target, a.Price, a.Trend, analysis.Emojis[a.Trend], a.RSI,
	))
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

	bot.SendMessage(text)
}

// IMPROVE: set float precision based on p.Asset.PricePrecision
func (bot *Bot) SendNewPosition(p *position.Position) {
	bot.SendMessage(fmt.Sprintf("💡 Opened *%s* | %s %s\n\n"+
		"    🖋 Entry @ %.3f\n"+
		"    🧨 SL: %f (%.2f%%)\n"+
		"    💎 TP: %f (%.2f%%)",
		p.Symbol, p.Side, analysis.Emojis[p.Side],
		p.EntryPrice,
		p.SL, position.SL*100,
		p.TP, position.TP*100,
	))
}

func (bot *Bot) SendClosedPosition(p *position.Position) {
	pnlEmoji := GetPNLEmoji(p.PNL)
	exitEmoji := map[string]string{"SL": "🧨", "TP": "💵"}[p.ExitSignal]

	bot.SendMessage(fmt.Sprintf("%s Closed *%s* | %s\n\n"+
		"    🖋 Exit @ %.3f\n"+
		"    %s *%s* hit\n"+
		"    💰 PNL: *$%.2f* (%.2f%%)",
		pnlEmoji, p.Symbol, analysis.Emojis[p.Side],
		p.ExitPrice,
		exitEmoji, p.ExitSignal,
		p.NetPNL, p.PNL*100,
	))
}

// TODO: abstract report* methods
func (bot *Bot) reportBriefing(acct *account.Account, symbolPrices map[string]float64, update tgbotapi.Update) {
	msg := tgbotapi.NewMessage(chatID, buildBriefingReport(acct, symbolPrices))
	msg.ParseMode = tgbotapi.ModeMarkdown
	msg.ReplyToMessageID = update.Message.MessageID // Reply to the previous message

	if _, err := bot.Send(msg); err != nil {
		bot.Error().Str("err", err.Error()).Msg("Could not send message")
	}
}

// TODO: abstract report* methods
func (bot *Bot) reportNetPNL(netPNL float64, update tgbotapi.Update) {
	msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Net PNL %s\n\n"+
		"    💵 *$%.2f*\n",
		GetPNLEmoji(netPNL), netPNL,
	))

	msg.ParseMode = tgbotapi.ModeMarkdown
	msg.ReplyToMessageID = update.Message.MessageID // Reply to the previous message

	if _, err := bot.Send(msg); err != nil {
		bot.Error().Str("err", err.Error()).Msg("Could not send message")
	}
}

func (bot *Bot) reportUnrealizedPNL(acct *account.Account, symbolPrices map[string]float64, update tgbotapi.Update) {
	msg := tgbotapi.NewMessage(chatID, buildPNLReport(acct, symbolPrices))
	msg.ParseMode = tgbotapi.ModeMarkdown
	msg.ReplyToMessageID = update.Message.MessageID // Reply to the previous message

	if _, err := bot.Send(msg); err != nil {
		bot.Error().Str("err", err.Error()).Msg("Could not send message")
	}
}

// TODO: improve formatting
func (bot *Bot) SendFinish(acct *account.Account, symbolPrices map[string]float64) {
	netPNL := acct.NetPNL

	bot.SendMessage(fmt.Sprintf("⛔️ *SESSION TERMINATED* ⛔️\n\n"+
		"Net PNL: *$%.2f* %s\n"+
		"%s",
		netPNL, GetPNLEmoji(netPNL), buildPNLReport(acct, symbolPrices),
	))
}

func buildBriefingReport(acct *account.Account, symbolPrices map[string]float64) string {
	pnls := acct.CalculateOpenPositionsPNLs(symbolPrices)
	positionsCount := len(pnls)

	if positionsCount >= 1 {
		report := fmt.Sprintf("📄 *Briefing* report (%d open positions) 📄\n\n", positionsCount)

		for symbol, pnlPair := range pnls {
			netPNL, rawPNL := pnlPair[0], pnlPair[1]
			emoji := GetPNLEmoji(rawPNL)

			report += fmt.Sprintf("*%s* %s\n"+
				"    💵 Net uPNL: *$%.2f*\n"+
				"    📐 Raw uPNL: *%.2f%%*\n\n",
				symbol, emoji, netPNL, rawPNL,
			)
		}

		return report
	}

	return "🧘‍♂️ No open positions to report"
}

func buildPNLReport(account *account.Account, symbolPrices map[string]float64) string {
	totalNetPNL, totalPNL := account.CalculateUnrealizedPNL(symbolPrices)

	return fmt.Sprintf("Unreal PNL %s\n\n"+
		"    💵 Net: *$%.2f*\n"+
		"    📐 Raw: *%.2f%%*",
		GetPNLEmoji(totalPNL), totalPNL, totalNetPNL,
	)
}

func (bot *Bot) SendMessage(text string) {
	message := tgbotapi.MessageConfig{
		BaseChat: tgbotapi.BaseChat{
			ChatID: chatID,
		},
		Text:      text,
		ParseMode: tgbotapi.ModeMarkdown,
	}

	// NOTE: may want to continue running instead of doing os.Exit()
	// TODO: handle err="Too Many Requests: retry after 39" without exiting
	if _, err := bot.Send(message); err != nil {
		fmt.Println(err)
		bot.Fatal().
			Str("err", err.Error()).
			Str("text", text).
			Msg("Crashed sending Telegram message")
	}
}

// TODO: turn function into map (keys being True and False)
func GetPNLEmoji(pnl float64) string {
	if pnl >= 0 {
		return "💸"
	}

	return "🤬"
}
