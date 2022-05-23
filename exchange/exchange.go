package exchange

import (
	"context"
	"os"
	"strconv"
	"sync"

	"hermes/account"
	"hermes/analysis"
	"hermes/position"
	"hermes/telegram"

	"github.com/adshao/go-binance/v2"
	"github.com/adshao/go-binance/v2/futures"
	"github.com/rs/zerolog"
)

type Exchange struct {
	*telegram.Bot
	*futures.Client
	*zerolog.Logger
}

func New(bot *telegram.Bot, log *zerolog.Logger) Exchange {
	futuresClient := binance.NewFuturesClient(os.Getenv("BINANCE_APIKEY"), os.Getenv("BINANCE_SECRETKEY"))

	return Exchange{bot, futuresClient, log}
}

func (e *Exchange) FetchAssets(
	interval string, limit int, symbolAssets map[string]analysis.Asset, symbolCloses map[string][]float64,
	wg *sync.WaitGroup,
) map[string]string {
	mutex := &sync.Mutex{}
	symbolIntervalPair := make(map[string]string)

	exchangeInfo, err := e.NewExchangeInfoService().Do(context.Background())
	if err != nil {
		e.Fatal().Str("err", err.Error()).Msg("Crashed getting exchange info")
	}

	// Filter unwanted symbols (non-USDT, quarterlies, indexes, unactive, and 1000BTTC)
	for _, rawAsset := range exchangeInfo.Symbols {
		if rawAsset.QuoteAsset == "USDT" && rawAsset.ContractType == "PERPETUAL" &&
			rawAsset.UnderlyingType == "COIN" && rawAsset.Status == "TRADING" &&
			rawAsset.BaseAsset != "1000BTTC" {

			symbol := rawAsset.Symbol
			maxQuantity, _ := strconv.ParseFloat(rawAsset.LotSizeFilter().MaxQuantity, 64)
			minQuantity, _ := strconv.ParseFloat(rawAsset.LotSizeFilter().MinQuantity, 64)

			symbolAssets[symbol] = analysis.Asset{
				BaseAsset:         rawAsset.BaseAsset,
				MaxQuantity:       maxQuantity,
				MinQuantity:       minQuantity,
				PricePrecision:    rawAsset.PricePrecision,
				QuantityPrecision: rawAsset.QuantityPrecision,
				Symbol:            symbol,
			}

			symbolIntervalPair[symbol] = interval

			wg.Add(1)

			// Get the closes.
			go func() {
				defer wg.Done()

				klines, err := e.NewKlinesService().
					Symbol(symbol).Interval(interval).Limit(limit).Do(context.Background())
				if err != nil {
					e.Fatal().Str("err", err.Error()).Msg("Crashed fetching klines")
				}

				// Discard assets with less than LIMIT candles due to impossibility of computing EMA <LIMIT>.
				if len(klines) == limit {
					for i := 0; i < limit; i++ {
						if close, err := strconv.ParseFloat(klines[i].Close, 64); err == nil {
							mutex.Lock()
							symbolCloses[symbol] = append(symbolCloses[symbol], close)
							mutex.Unlock()
						}
					}
				} else {
					delete(symbolIntervalPair, symbol)
				}
			}()
		}
	}

	return symbolIntervalPair
}

// FetchBalance gets the total balance from the exchange's account wallet and parses it.
func (e *Exchange) FetchBalance() float64 {
	res, err := e.NewGetAccountService().Do(context.Background())
	if err != nil {
		e.Fatal().Str("err", err.Error()).Msg("Crashed getting wallet balance")
	}

	balance, _ := strconv.ParseFloat(res.TotalWalletBalance, 64)
	availableBalance := balance - (balance * .05) // NOTE: substract 5% to give margin.

	return availableBalance
}

// NewOrder creates a market order in the exchange for the passed position.
func (e *Exchange) NewOrder(p *position.Position) {
	asset, quantity := p.Asset, p.Quantity

	side := futures.SideTypeBuy
	if p.Side == analysis.SELL {
		side = futures.SideTypeSell
	}

	finalQuantity := strconv.FormatFloat(quantity, 'f', asset.QuantityPrecision, 64)

	// NOTE: API wrapper doesn't store the executed price and quantity (may be slightly off from targets).
	order, err := e.NewCreateOrderService().
		Symbol(asset.Symbol).Side(side).Type(futures.OrderTypeMarket).Quantity(finalQuantity).
		Do(context.Background())
	if err != nil {
		e.Fatal().Str("err", err.Error()).Msg("Crashed creating Binance order")
	}

	e.Info().Int64("OrderID", order.OrderID).Msg("💳 Created order")
}

// CloseOrder closes the given position in the exchange with a market order.
func (e *Exchange) CloseOrder(acct *account.Account, p *position.Position) {
	// WIP
}
