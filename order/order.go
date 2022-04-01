package order

import (
	"hermes/pair"

	"context"
	"strconv"

	"github.com/adshao/go-binance/v2/futures"
	"github.com/rs/zerolog"
)

func New(futuresClient *futures.Client, log zerolog.Logger, p *pair.Pair) {
	side := futures.SideTypeBuy
	if p.Side == pair.SELL {
		side = futures.SideTypeSell
	}

	res, err := futuresClient.NewGetAccountService().Do(context.Background())
	if err != nil {
		log.Fatal().Str("err", err.Error()).Msg("Crashed getting wallet balance")
	}

	balance, _ := strconv.ParseFloat(res.TotalWalletBalance, 32)
	quantity := strconv.FormatFloat(p.Price/balance, 'f', 6, 64)

	log.Info().
		Str("symbol", p.Symbol).
		Str("quantity", quantity).
		Msg("creating new order")

	order, err := futuresClient.NewCreateOrderService().Symbol(p.Symbol + "USDT").
		Side(side).Type(futures.OrderTypeMarket).Quantity(quantity).Do(context.Background())
	if err != nil {
		log.Fatal().Str("err", err.Error()).Msg("Crashed creating Binance order")
	}

	log.Info().
		Int64("OrderID", order.OrderID).
		Msg("CREATED ORDER")

	// TODO: send LONG/SHORT ALERT on TELEGRAM
	// utils.SendTelegramAlert()
}
