package position

import (
	"fmt"
	"hermes/analysis"
)

// TODO: move to a variable set elsewhere. Do not hardcode!
// NOTE: these should work for 4h.
const SL float64 = 0.1
const TP float64 = 0.5

type Position struct {
	EntryPrice  float64 // Price returned by the exchange (USDT)
	EntrySignal string
	ExitPrice   float64 // Price returned by the exchange (USDT)
	ExitSignal  string
	NetPNL      float64 // Unrealized (USDT)
	PNL         float64 // Unrealized (percentage)
	Side        string  // One of [analysis.BUY, analysis.SELL]
	Size        float64 // (USDT)
	Symbol      string
	SL          float64 // Target stop loss (USDT)
	TP          float64 // Target take profit (USDT)
}

var simulatedPositions []Position

func New(a *analysis.Analysis) Position {
	// TODO: calculate TP & SL
	price := a.Price
	p := Position{
		EntryPrice:  price,
		EntrySignal: a.EMA_Cross + " EMA cross",
		ExitPrice:   0,
		ExitSignal:  "", // Temporary, may be an indicator in the future.
		NetPNL:      0,
		PNL:         0,
		Side:        a.Side,
		Size:        100,
		Symbol:      a.Symbol,
		SL:          price - (price * SL),
		TP:          price + (price * TP),
	}

	simulatedPositions = append(simulatedPositions, p)

	return p
}

func CalculateAggregatedPNLs(symbolPrices map[string]float64) (float64, float64) {
	totalNetPNL, totalPNL := 0.0, 0.0

	for _, position := range simulatedPositions {
		pnl := position.calculatePNL(symbolPrices[position.Symbol])

		totalNetPNL += (pnl * position.Size)
		totalPNL += pnl
	}

	return totalNetPNL, totalPNL * 100
}

func CalculateAllPNLs(symbolPrices map[string]float64) map[string][]float64 {
	pnls := make(map[string][]float64, len(simulatedPositions))

	for _, position := range simulatedPositions {
		symbol := position.Symbol

		pnl := position.calculatePNL(symbolPrices[symbol])

		pnls[symbol] = append(pnls[symbol], pnl*position.Size, pnl*100)
	}

	return pnls
}

func (p *Position) Close(exitPrice float64, exitSignal string) {
	fmt.Println(p)

	p.ExitPrice, p.ExitSignal = exitPrice, exitSignal

	p.PNL = p.calculatePNL(exitPrice)
	p.NetPNL = p.PNL * p.Size

	fmt.Println(p)
}

func (p *Position) calculatePNL(price float64) float64 {
	if p.Side == analysis.BUY {
		return (price - p.EntryPrice) / p.EntryPrice
	}

	return (p.EntryPrice - price) / price
}
