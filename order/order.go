package order

import (
	"hermes/pair"

	"context"
	"strconv"

	"github.com/adshao/go-binance/v2/futures"
	"github.com/rs/zerolog"
)

func New(futuresClient *futures.Client, log zerolog.Logger, p *pair.Pair) {
	// TODO: cache available balance >>> do NOT request on every call.
	res, err := futuresClient.NewGetAccountService().Do(context.Background())
	if err != nil {
		log.Fatal().Str("err", err.Error()).Msg("Crashed getting wallet balance")
	}

	asset := p.Asset

	// NOTE: substract 10% from total balance to give margin
	balance, _ := strconv.ParseFloat(res.TotalWalletBalance, 64)
	availableBalance := balance - (balance * .1)

	quantity := availableBalance / p.Price

	log.Debug().
		Float64("availableBalance", availableBalance).
		Float64("minQuantity", asset.MinQuantity).
		Int("precision", asset.QuantityPrecision).
		Float64("quantity", quantity).
		Str("symbol", asset.Symbol).
		Msg("Trying to create new order...")

	side := futures.SideTypeBuy
	if p.Side == pair.SELL {
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
