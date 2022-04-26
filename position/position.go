package position

import (
	"hermes/analysis"
)

type Position struct {
	EntryPrice  float64
	EntrySignal string
	PNL         float64 // Unrealized
	Side        string  // One of analysis.BUY, analysis.SELL
	Size        float64 // In USDT
	Symbol      string
}

var simulatedPositions []Position

func New(a *analysis.Analysis) Position {
	p := Position{
		EntryPrice:  a.Price,
		EntrySignal: a.EMA_Cross + " EMA cross",
		PNL:         0,
		Side:        a.Side,
		Size:        100,
		Symbol:      a.Symbol,
	}

	simulatedPositions = append(simulatedPositions, p)

	return p
}

func CalculateTotalPNL(symbolPrices map[string]float64) float64 {
	totalPNL := 0.0

	for _, position := range simulatedPositions {
		price := symbolPrices[position.Symbol]

		pnl := (price - position.EntryPrice) / position.EntryPrice // Buy
		if position.Side == analysis.SELL {
			pnl = (position.EntryPrice - price) / price
		}

		totalPNL += (pnl * position.Size)
	}

	return totalPNL
}
