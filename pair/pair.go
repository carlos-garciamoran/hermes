package pair

import (
	"math"

	"github.com/markcheno/go-talib"
)

type Pair struct {
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

// ⬆️, ⬇️
var Emojis = map[string]string{
	BUY:  "⬆️🚀",
	SELL: "⬇️💣",

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

func New(closes []float64, lastCloseIndex int, symbol string) Pair {
	p := Pair{
		EMA_005:      talib.Ema(closes, 5)[lastCloseIndex-2:],
		EMA_009:      talib.Ema(closes, 9)[lastCloseIndex-2:],
		EMA_050:      talib.Ema(closes, 50)[lastCloseIndex],
		EMA_200:      talib.Ema(closes, 200)[lastCloseIndex],
		Price:        closes[lastCloseIndex],
		RSI:          math.Round(talib.Rsi(closes, 14)[lastCloseIndex]*100) / 100,
		Signal_Count: 0,
		Side:         NA,
		Symbol:       symbol[:len(symbol)-4], // Trim "USDT" suffix
	}

	// TODO: REMEMBER EMAs are LAGGING INDICATORS: they should be used as CONFIRMATION
	// TODO: check for EMA200[x]close cross (reversal signal)

	p.calculateEMACross()

	if p.EMA_Cross != "NA" {
		p.Signal_Count += 1
	}

	p.Trend = p.calculateTrend()

	p.RSI_Signal = p.evaluateRSI()

	if p.RSI_Signal != "NA" {
		p.Signal_Count += 1
	}

	p.chooseSide()

	return p
}

func (p *Pair) calculateEMACross() {
	// TODO: check for EMA cross between 5 & 9
	// TODO: check for EMA cross between 10 & 50

	var cross string = NA
	var delta [3]int
	var sum int

	for i := 0; i < 3; i++ {
		if p.EMA_005[i] < p.EMA_009[i] {
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

	p.EMA_Cross = cross
}

// TODO: give margin to evaluation (< 0.15% distance to EMA should be neutral)
func (p *Pair) calculateTrend() string {
	if p.Price >= p.EMA_050 && p.Price >= p.EMA_200 {
		return BULLISH_X2
	}

	if p.Price >= p.EMA_050 || p.Price >= p.EMA_200 {
		return BULLISH
	}

	if p.Price < p.EMA_050 && p.Price < p.EMA_200 {
		return BEARISH_X2
	}

	if p.Price < p.EMA_050 || p.Price < p.EMA_200 {
		return BEARISH
	}

	return NA
}

func (p *Pair) chooseSide() {
	// NOTE: may want to check RSI for confirmation/discard
	if p.Price < p.EMA_200 && p.EMA_Cross == BULLISH {
		p.Side = BUY
	} else if p.Price > p.EMA_200 && p.EMA_Cross == BEARISH {
		p.Side = SELL
	}
}

func (p *Pair) evaluateRSI() string {
	if p.RSI >= RSI_HOT_L3 {
		return OVERBOUGHT_X3
	}

	if p.RSI >= RSI_HOT_L2 {
		return OVERBOUGHT_X2
	}

	if p.RSI >= RSI_HOT_L1 {
		return OVERBOUGHT
	}

	if p.RSI <= RSI_COLD_L3 {
		return OVERSOLD_X3
	}

	if p.RSI <= RSI_COLD_L2 {
		return OVERSOLD_X2
	}

	if p.RSI <= RSI_COLD_L1 {
		return OVERSOLD_X2
	}

	return NA
}
