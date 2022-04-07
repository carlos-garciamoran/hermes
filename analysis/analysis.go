package analysis

import (
	// "hermes/telegram"
	// "hermes/utils"

	"hermes/utils"
	"math"

	"github.com/markcheno/go-talib"
	"github.com/rs/zerolog"
)

type Asset struct {
	BaseAsset         string
	MaxQuantity       float64
	MinQuantity       float64
	PricePrecision    int
	QuantityPrecision int
	Symbol            string
}

type Analysis struct {
	Asset        *Asset    // Pointer to the Asset.
	EMA_005      []float64 // Array to check for cross.
	EMA_009      []float64 // Array to check for cross.
	EMA_050      float64   // Current average to read trend.
	EMA_200      float64   // Current average to read trend.
	EMA_Cross    string
	Price        float64
	RSI          float64 // Rounded to 2 digits.
	RSI_Signal   string
	Side         string
	Signal_Count uint
	Symbol       string
	Trend        string // Based on EMA_050, EMA_200, and Price.
}

// Constant value for neutral signal (EMA_Trend and RSI_Signal).
const (
	NA = "NA"
)

// Constant values for RSI triggers.
const (
	RSI_HOT_L1 = 69.9
	RSI_HOT_L2 = 79.9
	RSI_HOT_L3 = 89.9

	RSI_COLD_L1 = 30.1
	RSI_COLD_L2 = 20.1
	RSI_COLD_L3 = 10.1
)

// Constant values for RSI_Signal.
const (
	OVERBOUGHT    = "overbought"
	OVERBOUGHT_X2 = "overbought-X2"
	OVERBOUGHT_X3 = "overbought-X3"

	OVERSOLD    = "oversold"
	OVERSOLD_X2 = "oversold-X2"
	OVERSOLD_X3 = "oversold-X3"
)

// Constant values for Side.
const (
	BUY  = "BUY"
	SELL = "SELL"
)

// Constant values for EMA_Cross and Trend.
const (
	BULLISH    = "bullish"
	BULLISH_X2 = "bullish-X2"

	BEARISH    = "bearish"
	BEARISH_X2 = "bearish-X2"
)

var Emojis = map[string]string{
	BUY:  "🚀",
	SELL: "⬇️",

	BULLISH:    "🐗",
	BULLISH_X2: "🐗🐗",

	BEARISH:    "🐻",
	BEARISH_X2: "🐻🐻",

	OVERBOUGHT:    "📈",
	OVERBOUGHT_X2: "📈📈",
	OVERBOUGHT_X3: "📈📈📈",

	OVERSOLD:    "📉",
	OVERSOLD_X2: "📉📉",
	OVERSOLD_X3: "📉📉📉",
}

func New(asset *Asset, closes []float64, lastCloseIndex int) Analysis {
	a := Analysis{
		Asset:        asset,
		EMA_005:      talib.Ema(closes, 5)[lastCloseIndex-2:],
		EMA_009:      talib.Ema(closes, 9)[lastCloseIndex-2:],
		EMA_050:      talib.Ema(closes, 50)[lastCloseIndex],
		EMA_200:      talib.Ema(closes, 200)[lastCloseIndex],
		Price:        closes[lastCloseIndex],
		RSI:          math.Round(talib.Rsi(closes, 14)[lastCloseIndex]*100) / 100,
		Signal_Count: 0,
		Side:         NA,
		Symbol:       asset.Symbol,
	}

	// TODO: REMEMBER EMAs are LAGGING INDICATORS: they should be used as CONFIRMATION
	// TODO: check for EMA200[x]close cross (reversal signal)

	a.calculateEMACross()

	if a.EMA_Cross != "NA" {
		a.Signal_Count += 1
	}

	a.Trend = a.calculateTrend()

	a.RSI_Signal = a.evaluateRSI()

	if a.RSI_Signal != "NA" {
		a.Signal_Count += 1
	}

	a.chooseSide()

	return a
}

func (a *Analysis) TriggersAlert(alerts *[]utils.Alert, log zerolog.Logger) bool {
	price, symbol := a.Price, a.Symbol

	// HACK: using a pre-built symbol map (of alerts) may improve performance: O(1) beats O(n)
	for i, alert := range *alerts {
		if alert.Symbol == symbol && !alert.Notified && alert.Type == "price" {
			triggersAlert := alert.Condition == ">=" && price >= alert.Price ||
				alert.Condition == "<=" && price <= alert.Price ||
				alert.Condition == "<" && price < alert.Price ||
				alert.Condition == ">" && price > alert.Price

			if triggersAlert {
				(*alerts)[i].Notified = true
				return true
			}
		}
	}

	return false
}

func (a *Analysis) TriggersSignal(log zerolog.Logger, sentAlerts *map[string]string) bool {
	// Only trade or send alert if there's a signal, a side, and no alert has been sent.
	if a.Signal_Count >= 1 && a.Side != NA && (*sentAlerts)[a.Symbol] != a.Side {
		log.Info().
			Str("EMA_Cross", a.EMA_Cross).
			Float64("Price", a.Price).
			Float64("RSI", a.RSI).
			Str("RSI_Signal", a.RSI_Signal).
			Uint("Signal_Count", a.Signal_Count).
			Str("Trend", a.Trend).
			Str("Side", a.Side).
			Str("Symbol", a.Asset.BaseAsset).
			Msg("⚡")

		return true
	}

	return false
}

func (a *Analysis) calculateEMACross() {
	// TODO: check for EMA cross between 10 & 50

	var cross string = NA
	var delta [3]int
	var sum int

	for i := 0; i < 3; i++ {
		if a.EMA_005[i] < a.EMA_009[i] {
			delta[i] = -1
		} else {
			delta[i] = 1
		}
	}

	for _, v := range delta {
		sum += v
	}

	// If all deltas are the same ([1,1,1] or [-1,-1,-1]), there can be no cross.
	if sum%3 != 0 {
		// Check the cross on the last candle.
		if delta[2] == 1 {
			cross = BULLISH
		} else if delta[2] == -1 {
			cross = BEARISH
		}
	}

	a.EMA_Cross = cross
}

// TODO: give margin to evaluation (< 0.15% distance to EMA should be neutral)
func (a *Analysis) calculateTrend() string {
	if a.Price >= a.EMA_050 && a.Price >= a.EMA_200 {
		return BULLISH_X2
	}

	if a.Price >= a.EMA_050 || a.Price >= a.EMA_200 {
		return BULLISH
	}

	if a.Price < a.EMA_050 && a.Price < a.EMA_200 {
		return BEARISH_X2
	}

	if a.Price < a.EMA_050 || a.Price < a.EMA_200 {
		return BEARISH
	}

	return NA
}

func (a *Analysis) chooseSide() {
	// NOTE: may want to check RSI for confirmation/discard
	if a.Price < a.EMA_200 && a.EMA_Cross == BULLISH {
		a.Side = BUY
	} else if a.Price > a.EMA_200 && a.EMA_Cross == BEARISH {
		a.Side = SELL
	}
}

func (a *Analysis) evaluateRSI() string {
	if a.RSI >= RSI_HOT_L3 {
		return OVERBOUGHT_X3
	}

	if a.RSI >= RSI_HOT_L2 {
		return OVERBOUGHT_X2
	}

	if a.RSI >= RSI_HOT_L1 {
		return OVERBOUGHT
	}

	if a.RSI <= RSI_COLD_L3 {
		return OVERSOLD_X3
	}

	if a.RSI <= RSI_COLD_L2 {
		return OVERSOLD_X2
	}

	if a.RSI <= RSI_COLD_L1 {
		return OVERSOLD_X2
	}

	return NA
}
