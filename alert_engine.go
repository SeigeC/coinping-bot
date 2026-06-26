package main

import (
	"fmt"
	"log"
	"strings"
	"time"

	"gopkg.in/telebot.v3"
)

const priceFreshnessWindow = 5 * time.Second
const multiExchangeCheckInterval = 10 * time.Minute

type PriceFeed struct {
	binance *BinanceFeed
	cg      *CoinGeckoClient
}

func NewPriceFeed(binance *BinanceFeed, cg *CoinGeckoClient) *PriceFeed {
	return &PriceFeed{binance: binance, cg: cg}
}

func (pf *PriceFeed) CurrentPrice(coinID string) (float64, string) {
	if pf.binance != nil {
		if p, ok := pf.binance.GetPriceFresh(coinID, priceFreshnessWindow); ok {
			return p, "binance_ws"
		}
	}
	p, err := pf.cg.GetSimplePrice(coinID)
	if err != nil {
		return 0, ""
	}
	return p, "coingecko"
}

func (pf *PriceFeed) CurrentPriceForUser(coinID string, userID int64) (float64, string) {
	premium, err := IsUserPremium(userID)
	if err != nil || !premium {
		p, err := pf.cg.GetSimplePrice(coinID)
		if err != nil {
			return 0, ""
		}
		return p, "coingecko"
	}
	return pf.CurrentPrice(coinID)
}

type AlertEngine struct {
	feed *PriceFeed
	bot  *telebot.Bot
}

func NewAlertEngine(bot *telebot.Bot, feed *PriceFeed) *AlertEngine {
	return &AlertEngine{feed: feed, bot: bot}
}

const freeAlertLimit = 3

func (e *AlertEngine) CheckAlerts() {
	alerts, err := GetActiveAlerts()
	if err != nil {
		log.Printf("alert engine: load alerts: %v", err)
		return
	}
	for _, a := range alerts {
		if err := e.checkOne(a); err != nil {
			log.Printf("alert engine: alert %d: %v", a.ID, err)
		}
	}
}

func (e *AlertEngine) checkOne(a Alert) error {
	coinID := resolveCoinID(a.Coin)
	displaySym := strings.ToUpper(a.Coin)

	switch a.AlertType {
	case "price":
		return e.checkPrice(a, coinID, displaySym)
	case "change":
		return e.checkChange(a, coinID, displaySym)
	case "spread":
		return e.checkSpread(a, coinID, displaySym)
	}
	return nil
}

func (e *AlertEngine) checkPrice(a Alert, coinID, displaySym string) error {
	price, source := e.feed.CurrentPriceForUser(coinID, a.UserID)
	if price == 0 {
		return fmt.Errorf("no price available from any source")
	}
	threshold := a.Threshold
	if threshold == 0 {
		return nil
	}
	if (a.Direction == "up" || a.Direction == "either") && price >= threshold {
		return e.trigger(a, fmt.Sprintf(
			"🔔 %s 上穿 $%s！当前价格 $%s (via %s)",
			displaySym, formatMoney(threshold), formatMoney(price), source,
		))
	}
	if (a.Direction == "down" || a.Direction == "either") && price <= threshold {
		return e.trigger(a, fmt.Sprintf(
			"🔔 %s 下穿 $%s！当前价格 $%s (via %s)",
			displaySym, formatMoney(threshold), formatMoney(price), source,
		))
	}
	return nil
}

func (e *AlertEngine) checkChange(a Alert, coinID, displaySym string) error {
	change, err := e.feed.cg.Get24hChange(coinID)
	if err != nil {
		return err
	}
	threshold := a.Threshold
	if threshold == 0 {
		return nil
	}
	absChange := change
	if absChange < 0 {
		absChange = -absChange
	}
	if absChange >= threshold {
		sign := "+"
		if change < 0 {
			sign = ""
		}
		return e.trigger(a, fmt.Sprintf(
			"🔔 %s 24h 涨跌幅 %s%.2f%% 触达 %.2f%% 阈值",
			displaySym, sign, change, threshold,
		))
	}
	return nil
}

func (e *AlertEngine) checkSpread(a Alert, coinID, displaySym string) error {
	prices, err := e.feed.cg.GetPriceMultiExchange(a.Coin)
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
	if min == 0 {
		return nil
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

func (e *AlertEngine) CheckMultiExchangeSpreads() {
	alerts, err := GetActiveAlerts()
	if err != nil {
		log.Printf("multi-exchange: load alerts: %v", err)
		return
	}

	checked := map[string]bool{}
	for _, a := range alerts {
		coinID := resolveCoinID(a.Coin)
		if checked[coinID] {
			continue
		}
		checked[coinID] = true

		prices, err := e.feed.cg.GetExchangeTickers(coinID)
		if err != nil {
			log.Printf("multi-exchange: %s: %v", coinID, err)
			continue
		}
		if len(prices) < 2 {
			continue
		}

		var min, max float64
		first := true
		for _, p := range prices {
			if first {
				min, max = p, p
				first = false
				continue
			}
			if p < min {
				min = p
			}
			if p > max {
				max = p
			}
		}
		if min == 0 {
			continue
		}
		spreadPct := (max - min) / min * 100

		spreadAlerts := filterSpreadAlerts(alerts, coinID)
		for _, sa := range spreadAlerts {
			if spreadPct >= sa.Threshold {
				e.triggerSpreadAlert(sa, spreadPct, prices)
			}
		}
	}
}

func filterSpreadAlerts(alerts []Alert, coinID string) []Alert {
	var out []Alert
	coinLower := strings.ToLower(coinID)
	for _, a := range alerts {
		if a.AlertType == "spread" && strings.ToLower(resolveCoinID(a.Coin)) == coinLower && a.Active {
			out = append(out, a)
		}
	}
	return out
}

func (e *AlertEngine) triggerSpreadAlert(a Alert, spreadPct float64, prices map[string]float64) {
	var exchanges []string
	for ex := range prices {
		exchanges = append(exchanges, ex)
	}
	msg := fmt.Sprintf(
		"🔔 %s 多所价差 %.2f%% 超过 %.2f%% 阈值\n交易所价格: %s",
		strings.ToUpper(a.Coin), spreadPct, a.Threshold,
		formatExchangePrices(prices),
	)
	if err := DeactivateAlert(a.ID); err != nil {
		log.Printf("multi-exchange: deactivate alert %d: %v", a.ID, err)
		return
	}
	recipient := &telebot.User{ID: a.UserID}
	if _, err := e.bot.Send(recipient, msg); err != nil {
		log.Printf("multi-exchange: send to user %d: %v", a.UserID, err)
	}
}

func formatExchangePrices(prices map[string]float64) string {
	var parts []string
	for ex, p := range prices {
		parts = append(parts, fmt.Sprintf("%s: $%s", strings.ToUpper(ex), formatMoney(p)))
	}
	return strings.Join(parts, ", ")
}

func (e *AlertEngine) trigger(a Alert, msg string) error {
	if err := DeactivateAlert(a.ID); err != nil {
		return fmt.Errorf("deactivate: %w", err)
	}
	recipient := &telebot.User{ID: a.UserID}
	if _, err := e.bot.Send(recipient, msg); err != nil {
		return fmt.Errorf("send: %w", err)
	}
	log.Printf("alert engine: triggered alert %d for user %d", a.ID, a.UserID)
	return nil
}

func (e *AlertEngine) SendDailyDigest(userID int64) {
	coins, err := e.feed.cg.GetTopCoins(10)
	if err != nil {
		log.Printf("digest user %d: fetch top: %v", userID, err)
		return
	}
	var sb strings.Builder
	sb.WriteString("📊 每日行情摘要\n\n")
	for _, coin := range coins {
		change := ""
		if coin.PriceChangePercentage24h != 0 {
			sign := "+"
			if coin.PriceChangePercentage24h < 0 {
				sign = ""
			}
			change = fmt.Sprintf(" (%s%.2f%%)", sign, coin.PriceChangePercentage24h)
		}
		sb.WriteString(fmt.Sprintf("%-6s $%s%s\n", strings.ToUpper(coin.Symbol), formatMoney(coin.CurrentPrice), change))
	}
	sb.WriteString("\n— CoinPing")
	recipient := &telebot.User{ID: userID}
	if _, err := e.bot.Send(recipient, sb.String()); err != nil {
		log.Printf("digest user %d: send: %v", userID, err)
	}
}

func formatMoney(v float64) string {
	s := fmt.Sprintf("%.2f", v)
	parts := strings.SplitN(s, ".", 2)
	intPart := parts[0]
	neg := false
	if strings.HasPrefix(intPart, "-") {
		neg = true
		intPart = intPart[1:]
	}
	n := len(intPart)
	if n <= 3 {
		if neg {
			return "-" + s
		}
		return s
	}
	var b strings.Builder
	pre := n % 3
	if pre > 0 {
		b.WriteString(intPart[:pre])
		if n > pre {
			b.WriteString(",")
		}
	}
	for i := pre; i < n; i += 3 {
		b.WriteString(intPart[i : i+3])
		if i+3 < n {
			b.WriteString(",")
		}
	}
	out := b.String()
	if neg {
		out = "-" + out
	}
	if len(parts) == 2 {
		out += "." + parts[1]
	}
	return out
}
