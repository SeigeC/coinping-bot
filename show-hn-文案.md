# Hacker News Show HN 发布文案

> Show HN 专区的规则：产品必须是自己做的、别人可以试用的。CoinPing 完全符合。
> 注意：Show HN 不能频发，这篇文章要用最有力的版本。一次机会。

## 标题（80 字限制）

> Show HN: CoinPing – Multi-condition crypto alerts on Telegram, built in Go

备选：

> Show HN: CoinPing – A zero-budget crypto alert bot for Telegram (Go + SQLite)

## 正文

I built CoinPing because I was burning too much mental energy refreshing crypto charts. Every alert tool I found only pinged when a coin crossed a price level — useful, but not the alerts I actually wanted.

CoinPing supports three alert conditions:

- **Price threshold** — "ping me if BTC crosses $65k" (with direction: up/down/either)
- **24h % change** — "ping me if BTC moves more than 5% in a day" (catch volatility without staring at a screen)
- **Cross-exchange spread** — "ping me when CoinGecko and Binance diverge more than 0.5%" (spot wicks and liquidity events)

It runs as a single Go binary. Stack: Go + telebot v3 + CoinGecko free API + SQLite (pure Go, no CGo) + Render free tier. Total monthly infra cost: $0. Two background goroutines handle polling: one checks alerts every 5 minutes, one dispatches daily digests every minute.

Free tier is 3 active alerts per user. Paid unlimited tier planned but not live — I want to understand usage patterns before building billing.

Try it: t.me/CoinPingAlertBot — open Telegram, type /start
Source: github.com/SeigeC/coinping-bot

I'd love feedback on the alert conditions. What would actually be useful to you? Also happy to answer questions about the Go implementation — the pure-Go SQLite driver (modernc.org/sqlite) was a surprisingly nice experience.

## 发帖时机建议

- **最佳**：周二/周三 7:00-10:00 AM ET（美东）—— HN 流量巅峰
- 北京时间：周二/周三 晚上 7:00-10:00 PM
- 先发 Dev.to，再发 HN——Dev.to 文章里引 HN 讨论，HN 帖可以放 Dev.to 链接双向引流

## 注意

- HN 标题不要用 "Show HN:" 开头的 bait-y 写法，保持技术事实
- 评论区前 30 分钟必须有人互动（可以提前让朋友帮忙）
- 如果你认识 HN 活跃用户，请他们在帖子早期留言——早期互动决定是否上首页
