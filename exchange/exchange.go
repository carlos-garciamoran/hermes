package exchange

import (
	"context"
	"strconv"
	"sync"

	"hermes/analysis"

	"github.com/adshao/go-binance/v2/futures"
	"github.com/rs/zerolog"
)

var mutex = &sync.Mutex{}

func FetchAssets(
	futuresClient *futures.Client, interval string, limit int, log *zerolog.Logger,
	symbolAssets map[string]analysis.Asset, symbolCloses map[string][]float64,
	wg *sync.WaitGroup,
) map[string]string {
	symbolIntervalPair := make(map[string]string)

	exchangeInfo, err := futuresClient.NewExchangeInfoService().Do(context.Background())
	if err != nil {
		log.Fatal().Str("err", err.Error()).Msg("Crashed getting exchange info")
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

				klines, err := futuresClient.NewKlinesService().
					Symbol(symbol).Interval(interval).Limit(limit).Do(context.Background())
				if err != nil {
					log.Fatal().Str("err", err.Error()).Msg("Crashed fetching klines")
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

// TODO: fetch and parse balance from Binance.
func FetchBalance() float64 {
	return 1337.00
}

func NewOrder(futuresClient *futures.Client, log zerolog.Logger, a *analysis.Analysis) {
	// TODO: cache available balance >>> do NOT request on every call.
	res, err := futuresClient.NewGetAccountService().Do(context.Background())
	if err != nil {
		log.Fatal().Str("err", err.Error()).Msg("Crashed getting wallet balance")
	}

	asset := a.Asset

	// NOTE: substract 10% from total balance to give margin
	balance, _ := strconv.ParseFloat(res.TotalWalletBalance, 64)
	availableBalance := balance - (balance * .1)

	quantity := availableBalance / a.Price

	log.Debug().
		Float64("availableBalance", availableBalance).
		Float64("minQuantity", asset.MinQuantity).
		Int("precision", asset.QuantityPrecision).
		Float64("quantity", quantity).
		Str("symbol", asset.Symbol).
		Msg("Trying to create new order...")

	side := futures.SideTypeBuy
	if a.Side == analysis.SELL {
		side = futures.SideTypeSell
	}

	if quantity >= asset.MinQuantity && quantity <= asset.MaxQuantity {
		finalQuantity := strconv.FormatFloat(quantity, 'f', asset.QuantityPrecision, 64)
		log.Info().Msg("Getting in...")

		order, err := futuresClient.NewCreateOrderService().
			Symbol(asset.Symbol).
			Side(side).
			Type(futures.OrderTypeMarket).
			Quantity(finalQuantity).
			Do(context.Background())
		if err != nil {
			log.Fatal().Str("err", err.Error()).Msg("Crashed creating Binance order")
		}

		log.Info().
			Int64("OrderID", order.OrderID).
			Msg("💳 Created order")

		// TODO: send LONG/SHORT ALERT on TELEGRAM
		// utils.SendTelegramAlert()
	}
}
