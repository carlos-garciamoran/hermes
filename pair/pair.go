package pair

type Pair struct {
	EMA_009      []float64 // Array to check for cross.
	EMA_021      []float64 // Array to check for cross.
	EMA_100      float64   // Float to compare with price.
	EMA_200      float64   // Float to compare with price.
	EMA_Cross    string
	EMA_Trend    string
	Price        float64
	RSI          float64
	RSI_Signal   string
	Signal_Count uint
	Symbol       string
}

// Constant value for neutral signal (EMA_Trend and RSI_Signal).
const (
	NA = "NA"
)

// Constant values for EMA_Trend.
const (
	BULLISH    = "bullish"
	BULLISH_X2 = "bullish-X2"

	BEARISH    = "bearish"
	BEARISH_X2 = "bearish-X2"
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

func New(
	EMA_009 []float64, EMA_021 []float64, EMA_100 float64, EMA_200 float64,
	price float64, RSI float64, symbol string,
) Pair {
	p := Pair{
		EMA_009:      EMA_009,
		EMA_021:      EMA_021,
		EMA_100:      EMA_100,
		EMA_200:      EMA_200,
		Price:        price,
		RSI:          RSI,
		Signal_Count: 0,
		Symbol:       symbol[:len(symbol)-4], // Trim "USDT" suffix
	}

	p.calculateEMACross()

	if p.EMA_Cross != "NA" {
		p.Signal_Count += 1
	}

	p.EMA_Trend = p.calculateEMATrend()

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
func (p *Pair) calculateEMATrend() string {
	if p.Price >= p.EMA_100 && p.Price >= p.EMA_200 {
		return BULLISH_X2
	}

	if p.Price >= p.EMA_100 || p.Price >= p.EMA_200 {
		return BULLISH
	}

	if p.Price < p.EMA_100 && p.Price < p.EMA_200 {
		return BEARISH_X2
	}

	if p.Price < p.EMA_100 || p.Price < p.EMA_200 {
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
