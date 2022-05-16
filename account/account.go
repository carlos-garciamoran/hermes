package account

import "hermes/position"

type Account struct {
	AllocatedBalance float64              // Balance locked in positions.
	AvailableBalance float64              // Balance free to use.
	ClosedPositions  []*position.Position // Self-explanatory.
	InitialBalance   float64              // Unchanged.
	Loses            int                  // Counter of losing trades.
	NetPNL           float64              // Net PNL in USDT.
	PNL              float64              // PNL in percentage.
	OpenPositions    []*position.Position // Self-explanatory.
	TotalBalance     float64              // AllocatedBalance + AvailableBalance.
	Wins             int                  // Counter of winning trades.
}

func New(initialBalance float64) Account {
	// See github.com/golang/go/wiki/CodeReviewComments#declaring-empty-slices
	var closedPositions, openPositions []*position.Position

	return Account{
		AllocatedBalance: 0.0,
		AvailableBalance: initialBalance,
		ClosedPositions:  closedPositions,
		InitialBalance:   initialBalance,
		Loses:            0,
		NetPNL:           0.0,
		PNL:              0.0,
		OpenPositions:    openPositions,
		TotalBalance:     0.0,
		Wins:             0,
	}
}

func (acct *Account) LogNewPosition(p *position.Position) {
	acct.AllocatedBalance += p.Size
	acct.AvailableBalance -= p.Size
	acct.OpenPositions = append(acct.OpenPositions, p)
}

func (acct *Account) LogClosedPosition(p *position.Position) {
	acct.AllocatedBalance -= p.Size
	acct.AvailableBalance += (p.Size + p.NetPNL)
	acct.TotalBalance += p.NetPNL
	acct.ClosedPositions = append(acct.ClosedPositions, p)
	acct.NetPNL += p.NetPNL

	if p.NetPNL >= 0 {
		acct.Wins += 1
	} else {
		acct.Loses += 1
	}

	openPositions := acct.OpenPositions // Used as a shorthand.

	// Find and remove the position from OpenPositions.
	for i := 0; i < len(openPositions); i++ {
		if p == openPositions[i] {
			acct.OpenPositions[i] = openPositions[len(openPositions)-1]
			acct.OpenPositions = openPositions[:len(openPositions)-1]
			break
		}
	}
}

func (acct *Account) CalculateUnrealizedPNL(symbolPrices map[string]float64) (float64, float64) {
	totalNetPNL, totalPNL := 0.0, 0.0

	for _, position := range acct.OpenPositions {
		pnl := position.CalculatePNL(symbolPrices[position.Symbol])

		totalNetPNL += (pnl * position.Size)
		totalPNL += pnl
	}

	return totalNetPNL, totalPNL * 100
}

func (acct *Account) CalculateOpenPositionsPNLs(symbolPrices map[string]float64) map[string][]float64 {
	openPositions := acct.OpenPositions
	pnls := make(map[string][]float64, len(openPositions))

	for _, position := range openPositions {
		symbol := position.Symbol

		pnl := position.CalculatePNL(symbolPrices[symbol])

		pnls[symbol] = append(pnls[symbol], pnl*position.Size, pnl*100)
	}

	return pnls
}