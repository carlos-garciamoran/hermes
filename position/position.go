package position

import (
	"hermes/analysis"
)

type Position struct {
	EntryPrice  float64
	EntrySignal string
	NetPNL      float64 // Unrealized, USDT
	PNL         float64 // Unrealized, percentage
	Side        string  // One of analysis.BUY, analysis.SELL
	Size        float64 // USDT
	Symbol      string
}

var simulatedPositions []Position

func New(a *analysis.Analysis) Position {
	p := Position{
		EntryPrice:  a.Price,
		EntrySignal: a.EMA_Cross + " EMA cross",
		NetPNL:      0,
		PNL:         0,
		Side:        a.Side,
		Size:        100,
		Symbol:      a.Symbol,
	}

	simulatedPositions = append(simulatedPositions, p)

	return p
}

func CalculateAggregatedPNLs(symbolPrices map[string]float64) (float64, float64) {
	totalNetPNL, totalPNL := 0.0, 0.0

	for _, position := range simulatedPositions {
		pnl := position.calculatePNL(symbolPrices)

		totalNetPNL += (pnl * position.Size)
		totalPNL += pnl
	}

	return totalNetPNL, totalPNL * 100
}

func CalculateAllPNLs(symbolPrices map[string]float64) map[string][]float64 {
	pnls := make(map[string][]float64, len(simulatedPositions))

	for _, position := range simulatedPositions {
		symbol := position.Symbol

		pnl := position.calculatePNL(symbolPrices)

		pnls[symbol] = append(pnls[symbol], pnl*position.Size, pnl*100)
	}

	return pnls
}

func (p *Position) calculatePNL(symbolPrices map[string]float64) float64 {
	symbol := p.Symbol
	price := symbolPrices[symbol]

	pnl := (price - p.EntryPrice) / p.EntryPrice
	if p.Side == analysis.SELL {
		pnl = (p.EntryPrice - price) / price
	}

	return pnl
}
