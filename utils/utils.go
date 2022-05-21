package utils

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
)

type Alert struct {
	Condition string
	Notified  bool
	Price     float64
	Symbol    string
	Type      string
}

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
func ParseFlags(log zerolog.Logger) (float64, string, int, bool, bool, bool, bool) {
	balance := flag.Float64("balance", 1000, "initial balance to simulate trading (ignored when trade=true)")
	interval := flag.String("interval", "", "interval to perform TA: 1m, 3m, 5m, 15m, 30m, 1h, 2h, 4h, 1d")
	maxPositions := flag.Int("max-positions", 5, "maximum positions to open")
	notifyOnSignals := flag.Bool("signals", false, "send signal alerts on Telegram")
	simulateTrades := flag.Bool("simulate", true, "simulate opening trades when signals are triggered")
	onDev := flag.Bool("dev", true, "send alerts to development bot (DEV_TELEGRAM_* in .env)")
	tradeSignals := flag.Bool("trade", false, "trade signals on Binance USD-M account")

	flag.Parse()

	intervalIsValid := false
	validIntervals := []string{"1m", "3m", "5m", "15m", "30m", "1h", "2h", "4h", "1d"}
	for _, valid_interval := range validIntervals {
		if *interval == valid_interval {
			intervalIsValid = true
			break
		}
	}

	if !intervalIsValid {
		log.Error().Msg("Please specify a valid interval")
		os.Exit(2)
	}

	return *balance, *interval, *maxPositions, *notifyOnSignals, *simulateTrades, *onDev, *tradeSignals
}

// LoadAlerts parses the alerts.json file into a struct of type Alert.
func LoadAlerts(log zerolog.Logger, interval string, validSymbols map[string]string) ([]Alert, []string) {
	var alertSymbols []string

	dat, err := os.ReadFile("./alerts.json")
	if err != nil {
		log.Fatal().Msg(err.Error())
	}

	alerts := []Alert{}
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
func LoadEnvFile(log zerolog.Logger) {
	err := godotenv.Load()
	if err != nil {
		log.Fatal().Str("err", err.Error()).Msg("Crashed loading .env file")
	}
}
