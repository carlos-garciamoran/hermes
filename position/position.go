package position

import (
	"hermes/analysis"
	"math"
)

// TODO: move to a variable set elsewhere. Do not hardcode!
const SL float64 = 0.04
const TP float64 = 0.20

type Position struct {
	Asset       *analysis.Asset // Asset of the symbol.
	EntryPrice  float64         // Entry price (USDT). When real, price returned by the exchange.
	EntrySignal string          // "EMA Cross", "RSI". May be expanded in the future.
	ExitPrice   float64         // Exit price (USDT). When real, price returned by the exchange.
	ExitSignal  string          // "SL", "TP". May be an indicator in the future.
	NetPNL      float64         // Net profit and loss (USDT).
	PNL         float64         // Net profit and loss (percentage).
	Quantity    float64         // Quantity of the position (in the base asset).
	Side        string          // analysis.BUY, analysis.SELL.
	Size        float64         // Size of the position (USDT).
	Symbol      string          // Name of the position's asset.
	SL          float64         // Target stop loss (USDT).
	TP          float64         // Target take profit (USDT).
}

// New creates a Position struct with all fields initialized.
func New(a *analysis.Analysis, quantity float64, size float64) *Position {
	asset, price := a.Asset, a.Price

	sl, tp := calculateSLAndTP(a)

	p := &Position{
		Asset:       asset,
		EntryPrice:  price,
		EntrySignal: a.EMACross + " EMA cross",
		ExitPrice:   0.0,
		ExitSignal:  "",
		NetPNL:      0.0,
		PNL:         0.0,
		Quantity:    round(quantity, asset.QuantityPrecision),
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
	rawPNL := p.CalculatePNL(exitPrice)

	p.ExitPrice, p.ExitSignal = exitPrice, exitSignal
	p.NetPNL = rawPNL * p.Size
	p.PNL = rawPNL * 100 // Store the percentage.
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
	decimals := a.Asset.PricePrecision

	sl, tp := a.Price-(a.Price*SL), a.Price+(a.Price*TP)
	if a.Side == analysis.SELL {
		sl, tp = a.Price+(a.Price*SL), a.Price-(a.Price*TP)
	}

	// NOTE: may want to do math.Ceil or math.Floor according to SL/TP and BUY/SELL
	// e.g., for BUY: SL should be Ceil (round up) and TP should be Floor (round down)

	return round(sl, decimals), round(tp, decimals)
}

// round rounds price to decimals.
func round(price float64, decimals int) float64 {
	factor := math.Pow(10, float64(decimals))

	return math.Round(price*factor) / factor
}
