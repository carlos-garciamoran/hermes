package account

import "hermes/position"

type Account struct {
	AllocatedBalance float64              // Balance locked in positions.
	AvailableBalance float64              // Balance free to use.
	ClosedPositions  []*position.Position // Self-explanatory.
	InitialBalance   float64              // Unchanged. Used for reference. NOTE: may want to rename to StartingCapital
	Loses            int                  // Counter of losing trades.
	NetPNL           float64              // Net PNL in USDT.
	PNL              float64              // PNL in percentage.
	OpenPositions    []*position.Position // Self-explanatory.
	Real             bool                 // Whether the account trades real capital or not.
	TotalBalance     float64              // AllocatedBalance + AvailableBalance.
	Wins             int                  // Counter of winning trades.
}

// New creates an Account struct with all fields initialized.
func New(initialBalance float64, real bool) Account {
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
		Real:             real,
		TotalBalance:     initialBalance,
		Wins:             0,
	}
}

// LogNewPosition records allocated and avaible balances and adds the passed position to OpenPositions.
func (acct *Account) LogNewPosition(p *position.Position) {
	acct.AllocatedBalance += p.Size
	acct.AvailableBalance -= p.Size
	acct.OpenPositions = append(acct.OpenPositions, p)
}

// LogClosedPosition records balances and PNLs, adds the position passed to ClosedPositions, and
// removes it from OpenPositions.
func (acct *Account) LogClosedPosition(p *position.Position) {
	acct.AllocatedBalance -= p.Size
	acct.AvailableBalance += (p.Size + p.NetPNL)
	acct.TotalBalance += p.NetPNL
	acct.ClosedPositions = append(acct.ClosedPositions, p)
	acct.NetPNL += p.NetPNL

	if p.NetPNL > 0 {
		acct.Wins += 1
	} else {
		acct.Loses += 1
	}

	acct.PNL = ((acct.TotalBalance - acct.InitialBalance) / acct.TotalBalance) * 100

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

// CalculateOpenPositionsPNLs calculates the P&L (in USDT and percentage) for each open position.
func (acct *Account) CalculateOpenPositionsPNLs(symbolPrices map[string]float64) map[string][]float64 {
	pnls := make(map[string][]float64, len(acct.OpenPositions))

	for _, p := range acct.OpenPositions {
		pnl := p.CalculatePNL(symbolPrices[p.Symbol])

		pnls[p.Symbol] = append(pnls[p.Symbol], pnl*p.Size, pnl*100)
	}

	return pnls
}

// CalculateUnrealizedPNL calculates the total unrealized P&L of all open positions, returning
// the USDT and percentage values.
func (acct *Account) CalculateUnrealizedPNL(symbolPrices map[string]float64) (float64, float64) {
	unrealizedPNL := 0.0

	for _, p := range acct.OpenPositions {
		pnl := p.CalculatePNL(symbolPrices[p.Symbol])

		unrealizedPNL += pnl * p.Size
	}

	liveBalance := acct.TotalBalance + unrealizedPNL
	rawPNL := ((liveBalance - acct.InitialBalance) / liveBalance) * 100

	return unrealizedPNL, rawPNL
}
