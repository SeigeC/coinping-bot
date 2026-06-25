# CoinPing — Multi-Condition Crypto Price Alerts on Telegram

Stop refreshing charts. Set your conditions, get pinged when they hit.

[@CoinPingAlertBot](https://t.me/CoinPingAlertBot)

## What it does

Most price-alert bots only ping you when a coin crosses a number. CoinPing supports **three alert types**:

- **Price threshold** — Get notified the moment a coin crosses your target price, up or down
- **24h % change** — Catch volatility without staring at a chart
- **Cross-exchange spread** — Spot wicks, lag, and liquidity events as they happen

Also includes live price lookup, top-10 market overview, alert management, and an optional daily digest.

## Commands

| Command | What it does |
|---|---|
| `/start` | Welcome and quick start |
| `/price BTC` | Live spot price |
| `/top` | Top 10 by market cap (with 24h change) |
| `/alert price BTC 65000 up` | Price threshold alert |
| `/alert change BTC 5` | 24h % change alert (±5%) |
| `/alert spread BTC 0.5` | Cross-exchange spread alert (±0.5%) |
| `/alerts` | List your active alerts |
| `/delalert 3` | Delete alert #3 |
| `/digest on` | Enable daily market summary |
| `/help` | Full help |

## Pricing

**Free tier**: 3 active alerts per user. Paid tier (unlimited alerts) coming soon.

## Tech stack

- Go + [telebot v3](https://github.com/tucnak/telebot)
- [CoinGecko API](https://www.coingecko.com/en/api) (free tier)
- SQLite (modernc.org/sqlite — pure Go, no CGo)
- Deployed on Render (free tier)

## Deploy your own

```bash
cp .env.example .env
# Edit .env with your bot token
docker build -t coinping-bot .
docker run -e TELEGRAM_BOT_TOKEN=your_token coinping-bot
```

Built by [@SeigeC](https://github.com/SeigeC)
