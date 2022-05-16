package position

import (
	"hermes/analysis"
)

// TODO: move to a variable set elsewhere. Do not hardcode!
// NOTE: these values should work for 4h.
const SL float64 = 0.01
const TP float64 = 0.04

type Position struct {
	EntryPrice  float64 // Price returned by the exchange (USDT)
	EntrySignal string  // "EMA Cross", "RSI". May be expanded in the future.
	ExitPrice   float64 // Price returned by the exchange (USDT)
	ExitSignal  string  // "SL", "TP". May be an indicator in the future.
	NetPNL      float64 // Unrealized (USDT)
	PNL         float64 // Unrealized (percentage)
	Side        string  // One of [analysis.BUY, analysis.SELL]
	Size        float64 // (USDT)
	Symbol      string  // Name of the position's asset.
	SL          float64 // Target stop loss (USDT)
	TP          float64 // Target take profit (USDT)
}

// TODO: set SL and TP targets according to asset's price precision.
func New(a *analysis.Analysis, size float64) *Position {
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
		Size:        size,
		Symbol:      a.Symbol,
		SL:          sl,
		TP:          tp,
	}

	return p
}

func (p *Position) Close(exitPrice float64, exitSignal string) {
	p.ExitPrice, p.ExitSignal = exitPrice, exitSignal

	p.PNL = p.CalculatePNL(exitPrice)
	p.NetPNL = p.PNL * p.Size
}

func (p *Position) CalculatePNL(price float64) float64 {
	if p.Side == analysis.BUY {
		return (price - p.EntryPrice) / p.EntryPrice
	}

	return (p.EntryPrice - price) / price
}

func calculateSL_TP(a *analysis.Analysis) (float64, float64) {
	if a.Side == analysis.BUY {
		return a.Price - (a.Price * SL), a.Price + (a.Price * TP)
	}

	return a.Price + (a.Price * SL), a.Price - (a.Price * TP)
}
