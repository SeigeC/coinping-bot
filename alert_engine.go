package main

import (
	"fmt"
	"log"
	"strings"

	"gopkg.in/telebot.v3"
)

type AlertEngine struct {
	cg  *CoinGeckoClient
	bot *telebot.Bot
}

func NewAlertEngine(bot *telebot.Bot, cg *CoinGeckoClient) *AlertEngine {
	return &AlertEngine{cg: cg, bot: bot}
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
	price, err := e.cg.GetSimplePrice(coinID)
	if err != nil {
		return err
	}
	threshold := a.Threshold
	if threshold == 0 {
		return nil
	}
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

func (e *AlertEngine) checkChange(a Alert, coinID, displaySym string) error {
	change, err := e.cg.Get24hChange(coinID)
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
	coins, err := e.cg.GetTopCoins(10)
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
