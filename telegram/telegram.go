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

func New(log *zerolog.Logger, onDev bool) Bot {
	prefix := "PROD_"
	if onDev {
		prefix = "DEV_"
	}

	bot, err := tgbotapi.NewBotAPI(os.Getenv(prefix + "TELEGRAM_APITOKEN"))
	if err != nil {
		log.Fatal().Str("err", err.Error()).Msg("Crashed creating Telegram bot")
	}

	chatID, err = strconv.ParseInt(os.Getenv(prefix+"TELEGRAM_CHATID"), 10, 64)
	if err != nil {
		log.Fatal().Str("err", err.Error()).Msg("Crashed parsing " + prefix + "TELEGRAM_CHATID")
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

		if chat.ID != chatID { // Make it private: ignore messages not coming from our chatID.
			bot.Error().
				Int64("ID", chat.ID).
				Str("UserName", chat.UserName).
				Msg("⛔️ Unauthorised access")
			continue
		}

		if !message.IsCommand() { // Ignore any non-command Messages
			continue
		}

		bot.Info().Str("text", message.Text).Str("UserName", chat.UserName).Msg("📡 Got command")

		switch message.Command() {
		case "account":
			bot.reportAccount(acct, symbolPrices, update)
		case "pnl":
			bot.reportNetPNL(acct, update)
		case "positions":
			bot.reportOpenPositions(acct, symbolPrices, update)
		case "upnl":
			bot.reportUnrealizedPNL(acct, symbolPrices, update)
		}
	}
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
		bot.Fatal().
			Str("err", err.Error()).
			Str("text", text).
			Msg("Crashed sending Telegram message")
	}
}

func (bot *Bot) SendInit(initialBalance float64, interval string, maxPositions int, simulatePositions bool) {
	bot.SendMessage(fmt.Sprintf(
		"🍾 *NEW SESSION STARTED* 🍾\n\n"+
			"    💰 initial balance: >*$%.2f*<\n"+
			"    ⏱ interval: >*%s*<\n"+
			"    🔝 max positions: >*%d*<\n"+
			"    📟 simulate: >*%t*<\n",
		initialBalance, interval, maxPositions, simulatePositions,
	))
}

// IMPROVE: set float precision based on p.Asset.PricePrecision
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

	if a.EMACross != "NA" {
		text += fmt.Sprintf(" | _%s 5/9 EMA cross_ %s", a.EMACross, analysis.Emojis[a.EMACross])
	}

	if a.RSISignal != "NA" {
		text += fmt.Sprintf(" | _RSI %s_ %s", a.RSISignal, analysis.Emojis[a.RSISignal])
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
		"    🖋 Entry @ %.3f with $%.2f\n"+
		"    🧨 SL: %f (%.2f%%)\n"+
		"    💎 TP: %f (%.2f%%)",
		p.Symbol, p.Side, analysis.Emojis[p.Side],
		p.EntryPrice, p.Size,
		p.SL, position.SL*100,
		p.TP, position.TP*100,
	))
}

func (bot *Bot) SendClosedPosition(p *position.Position) {
	pnlEmoji := GetPNLEmoji(p.PNL)
	exitEmoji := map[string]string{"SL": "🧨", "TP": "💎"}[p.ExitSignal]

	bot.SendMessage(fmt.Sprintf("%s Closed *%s* | %s\n\n"+
		"    🖋 Exit @ %.3f with $%.2f\n"+
		"    %s *%s* hit\n"+
		"    💰 PNL: *$%.2f* (%.2f%%)",
		pnlEmoji, p.Symbol, analysis.Emojis[p.Side],
		p.ExitPrice, p.Size,
		exitEmoji, p.ExitSignal,
		p.NetPNL, p.PNL,
	))
}

// TODO: report account info (extract content from reportAccount)
func (bot *Bot) SendFinish(acct *account.Account, symbolPrices map[string]float64) {
	bot.SendMessage(fmt.Sprintf("‼️ *SESSION TERMINATED* ‼️\n\n"+
		"    %s\n"+
		"    %s",
		buildNetPNLReport(acct),
		buildUnrealPNLReport(acct, symbolPrices),
	))
}

func (bot *Bot) report(content string, update tgbotapi.Update) {
	msg := tgbotapi.NewMessage(chatID, content)
	msg.ParseMode = tgbotapi.ModeMarkdown
	msg.ReplyToMessageID = update.Message.MessageID // Reply to the previous message

	if _, err := bot.Send(msg); err != nil {
		bot.Error().Str("err", err.Error()).Msg("Could not send message")
	}
}

func (bot *Bot) reportAccount(acct *account.Account, symbolPrices map[string]float64, update tgbotapi.Update) {
	totalTrades := len(acct.ClosedPositions)

	content := fmt.Sprintf(
		"🪙 Allocated balance: *$%.2f*\n"+
			"💰 Available balance: $%.2f\n"+
			"🖋 Initial balance: $%.2f\n"+
			"%s\n"+
			"%s\n"+
			"💡 Open positions: %d\n"+
			"🐸 Losing trades: *%d*/%d\n"+
			"🎉 Winning trades: *%d*/%d",
		acct.AllocatedBalance, acct.AvailableBalance, acct.InitialBalance,
		buildNetPNLReport(acct),
		buildUnrealPNLReport(acct, symbolPrices),
		len(acct.OpenPositions),
		acct.Loses, totalTrades, acct.Wins, totalTrades,
	)

	bot.report(content, update)
}

func (bot *Bot) reportNetPNL(acct *account.Account, update tgbotapi.Update) {
	bot.report(buildNetPNLReport(acct), update)
}

func (bot *Bot) reportOpenPositions(acct *account.Account, symbolPrices map[string]float64, update tgbotapi.Update) {
	content := "🧘‍♂️ No open positions to report"
	unrealizedPNLs := acct.CalculateOpenPositionsPNLs(symbolPrices)
	openPositionsCount := len(unrealizedPNLs)

	if openPositionsCount >= 1 {
		content = fmt.Sprintf("📄 Got %d open positions\n\n", openPositionsCount)

		for symbol, pnlPair := range unrealizedPNLs {
			unrealizedPNL, rawPNL := pnlPair[0], pnlPair[1]

			content += fmt.Sprintf(
				"    %s %s: *$%.2f* (%.2f%%)\n",
				GetPNLEmoji(unrealizedPNL), symbol, unrealizedPNL, rawPNL,
			)
		}
	}

	bot.report(content, update)
}

func (bot *Bot) reportUnrealizedPNL(acct *account.Account, symbolPrices map[string]float64, update tgbotapi.Update) {
	bot.report(buildUnrealPNLReport(acct, symbolPrices), update)
}

func buildNetPNLReport(acct *account.Account) string {
	return fmt.Sprintf(
		"%s Net PNL: *$%.2f* (%.2f%%)",
		GetPNLEmoji(acct.NetPNL), acct.NetPNL, acct.PNL,
	)
}

func buildUnrealPNLReport(acct *account.Account, symbolPrices map[string]float64) string {
	unrealizedPNL, rawPNL := acct.CalculateUnrealizedPNL(symbolPrices)

	return fmt.Sprintf(
		"%s Unrealized PNL: *$%.2f* (%.2f%%)",
		GetPNLEmoji(unrealizedPNL), unrealizedPNL, rawPNL,
	)
}

// TODO: turn function into map (keys being True and False)
func GetPNLEmoji(pnl float64) string {
	if pnl >= 0 {
		return "💸"
	}

	return "🤬"
}
