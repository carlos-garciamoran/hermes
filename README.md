# hermes 💎
Hermes is a cryptocurrency scanner with trading bot capabilities (Binance USD-M Futures).

## Features
- Gets price data from WebSocket streams 🔌
- Leverages Telegram 🔔
  - Notifies on signals and price alerts
  - Listens for commands
- Analyzes 💡
  - RSI
  - EMA trend
  - EMA crossovers
- Opens trades 💸
  - with real capital on Binance USD-M Futures
  - simulated while keeping track of PNL (net and unrealized)

## Telegram bot commands
- `/account`: Get a breakdown of the trading account.
- `/pnl`: Get the account's net PNL (closed positions).
- `/positions`: Get a briefing of the open positions.
- `/upnl`: Get the current unrealized PNL (open positions).

## Usage
1. Rename `.env.example` to `.env`
2. Set up `.env`
3. Rename `alerts.example.json` to `alerts.json`
4. Run!

```bash
$ go run main.go -help
  -interval string
      interval to perform TA: 1m, 3m, 5m, 15m, 30m, 1h, 2h, 4h, 1d
  -max-positions int
    	maximum positions to open (default=5)
  -signals
    	send signal alerts on Telegram (default=false)
  -simulate
    	simulate opening trades when signals are triggered (default=true)
  -trade
    	trade signals on Binance USD-M account (default=false)
```