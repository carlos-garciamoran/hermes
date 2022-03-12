package pair

import (
	"math"

	"github.com/markcheno/go-talib"
)

type Pair struct {
	EMA_009      []float64 // Array to check for cross.
	EMA_021      []float64 // Array to check for cross.
	EMA_055      float64   // Float to compare with price.
	EMA_200      float64   // Float to compare with price.
	EMA_Cross    string
	Price        float64
	RSI          float64
	RSI_Signal   string
	Signal_Count uint
	Symbol       string
	Trend        string // Based on EMA_055, EMA_200, and Price.
}

// Constant value for neutral signal (EMA_Trend and RSI_Signal).
const (
	NA = "NA"
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

// Constant values for Trend.
const (
	BULLISH    = "bullish"
	BULLISH_X2 = "bullish-X2"

	BEARISH    = "bearish"
	BEARISH_X2 = "bearish-X2"
)

// ⬆️, ⬇️
var Emojis = map[string]string{
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
	EMA_009 := talib.Ema(closes, 9)[lastCloseIndex-2:]
	EMA_021 := talib.Ema(closes, 21)[lastCloseIndex-2:]
	EMA_055 := talib.Ema(closes, 55)[lastCloseIndex]
	EMA_200 := talib.Ema(closes, 200)[lastCloseIndex]

	// Round to 2 digits.
	RSI := math.Round(talib.Rsi(closes, 14)[lastCloseIndex]*100) / 100

	p := Pair{
		EMA_009:      EMA_009,
		EMA_021:      EMA_021,
		EMA_055:      EMA_055,
		EMA_200:      EMA_200,
		Price:        closes[lastCloseIndex],
		RSI:          RSI,
		Signal_Count: 0,
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

	return p
}

func (p *Pair) calculateEMACross() {
	var cross string = NA
	var delta [3]int
	var sum int

	for i := 0; i < 3; i++ {
		if p.EMA_009[i] < p.EMA_021[i] {
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
	if p.Price >= p.EMA_055 && p.Price >= p.EMA_200 {
		return BULLISH_X2
	}

	if p.Price >= p.EMA_055 || p.Price >= p.EMA_200 {
		return BULLISH
	}

	if p.Price < p.EMA_055 && p.Price < p.EMA_200 {
		return BEARISH_X2
	}

	if p.Price < p.EMA_055 || p.Price < p.EMA_200 {
		return BEARISH
	}

	return NA
}

func (p *Pair) evaluateRSI() string {
	if p.RSI >= 89.9 { // 90
		return OVERBOUGHT_X3
	}

	if p.RSI >= 84.9 { // 85
		return OVERBOUGHT_X2
	}

	if p.RSI >= 69.9 { // 70
		return OVERBOUGHT
	}

	if p.RSI <= 10.1 { // 10
		return OVERSOLD_X3
	}

	if p.RSI <= 15.1 { // 15
		return OVERSOLD_X2
	}

	if p.RSI <= 30.1 { // 30
		return OVERSOLD_X2
	}

	return NA
}
