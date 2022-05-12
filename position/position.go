package position

import (
	"fmt"

	"hermes/analysis"
)

// TODO: move to a variable set elsewhere. Do not hardcode!
// NOTE: these should work for 4h.
const SL float64 = 0.01
const TP float64 = 0.04

type Position struct {
	EntryPrice  float64 // Price returned by the exchange (USDT)
	EntrySignal string
	ExitPrice   float64 // Price returned by the exchange (USDT)
	ExitSignal  string  // "SL", "TP". may be an indicator in the future.
	NetPNL      float64 // Unrealized (USDT)
	PNL         float64 // Unrealized (percentage)
	Side        string  // One of [analysis.BUY, analysis.SELL]
	Size        float64 // (USDT)
	Symbol      string
	SL          float64 // Target stop loss (USDT)
	TP          float64 // Target take profit (USDT)
}

var allPositions []*Position
var openPositions []*Position

// TODO: set SL and TP targets according to asset's price precision.
func New(a *analysis.Analysis) *Position {
	price := a.Price

	sl, tp := calculateSL_TP(a)

	p := &Position{
		EntryPrice:  price,
		EntrySignal: a.EMA_Cross + " EMA cross",
		ExitPrice:   0,
		ExitSignal:  "",
		NetPNL:      0,
		PNL:         0,
		Side:        a.Side,
		Size:        100,
		Symbol:      a.Symbol,
		SL:          sl,
		TP:          tp,
	}

	allPositions = append(allPositions, p)
	openPositions = append(openPositions, p)

	return p
}

func CalculateAggregatedPNLs(symbolPrices map[string]float64) (float64, float64) {
	totalNetPNL, totalPNL := 0.0, 0.0

	for _, position := range openPositions {
		pnl := position.calculatePNL(symbolPrices[position.Symbol])

		totalNetPNL += (pnl * position.Size)
		totalPNL += pnl
	}

	return totalNetPNL, totalPNL * 100
}

func CalculateAllPNLs(symbolPrices map[string]float64) map[string][]float64 {
	pnls := make(map[string][]float64, len(openPositions))

	for _, position := range openPositions {
		symbol := position.Symbol

		pnl := position.calculatePNL(symbolPrices[symbol])

		pnls[symbol] = append(pnls[symbol], pnl*position.Size, pnl*100)
	}

	return pnls
}

func (p *Position) Close(exitPrice float64, exitSignal string) {
	p.ExitPrice, p.ExitSignal = exitPrice, exitSignal

	p.PNL = p.calculatePNL(exitPrice)
	p.NetPNL = p.PNL * p.Size

	// Find and remove position from slice.
	for i := 0; i < len(openPositions); i++ {
		if p == openPositions[i] {
			fmt.Println("FOUND POSITION")
			openPositions[i] = openPositions[len(openPositions)-1]
			openPositions = openPositions[:len(openPositions)-1]
			break
		}
	}
}

func (p *Position) calculatePNL(price float64) float64 {
	if p.Side == analysis.BUY {
		return (price - p.EntryPrice) / p.EntryPrice
	}

	return (p.EntryPrice - price) / price
}

func calculateSL_TP(a *analysis.Analysis) (float64, float64) {
	if a.Side == analysis.BUY {
		return a.Price - (a.Price * SL), a.Price + (a.Price * TP)
	}

	return a.Price - (a.Price * SL), a.Price + (a.Price * TP)
}
