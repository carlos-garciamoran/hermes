package utils

import (
	"encoding/json"
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
