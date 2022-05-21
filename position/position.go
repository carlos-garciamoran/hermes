package position

import "hermes/analysis"

// TODO: move to a variable set elsewhere. Do not hardcode!
const SL float64 = 0.01
const TP float64 = 0.04

type Position struct {
	EntryPrice  float64 // Entry price (USDT). When real, price returned by the exchange.
	EntrySignal string  // "EMA Cross", "RSI". May be expanded in the future.
	ExitPrice   float64 // Exit price (USDT). When real, price returned by the exchange.
	ExitSignal  string  // "SL", "TP". May be an indicator in the future.
	NetPNL      float64 // Net profit and loss (USDT).
	PNL         float64 // Net profit and loss (percentage).
	Side        string  // analysis.BUY, analysis.SELL.
	Size        float64 // Size of the position (USDT).
	Symbol      string  // Name of the position's asset.
	SL          float64 // Target stop loss (USDT).
	TP          float64 // Target take profit (USDT).
}

// New returns a struct of type Position with all fields initialized.
func New(a *analysis.Analysis, size float64) *Position {
	price := a.Price

	// TODO: set SL and TP targets according to asset's price precision.

	sl, tp := calculateSLAndTP(a)

	p := &Position{
		EntryPrice:  price,
		EntrySignal: a.EMACross + " EMA cross",
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

// Close closes a position by setting ExitPrice, ExitSignal, NetPNL, and PNL.
func (p *Position) Close(exitPrice float64, exitSignal string) {
	p.ExitPrice, p.ExitSignal = exitPrice, exitSignal

	p.PNL = p.CalculatePNL(exitPrice)
	p.NetPNL = p.PNL * p.Size
	p.PNL *= 100 // Store the percentage
}

// CalculatePNL calculates then PNL based on the position's size and the [exit] price passed.
func (p *Position) CalculatePNL(price float64) float64 {
	if p.Side == analysis.BUY {
		return (price - p.EntryPrice) / p.EntryPrice
	}

	return (p.EntryPrice - price) / price
}

// calculateSLAndTP calculates the SL and TP targets based on the analysis' side, SL/TP constants, and price.
func calculateSLAndTP(a *analysis.Analysis) (float64, float64) {
	if a.Side == analysis.BUY {
		return a.Price - (a.Price * SL), a.Price + (a.Price * TP)
	}

	return a.Price + (a.Price * SL), a.Price - (a.Price * TP)
}
