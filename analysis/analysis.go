package analysis

import (
	"math"

	"hermes/utils"

	"github.com/markcheno/go-talib"
)

// Defines characteristics of an asset according to the exchange.
type Asset struct {
	BaseAsset         string  // Base of the asset (e.g., "BTC", "ETH")
	MaxQuantity       float64 // Maximum quantity allowed to trade.
	MinQuantity       float64 // Minimum quantity allowed to trade.
	PricePrecision    int     // Maximum number of decimals allowed in the order's price.
	QuantityPrecision int     // Maximum number of decimals allowed in the order's quantity.
	Symbol            string  // Representation of the asset. "<BASE><QUOTE>"
}

type Analysis struct {
	Asset       *Asset
	EMA_005     []float64 // Array for checking for cross.
	EMA_009     []float64 // Array for checking for cross.
	EMA_050     float64   // Latest average for reading the trend.
	EMA_200     float64   // Latest average for reading the trend.
	EMACross    string    // BULLISH, BULLISH_X2, BEARISH, BEARISH_X2.
	Price       float64
	RSI         float64 // Rounded to 2 digits.
	RSISignal   string  // RSI_[HOT|COLD]_L{1,3}.
	Side        string  // BUY, SELL.
	SignalCount uint
	Symbol      string // Could create a pointer to Asset.Symbol to save space (instead of copying).
	Trend       string // Based on EMA_050, EMA_200, and Price.
}

// Constant value for neutral signal (EMA_Cross, EMA_Trend and RSI_Signal).
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

// Constant values for EMACross and Trend.
const (
	BULLISH    = "bullish"
	BULLISH_X2 = "bullish-X2"

	BEARISH    = "bearish"
	BEARISH_X2 = "bearish-X2"
)

var Emojis = map[string]string{
	BUY:           "🚀",
	SELL:          "⬇️",
	BULLISH:       "🐗",
	BULLISH_X2:    "🐗🐗",
	BEARISH:       "🐻",
	BEARISH_X2:    "🐻🐻",
	OVERBOUGHT:    "📈",
	OVERBOUGHT_X2: "📈📈",
	OVERBOUGHT_X3: "📈📈📈",
	OVERSOLD:      "📉",
	OVERSOLD_X2:   "📉📉",
	OVERSOLD_X3:   "📉📉📉",
}

func New(asset *Asset, closes []float64, lastIndex int) Analysis {
	a := Analysis{
		Asset:       asset,
		EMA_005:     talib.Ema(closes, 5)[lastIndex-2:],
		EMA_009:     talib.Ema(closes, 9)[lastIndex-2:],
		EMA_050:     talib.Ema(closes, 50)[lastIndex],
		EMA_200:     talib.Ema(closes, 200)[lastIndex],
		EMACross:    NA,
		Price:       closes[lastIndex],
		RSI:         math.Round(talib.Rsi(closes, 14)[lastIndex]*100) / 100,
		SignalCount: 0,
		Side:        NA,
		Symbol:      asset.Symbol,
	}

	a.calculateEMACross()

	a.Trend = a.calculateTrend()

	a.RSISignal = a.evaluateRSI()

	if a.RSISignal != "NA" {
		a.SignalCount += 1
	}

	a.chooseSide()

	return a
}

func (a *Analysis) TriggersAlert(alerts *[]utils.Alert) (bool, float64) {
	price := a.Price

	// HACK: use a pre-built symbol map (of alerts) to improve performance: O(1) beats O(n)
	for i, alert := range *alerts {
		if alert.Symbol == a.Symbol && !alert.Notified && alert.Type == "price" {
			targetPrice := alert.Price
			triggersAlert := alert.Condition == ">=" && price >= targetPrice ||
				alert.Condition == "<=" && price <= targetPrice ||
				alert.Condition == "<" && price < targetPrice ||
				alert.Condition == ">" && price > targetPrice

			if triggersAlert {
				(*alerts)[i].Notified = true
				return true, targetPrice
			}
		}
	}

	return false, 0
}

func (a *Analysis) TriggersSignal(sentSignals map[string]string) bool {
	// Only trade or send alert if there's a signal, a side, and no alert has been sent.
	if a.SignalCount >= 1 && a.Side != NA && sentSignals[a.Symbol] != a.Side {
		return true
	}

	return false
}

// TODO: check for EMA200[x]close cross (reversal signal)
// TODO: check for EMA cross between 10 & 50
func (a *Analysis) calculateEMACross() {
	var delta [3]int
	sum := 0

	// IDEA: add some margin between EMAs (e.g., distance min >= 0.10%)
	for i := 0; i < 3; i++ {
		if a.EMA_005[i] < a.EMA_009[i] {
			delta[i] = -1
		} else {
			delta[i] = 1
		}
		sum += delta[i]
	}

	// If all deltas are the same ([1,1,1] or [-1,-1,-1]), there can be no cross.
	if sum%3 != 0 {
		// Check for the cross on the last candle.
		if delta[2] == 1 {
			a.EMACross = BULLISH
			a.SignalCount += 1
		} else if delta[2] == -1 {
			a.EMACross = BEARISH
			a.SignalCount += 1
		}
	}
}

// TODO: allow some margin to evaluation (< 0.15% distance to EMA should be neutral)
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
	// NOTE: REMEMBER EMAs are LAGGING INDICATORS: they should be used as CONFIRMATION
	// NOTE: may want to check RSI for confirmation/discard

	if a.Price < a.EMA_200 && a.EMACross == BULLISH {
		// Buy the undervalued asset gaining bullish momentum.
		a.Side = BUY
	} else if a.Price > a.EMA_200 && a.EMACross == BEARISH {
		// Sell the overvalued asset gaining bearish momentum.
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
