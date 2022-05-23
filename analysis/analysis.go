package analysis

import (
	"math"

	"hermes/utils"

	"github.com/markcheno/go-talib"
)

// Defines characteristics of a asset according to the exchange.
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
	Price       float64   // Price of the asset at the time of analysis.
	RSI         float64   // Relative Strength Index Rounded to 2 digits.
	RSISignal   string    // RSI_[HOT|COLD]_L{1,3}.
	Side        string    // BUY, SELL.
	SignalCount uint      // Count of trading signals found in the analysis.
	Symbol      string    // Could create a pointer to Asset.Symbol to save space (instead of copying).
	Trend       string    // Based on EMA_050, EMA_200, and Price.
}

// Value for neutral signal (EMA_Cross, EMA_Trend and RSI_Signal).
const NA = "NA"

// Values for EMACross and Trend.
const (
	BULLISH    = "bullish"
	BULLISH_X2 = "bullish-X2"
	BEARISH    = "bearish"
	BEARISH_X2 = "bearish-X2"
)

// Values for RSI triggers.
const (
	RSI_HOT_L1 = 69.9
	RSI_HOT_L2 = 79.9
	RSI_HOT_L3 = 89.9

	RSI_COLD_L1 = 30.1
	RSI_COLD_L2 = 20.1
	RSI_COLD_L3 = 10.1
)

// Values for RSISignal.
const (
	OVERBOUGHT    = "overbought"
	OVERBOUGHT_X2 = "overbought-X2"
	OVERBOUGHT_X3 = "overbought-X3"
	OVERSOLD      = "oversold"
	OVERSOLD_X2   = "oversold-X2"
	OVERSOLD_X3   = "oversold-X3"
)

// Values for Side.
const (
	BUY  = "BUY"
	SELL = "SELL"
)

// NOTE: may want to move to telegram.go, since that is where this map is used.
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

// New...
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

// TriggersAlert...
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

// TriggersSignal returns true if the analysis has found a signal, a side, and no signal has been triggered.
func (a *Analysis) TriggersSignal(triggeredSignals map[string]string) bool {
	if a.SignalCount >= 1 && a.Side != NA && triggeredSignals[a.Symbol] != a.Side {
		return true
	}

	return false
}

// TODO: check for EMA200[x]close cross (reversal signal)
// TODO: check for EMA cross between 10 & 50
// calculateEMACross determines if there is a cross of EMAs 5 and 9, and if so, what is its type.
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

// calculateTrend determines the trend based on the Price and its positioning with EMA_050 and EMA_200.
func (a *Analysis) calculateTrend() string {
	// TODO: allow some margin to evaluation (< 0.15% distance to EMA should be neutral)
	switch {
	case a.Price >= a.EMA_050 && a.Price >= a.EMA_200:
		return BULLISH_X2
	case a.Price >= a.EMA_050 || a.Price >= a.EMA_200:
		return BULLISH
	case a.Price < a.EMA_050 && a.Price < a.EMA_200:
		return BEARISH_X2
	case a.Price < a.EMA_050 || a.Price < a.EMA_200:
		return BEARISH
	}

	return NA
}

// chooseSide sets the Side field based on the Price and EMA_200 relation and the EMACross type.
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

// evaluateRSI returns a reading of the RSI (overbought/oversold) based on the defined RSI constants.
func (a *Analysis) evaluateRSI() string {
	switch {
	case a.RSI >= RSI_HOT_L3:
		return OVERBOUGHT_X3
	case a.RSI >= RSI_HOT_L2:
		return OVERBOUGHT_X2
	case a.RSI >= RSI_HOT_L1:
		return OVERBOUGHT
	case a.RSI <= RSI_COLD_L3:
		return OVERSOLD_X3
	case a.RSI <= RSI_COLD_L2:
		return OVERSOLD_X2
	case a.RSI <= RSI_COLD_L1:
		return OVERSOLD_X2
	}

	return NA
}
