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
