package pair

type Pair struct {
	Bias   string // "bullish", "bearish"
	EMA_09 []float64
	EMA_21 []float64
	Price  float64
	RSI    float64
	Symbol string
}

// Bias constants.
const (
	NA      = "NA"
	BULLISH = "bullish"
	BEARISH = "bearish"
)

var Emoji = map[string]string{
	BULLISH: "🐗",
	BEARISH: "🐻",
}

func New(EMA_09 []float64, EMA_21 []float64, price float64, RSI float64, symbol string) Pair {
	p := Pair{
		EMA_09: EMA_09,
		EMA_21: EMA_21,
		Price:  price,
		RSI:    RSI,
		Symbol: symbol,
	}

	p.calculateEMACross()

	return p
}

func (p *Pair) calculateEMACross() {
	var delta [3]int
	var sum int
	var bias string = NA

	for i := 0; i < 3; i++ {
		if p.EMA_09[i] < p.EMA_21[i] {
			delta[i] = -1
		} else {
			delta[i] = 1
		}
	}

	for _, v := range delta {
		sum += v
	}

	// If all deltas are the same (3 or -3), there can be no cross.
	if sum%3 != 0 {
		// Check the cross on the last candle.
		if delta[2] == 1 {
			bias = BULLISH
		} else if delta[2] == -1 {
			bias = BEARISH
		}
	}

	p.Bias = bias
}
