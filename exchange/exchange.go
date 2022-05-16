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
