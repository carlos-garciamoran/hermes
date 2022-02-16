package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/adshao/go-binance/v2"
	"github.com/adshao/go-binance/v2/futures"
	"github.com/joho/godotenv"
	"github.com/markcheno/go-talib"
)

const limit int = 200

var services = make(map[string]*futures.KlinesService)

func loadEnvFile() (string, string) {
	err := godotenv.Load()

	if err != nil {
		log.Fatal("Error loading .env file")
	}

	return os.Getenv("BINANCE_APIKEY"), os.Getenv("BINANCE_SECRETKEY")
}

func fetchKlines(futuresClient *futures.Client, symbol string, c chan error) {
	klines, err := services[symbol].Do(context.Background())

	// TODO: find out how to sleep on ALL go subroutines (only sleeps in current one)
	if err != nil {
		fmt.Println("⛔️⛔️⛔️⛔️⛔️⛔️⛔️⛔️⛔️⛔️⛔️⛔️⛔️⛔️⛔️⛔️⛔️ RATE LIMIT ⛔️⛔️⛔️⛔️⛔️⛔️⛔️⛔️⛔️⛔️⛔️⛔️⛔️⛔️⛔️⛔️⛔️")
		fmt.Println(err)
		c <- err
		time.Sleep(30 * time.Second)
	}

	var closes []float64

	// NOTE: can't use limit because asset may be too new (num of candles < 200)
	kline_count := len(klines)

	for i := 0; i < kline_count; i++ {
		if close, err := strconv.ParseFloat(klines[i].Close, 64); err == nil {
			closes = append(closes, close)
		}
	}

	k := klines[kline_count-1]

	rsi := talib.Rsi(closes, 14)
	ema_9, ema_21 := talib.Ema(closes, 9), talib.Ema(closes, 21)

	fmt.Printf(
		"✅ %-12s %s %s %s %s | %.2f %.3f %.3f\n",
		symbol, k.Open, k.High, k.Low, k.Close,
		rsi[kline_count-1], ema_9[kline_count-1], ema_21[kline_count-1],
	)

	c <- nil
}

func main() {
	var wg sync.WaitGroup

	apiKey, secretKey := loadEnvFile()

	futuresClient := binance.NewFuturesClient(apiKey, secretKey)

	exchangeInfo, err := futuresClient.NewExchangeInfoService().Do(context.Background())
	if err != nil {
		fmt.Println(err)
		return
	}

	symbols := make([]string, 0) // Slice due to unknown length

	// Filter unwanted symbols (non-USDT, quarterlies, indexes, and unactive)
	for _, asset := range exchangeInfo.Symbols {
		if asset.QuoteAsset == "USDT" && asset.ContractType == "PERPETUAL" && asset.UnderlyingType == "COIN" && asset.Status == "TRADING" {
			symbol := asset.Symbol
			symbols = append(symbols, symbol)
			services[symbol] = futuresClient.NewKlinesService().Interval("1h").Limit(limit).Symbol(symbol)
		}
	}

	fmt.Printf("[*] Loaded %d symbols\n", len(symbols))

	// TODO: implement rate-limiting
	rate_limit := make(chan error, 1)

	for {
		i := 0
		for _, symbol := range symbols {
			fmt.Println("💡 Fetching ", symbol)

			wg.Add(1)

			go func() {
				defer wg.Done()
				fetchKlines(futuresClient, symbol, rate_limit)
				i += 1
				msg := <-rate_limit

				if msg != nil {
					fmt.Println("SLEEP SLEEP SLEEP SLEEP SLEEP SLEEP")
					time.Sleep(30 * time.Second)
				}
			}()

			time.Sleep(10 * time.Millisecond)
		}

		fmt.Println("💤💤💤💤💤💤💤💤💤💤💤💤💤💤💤💤💤💤💤 SLEEPING 💤💤💤💤💤💤💤💤💤💤💤💤💤💤💤💤💤💤")

		wg.Wait()
		// time.Sleep(1 * time.Second)

		// NOTE: after several testing, this check can be removed.
		if i == len(symbols) {
			fmt.Println("✅✅✅✅✅✅✅✅✅✅✅✅✅✅✅✅✅✅✅ DONE ✅✅✅✅✅✅✅✅✅✅✅✅✅✅✅✅✅✅")
		} else {
			fmt.Println("⛔️⛔️⛔️⛔️⛔️⛔️⛔️⛔️⛔️⛔️⛔️⛔️⛔️⛔️⛔️⛔️⛔️⛔️ MISSED ⛔️⛔️⛔️⛔️⛔️⛔️⛔️⛔️⛔️⛔️⛔️⛔️⛔️⛔️⛔️⛔️⛔️⛔️⛔️⛔️")
		}
	}
}
