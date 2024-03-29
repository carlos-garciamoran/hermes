package utils

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"hermes/account"
	"hermes/analysis"
	"hermes/exchange"
	"hermes/telegram"

	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
)

// InitLogging returns a customized zerolog.Logger instance writing to a .log file and os.Stdout.
func InitLogging() zerolog.Logger {
	t := time.Now()
	fileName := fmt.Sprintf("./session_%d-%02d-%02dT%02d:%02d:%02d.log",
		t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(),
	)

	logFile, err := os.Create(fileName)
	if err != nil {
		fmt.Println("Error: could not create the log file:", err)
		os.Exit(1)
	}

	timeFormat := "2006-01-02T15:04:05.9999"
	zerolog.TimeFieldFormat = timeFormat
	consoleOutput := zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: timeFormat}
	consoleOutput.FormatLevel = func(i interface{}) string {
		return strings.ToUpper(fmt.Sprintf("|%-5s|", i))
	}

	return zerolog.New(io.MultiWriter(consoleOutput, logFile)).With().Timestamp().Logger()
}

// ParseFlags parses the CLI flags, validates the interval passed, and returns pointers to their values.
func ParseFlags(log *zerolog.Logger) (float64, bool, string, int, bool, bool, bool) {
	balance := flag.Float64("balance", 1000, "initial balance to simulate trading (ignored when trade=true)")
	dev := flag.Bool("dev", true, "send alerts to development bot (DEV_TELEGRAM_* in .env)")
	interval := flag.String("interval", "", "interval to perform TA: 1m, 3m, 5m, 15m, 30m, 1h, 2h, 4h, 12h, 1d")
	maxPositions := flag.Int("max-positions", 4, "maximum positions to open")
	trackPositions := flag.Bool("positions", true, "open positions when signals are triggered (simulated by default)")
	isReal := flag.Bool("real", false, "open a real trade for every position on Binance USD-M")
	sendSignals := flag.Bool("signals", false, "send alerts on Telegram when a signal is triggered")

	flag.Parse()

	intervalIsValid := false
	validIntervals := []string{"1m", "3m", "5m", "15m", "30m", "1h", "2h", "4h", "12h", "1d"}
	for _, validInterval := range validIntervals {
		if *interval == validInterval {
			intervalIsValid = true
			break
		}
	}

	if !intervalIsValid {
		log.Error().Msg("Please specify a valid interval")
		os.Exit(2)
	}

	return *balance, *dev, *interval, *maxPositions, *trackPositions, *isReal, *sendSignals
}

// LoadAlerts parses the alerts.json file into a struct of type Alert.
func LoadAlerts(log *zerolog.Logger, interval string, validSymbols map[string]string) ([]analysis.Alert, []string) {
	var alertSymbols []string

	dat, err := os.ReadFile("./alerts.json")
	if err != nil {
		log.Fatal().Msg(err.Error())
	}

	alerts := []analysis.Alert{}
	json.Unmarshal(dat, &alerts)

	for i, alert := range alerts {
		symbol := alert.Symbol
		if !(validSymbols[symbol] == interval) {
			// Remove alert from array.
			alerts[i] = alerts[len(alerts)-1]
			alerts = alerts[:len(alerts)-1]
			alertSymbols = append(alertSymbols, symbol)
		}
	}

	return alerts, alertSymbols
}

// LoadEnvFile makes the variable in the .env file available via os.GetEnv() using godotenv.
func LoadEnvFile(log *zerolog.Logger) {
	err := godotenv.Load()
	if err != nil {
		log.Fatal().Str("err", err.Error()).Msg("Crashed loading .env file")
	}
}

func HandleCTRLC(
	acct *account.Account, bot *telegram.Bot, c chan os.Signal, excg *exchange.Exchange,
	isReal bool, log *zerolog.Logger, symbolPrices map[string]float64, usesTelegramBot bool,
) {
	for sig := range c {
		var wantsToExit string

		fmt.Print("Are you sure you want to exit? (y/N) ")
		fmt.Scanln(&wantsToExit)

		wantsToExit = strings.ToUpper(wantsToExit)

		if wantsToExit == "Y" || wantsToExit == "YES" {
			log.Warn().Str("sig", sig.String()).Msg("Received CTRL-C. Exiting...")

			if isReal {
				excg.CloseAllPositions(acct.OpenPositions)
			}

			if usesTelegramBot {
				bot.SendFinish(acct, symbolPrices)
			}

			log.Info().
				Float64("AllocatedBalance", acct.AllocatedBalance).
				Float64("AvailableBalance", acct.AvailableBalance).
				Float64("TotalBalance", acct.TotalBalance).
				Float64("NetPNL", acct.NetPNL).
				Float64("PNL", acct.PNL).
				Int("Loses", acct.Loses).
				Int("Wins", acct.Wins).
				Msg("📄")

			close(c)
			os.Exit(1)
		}
	}
}
