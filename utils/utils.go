package utils

import (
	"encoding/json"
	"flag"
	"fmt"
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

func InitLogging() zerolog.Logger {
	zerolog.TimeFieldFormat = time.RFC3339Nano // time.RFC3339, time.RFC822, zerolog.TimeFormatUnix
	output := zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339Nano}

	output.FormatLevel = func(i interface{}) string {
		return strings.ToUpper(fmt.Sprintf("|%-5s|", i))
	}

	return zerolog.New(output).With().Timestamp().Logger()
}

func ParseFlags(log zerolog.Logger) (bool, bool, string) {
	interval := flag.String("interval", "", "interval to perform TA: 1m, 3m, 5m, 15m, 30m, 1h, 4h, 1d")
	alertOnSignals := flag.Bool("signals", false, "send signal alerts on Telegram")
	tradeSignals := flag.Bool("trade", false, "trade signals on Binance USD-M account")

	flag.Parse()

	intervalIsValid := false
	validIntervals := []string{"1m", "3m", "5m", "15m", "30m", "1h", "4h", "1d"}
	for _, valid_interval := range validIntervals {
		if *interval == valid_interval {
			intervalIsValid = true
		}
	}

	if !intervalIsValid {
		log.Error().Msg("Please specify a valid interval")
		os.Exit(2)
	}

	return *alertOnSignals, *tradeSignals, *interval
}

func LoadAlerts(log zerolog.Logger) []Alert {
	dat, err := os.ReadFile("./alerts.json")
	if err != nil {
		log.Fatal().Msg(err.Error())
	}

	alerts := []Alert{}
	json.Unmarshal(dat, &alerts)

	return alerts
}

func LoadEnvFile(log zerolog.Logger) (string, string) {
	err := godotenv.Load()
	if err != nil {
		log.Fatal().Str("err", err.Error()).Msg("Crashed loading .env file")
	}

	return os.Getenv("BINANCE_APIKEY"), os.Getenv("BINANCE_SECRETKEY")
}
