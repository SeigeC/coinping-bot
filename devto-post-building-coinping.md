# Building a Zero-Budget Crypto Alert Bot with Go, SQLite, and Telegram

**I couldn't find a crypto alert bot that did more than simple price pings. So I built one in Go that costs $0 to run.**

---

I've got a bad habit. I set a mental buy target for BTC — say "if it dips 5%, I'll buy" — and then spend the next three hours refreshing a chart. I either burn out staring at nothing, or close the tab and miss the move entirely.

Most crypto alert tools solve half the problem. They ping you when a coin crosses a price. That's fine. But the alerts I actually wanted were conditional: catch a 5% drop in 24 hours without watching a screen, or spot when an exchange price diverges from the market (because that sometimes signals a wick or a liquidity event).

So I did what solo devs do. I built it myself.

[CoinPing](https://t.me/CoinPingAlertBot) is a Telegram bot that does three types of alerts — price thresholds, 24h % change, and cross-exchange spread — plus live price lookups, a top-10 market cap view, and an optional daily digest. It's built in Go, uses SQLite for storage, pulls data from CoinGecko's free API, and runs on Render's free tier. Total monthly infrastructure cost: **$0**.

## The Stack

The constraint was simple: no budget, no complexity, no services that'll start billing me if someone posts it on Hacker News.

| Layer | Choice | Why |
|-------|--------|-----|
| Language | Go | Single binary, trivial deployment, great concurrency primitives |
| Telegram API | [telebot v3](https://github.com/tucnak/telebot) | The cleanest Go Telegram framework — handler-based, long-polling, no webhook required |
| Price data | [CoinGecko API](https://www.coingecko.com/en/api) free tier | 30 calls/min, no API key needed for public endpoints |
| Database | SQLite via `modernc.org/sqlite` | Pure Go (no CGo), zero setup, file-based — ship it with the binary |
| Hosting | Render free tier | 750 hours/month free, Dockerfile deploy, health-check endpoint keeps it alive |
| Cross-exchange | Binance public API | Unauthenticated ticker endpoint for spread calculation |

The critical dependency choice was `modernc.org/sqlite` over `mattn/go-sqlite3`. The latter requires CGo and a C compiler, which bloats the Docker image and complicates cross-compilation. The pure-Go driver compiles everywhere and the binary stays small — mine is under 15 MB.

## Architecture

```
Telegram user
     │
     ▼
┌─────────────┐     ┌──────────────┐     ┌───────────────┐
│  telebot v3  │────▶│   handlers   │────▶│  CoinGecko    │
│  long poll   │     │  (handler.go)│     │  (coingecko.go)│
└─────────────┘     └──────┬───────┘     └───────────────┘
                           │
                     ┌─────▼──────┐     ┌───────────────┐
                     │   SQLite   │     │  Alert Engine  │
                     │ (store.go) │◀────│(alert_engine.go)│
                     └────────────┘     └───────────────┘
                           │
                           ▼
              ┌──────────────────────┐
              │  Background Workers  │
              │  - alert loop (5 min)│
              │  - digest loop (1 min)│
              └──────────────────────┘
```

The bot runs as a single process. Two background goroutines handle periodic work: an alert checker that polls every 5 minutes, and a digest dispatcher that ticks every minute and sends summaries to subscribers whose scheduled time matches the current minute.

## The Alert Engine

This is where things get interesting. Alerts come in three flavors, and the evaluation logic is straightforward but fun to write.

### 1. Price Threshold

The simplest case — did the price cross a level?

```go
func (e *AlertEngine) checkPrice(a Alert, coinID, displaySym string) error {
    price, err := e.cg.GetSimplePrice(coinID)
    if err != nil {
        return err
    }
    threshold := a.Threshold
    if (a.Direction == "up" || a.Direction == "either") && price >= threshold {
        return e.trigger(a, fmt.Sprintf(
            "🔔 %s 上穿 $%s！当前价格 $%s",
            displaySym, formatMoney(threshold), formatMoney(price),
        ))
    }
    if (a.Direction == "down" || a.Direction == "either") && price <= threshold {
        return e.trigger(a, fmt.Sprintf(
            "🔔 %s 下穿 $%s！当前价格 $%s",
            displaySym, formatMoney(threshold), formatMoney(price),
        ))
    }
    return nil
}
```

The `"either"` direction is a subtle affordance — if the user doesn't specify up or down, the alert fires on any cross. Useful when you just want to know when price action gets interesting.

### 2. 24h Change

CoinGecko returns the percentage as a signed float. The alert fires when the absolute value exceeds the threshold, but the notification shows the direction so you know whether it's a pump or a dump.

```go
func (e *AlertEngine) checkChange(a Alert, coinID, displaySym string) error {
    change, err := e.cg.Get24hChange(coinID)
    if err != nil {
        return err
    }
    absChange := change
    if absChange < 0 {
        absChange = -absChange
    }
    if absChange >= a.Threshold {
        sign := "+"
        if change < 0 {
            sign = ""
        }
        return e.trigger(a, fmt.Sprintf(
            "🔔 %s 24h 涨跌幅 %s%.2f%% 触达 %.2f%% 阈值",
            displaySym, sign, change, a.Threshold,
        ))
    }
    return nil
}
```

### 3. Cross-Exchange Spread

This one was the most fun. It hits CoinGecko for the aggregate price and Binance's public API for the exchange-specific price, then computes the percentage spread between them. If the spread exceeds the threshold, something unusual is happening — a wick, lag, or liquidity gap.

```go
func (e *AlertEngine) checkSpread(a Alert, coinID, displaySym string) error {
    prices, err := e.cg.GetPriceMultiExchange(a.Coin)
    if err != nil {
        return err
    }
    if len(prices) < 2 {
        return nil
    }
    min, max := prices["coingecko"], prices["binance"]
    if min > max {
        min, max = max, min
    }
    spreadPct := (max - min) / min * 100
    if spreadPct >= a.Threshold {
        return e.trigger(a, fmt.Sprintf(
            "🔔 %s 跨所价差 %.2f%% 超过 %.2f%% 阈值\nCoinGecko $%s / Binance $%s",
            displaySym, spreadPct, a.Threshold,
            formatMoney(prices["coingecko"]), formatMoney(prices["binance"]),
        ))
    }
    return nil
}
```

Once triggered, the alert deactivates itself — one-shot by design. You re-enable it by creating a new one. This keeps the alert table clean and prevents notification spam.

## The Concurrent Worker Pattern

The background workers are dead simple goroutines with ticker-select loops. Shut them down cleanly with a channel close.

```go
func runAlertChecker(engine *AlertEngine, stopCh <-chan struct{}) {
    ticker := time.NewTicker(5 * time.Minute)
    defer ticker.Stop()
    for {
        select {
        case <-stopCh:
            return
        case <-ticker.C:
            engine.CheckAlerts()
        }
    }
}
```

The digest worker ticks every minute and matches subscribers by their configured time — a surprisingly pleasant primitive compared to cron.

```go
func sendDigestsDue(engine *AlertEngine) {
    now := time.Now().UTC()
    currentTime := fmt.Sprintf("%02d:%02d", now.Hour(), now.Minute())
    subscribers, err := GetDigestSubscribers(currentTime)
    // ...
    for _, userID := range subscribers {
        engine.SendDailyDigest(userID)
    }
}
```

## The Free Tier Strategy

Every alert tool I found was either fully paid or had a terrible free tier. I wanted something different: usable for free, with a clear upgrade path.

Right now it's 3 active alerts per user, no payment required. A paid unlimited tier is planned but not live. The constraint is backpressure — I want to understand usage patterns before building billing, and 3 alerts is generous enough that most casual traders won't hit the limit. If they do, they're power users worth monetizing.

## What I Learned

- **Go's concurrency model is perfect for this.** Two goroutines, two tickers, one shared engine, zero races. No message queues, no worker pools, no Redis. Just channels and select.
- **Pure-Go SQLite is a cheat code.** No CGo, no system dependencies, the database file lives next to the binary. Backups are a file copy. Migration is a function that checks `PRAGMA table_info` and runs `ALTER TABLE` if columns are missing.
- **CoinGecko's free tier is generous but fragile.** The 30 calls/min limit means the 5-minute alert cycle handles dozens of users before rate-limiting becomes a concern. But the API sometimes returns 429s during volatile markets — exactly when users want alerts most. A retry layer is next on the roadmap.
- **Telegram bots have great organic reach.** No app store review, no install friction, no notification permission prompt. Users just open a chat and type commands. For a solo dev without a marketing budget, this distribution channel is hard to beat.

## Try It

The bot is live at [@CoinPingAlertBot](https://t.me/CoinPingAlertBot). Open it on Telegram, type `/start`, and set your first alert. The source is at [github.com/SeigeC/coinping-bot](https://github.com/SeigeC/coinping-bot) — single `go build`, one Dockerfile, deploy anywhere.

If you've got ideas for alert conditions or features you'd actually use, open an issue or find me on GitHub. I'm building this in public, one alert type at a time.

---

## Social Media Versions

### Twitter/X Thread (3 tweets)

**Tweet 1:**

I built a crypto alert bot in Go that costs $0 to run.

Go + telebot v3 + SQLite (pure Go, no CGo) + CoinGecko free API + Render free tier.

Price thresholds, 24h % change, cross-exchange spread. Live at @CoinPingAlertBot.

Thread on the interesting parts of the alert engine.


**Tweet 2:**

The alert evaluation is dead simple but covers 3 conditions most bots miss:

1. Price threshold (with direction)
2. 24h % change (absolute threshold)
3. Cross-exchange spread (CoinGecko vs Binance)

Trigger once, deactivate, no spam. All in a single goroutine loop with a ticker.


**Tweet 3:**

$0 budget takeaways:

- Pure-Go SQLite = cheat code (no CGo, single binary)
- Telegram has zero user acquisition friction
- 2 goroutines + channels, no Redis needed

Code: github.com/SeigeC/coinping-bot
Bot: t.me/CoinPingAlertBot
