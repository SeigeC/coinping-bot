package main

import (
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/telebot.v3"
)

var coinAliases = map[string]string{
	"btc":  "bitcoin",
	"eth":  "ethereum",
	"sol":  "solana",
	"usdt": "tether",
	"bnb":  "binancecoin",
	"xrp":  "ripple",
	"ada":  "cardano",
	"doge": "dogecoin",
}

func resolveCoinID(input string) string {
	id := strings.ToLower(strings.TrimSpace(input))
	if mapped, ok := coinAliases[id]; ok {
		return mapped
	}
	return id
}

func HandleStart(c telebot.Context) error {
	return c.Send(`👋 Welcome to CoinPing!

Stop refreshing charts. Set alerts, get pinged when they hit.

🚀 Quick start:
/price BTC  — live price
/top  — top 10 by market cap
/alert price BTC 65000 up  — breakout alert
/alert change BTC 5  — volatility alert (24h move >5%)
/alert spread BTC 0.5  — exchange spread alert

📋 Manage:
/alerts  — your active alerts
/delalert <id>  — delete an alert
/history  — recently triggered alerts
/digest on  — daily market summary
/digest time HH:00  — set digest time (whole hour)

Free tier: 3 active alerts. /upgrade for unlimited alerts. Happy trading! 📈`)
}

func HandleHelp(c telebot.Context) error {
	return c.Send(`Available commands:
/start — welcome & usage
/price <coin> — current USD price (supports btc, eth, sol, etc.)
/top — top 10 coins by market cap
/alert — alert subcommands:
    /alert price <coin> <price> [up|down]  — price threshold alert
    /alert change <coin> <percent>         — 24h change alert
    /alert spread <coin> <percent>         — cross-exchange spread alert
/alerts — list your active alerts
/delalert <id> — delete an alert
/digest on|off|time <HH:00> — daily digest settings
/history — recently triggered alerts
/help — this message`)
}

func HandlePrice(c telebot.Context, cg *CoinGeckoClient) error {
	args := c.Args()
	if len(args) == 0 {
		return c.Send("Usage: /price <coin>\nExample: /price btc")
	}
	coinID := resolveCoinID(args[0])

	price, err := cg.GetSimplePrice(coinID)
	if err != nil {
		return c.Send(fmt.Sprintf("Failed to fetch price for %q: %v", args[0], err))
	}
	return c.Send(fmt.Sprintf("%s: $%s", strings.ToUpper(args[0]), formatMoney(price)))
}

func HandleTop(c telebot.Context, cg *CoinGeckoClient) error {
	coins, err := cg.GetTopCoins(10)
	if err != nil {
		return c.Send(fmt.Sprintf("Failed to fetch top coins: %v", err))
	}

	var sb strings.Builder
	sb.WriteString("Top 10 by market cap:\n\n")
	for i, coin := range coins {
		change := ""
		if coin.PriceChangePercentage24h != 0 {
			sign := "+"
			if coin.PriceChangePercentage24h < 0 {
				sign = ""
			}
			change = fmt.Sprintf(" (%s%.2f%%)", sign, coin.PriceChangePercentage24h)
		}
		sb.WriteString(fmt.Sprintf("%2d. %-6s $%s%s\n", i+1, strings.ToUpper(coin.Symbol), formatMoney(coin.CurrentPrice), change))
	}
	return c.Send(sb.String())
}

func HandleAlert(c telebot.Context, engine *AlertEngine) error {
	userID := c.Sender().ID
	if err := EnsureUser(userID, c.Sender().Username); err != nil {
		return c.Send(fmt.Sprintf("Failed to register user: %v", err))
	}

	args := c.Args()
	if len(args) == 0 {
		return c.Send(`🔔 Alert commands:

/alert price <coin> <price> [up|down]  — price threshold (e.g. /alert price BTC 65000 up)
/alert change <coin> <percent>         — 24h change alert (e.g. /alert change BTC 5)
/alert spread <coin> <percent>         — cross-exchange spread (e.g. /alert spread BTC 0.5)

/alerts — list your alerts
/delalert <id> — delete an alert`)
	}

	sub := strings.ToLower(args[0])
	switch sub {
	case "price":
		return handleAlertPrice(c, userID, args[1:])
	case "change":
		return handleAlertChange(c, userID, args[1:])
	case "spread":
		return handleAlertSpread(c, userID, args[1:])
	default:
		return c.Send("Unknown alert type. Use: price, change, or spread.\nExample: /alert price BTC 65000 up")
	}
}

func handleAlertPrice(c telebot.Context, userID int64, args []string) error {
	if len(args) < 2 {
		return c.Send("Usage: /alert price <coin> <price> [up|down]\nExample: /alert price BTC 65000 up")
	}
	coin := args[0]
	price, err := strconv.ParseFloat(args[1], 64)
	if err != nil || price <= 0 {
		return c.Send("Invalid price. Example: /alert price BTC 65000 up")
	}
	direction := "up"
	if len(args) >= 3 {
		direction = strings.ToLower(args[2])
		if direction != "up" && direction != "down" {
			return c.Send("Direction must be 'up' or 'down'. Example: /alert price BTC 65000 up")
		}
	}
	if err := checkAlertLimit(userID); err != nil {
		return c.Send(err.Error())
	}
	id, err := CreateAlert(userID, coin, "price", direction, price)
	if err != nil {
		return c.Send(fmt.Sprintf("Failed to create alert: %v", err))
	}
	return c.Send(fmt.Sprintf("✅ Alert #%d created: %s %s $%s", id, strings.ToUpper(coin), direction, formatMoney(price)))
}

func handleAlertChange(c telebot.Context, userID int64, args []string) error {
	if len(args) < 2 {
		return c.Send("Usage: /alert change <coin> <percent>\nExample: /alert change BTC 5")
	}
	coin := args[0]
	percent, err := strconv.ParseFloat(args[1], 64)
	if err != nil || percent <= 0 {
		return c.Send("Invalid percent. Example: /alert change BTC 5")
	}
	if err := checkAlertLimit(userID); err != nil {
		return c.Send(err.Error())
	}
	id, err := CreateAlert(userID, coin, "change", "either", percent)
	if err != nil {
		return c.Send(fmt.Sprintf("Failed to create alert: %v", err))
	}
	return c.Send(fmt.Sprintf("✅ Alert #%d created: %s 24h change ≥ %.2f%%", id, strings.ToUpper(coin), percent))
}

func handleAlertSpread(c telebot.Context, userID int64, args []string) error {
	if len(args) < 2 {
		return c.Send("Usage: /alert spread <coin> <percent>\nExample: /alert spread BTC 0.5")
	}
	coin := args[0]
	percent, err := strconv.ParseFloat(args[1], 64)
	if err != nil || percent <= 0 {
		return c.Send("Invalid percent. Example: /alert spread BTC 0.5")
	}
	if err := checkAlertLimit(userID); err != nil {
		return c.Send(err.Error())
	}
	id, err := CreateAlert(userID, coin, "spread", "either", percent)
	if err != nil {
		return c.Send(fmt.Sprintf("Failed to create alert: %v", err))
	}
	return c.Send(fmt.Sprintf("✅ Alert #%d created: %s cross-exchange spread ≥ %.2f%%", id, strings.ToUpper(coin), percent))
}

func checkAlertLimit(userID int64) error {
	premium, err := IsUserPremium(userID)
	if err != nil {
		return fmt.Errorf("check user status: %w", err)
	}
	if premium {
		return nil
	}
	count, err := GetUserAlertCount(userID)
	if err != nil {
		return fmt.Errorf("check alert count: %w", err)
	}
	if count >= freeAlertLimit {
		return fmt.Errorf("❌ Free tier limit reached (%d active alerts). Upgrade to premium for unlimited alerts.", freeAlertLimit)
	}
	return nil
}

func HandleAlerts(c telebot.Context) error {
	userID := c.Sender().ID
	if err := EnsureUser(userID, c.Sender().Username); err != nil {
		return c.Send(fmt.Sprintf("Failed to register user: %v", err))
	}
	alerts, err := GetUserAlerts(userID)
	if err != nil {
		return c.Send(fmt.Sprintf("Failed to load alerts: %v", err))
	}
	if len(alerts) == 0 {
		return c.Send("You have no active alerts.\nCreate one with /alert")
	}
	var sb strings.Builder
	sb.WriteString("Your active alerts:\n\n")
	for _, a := range alerts {
		sb.WriteString(formatAlertLine(a))
		sb.WriteString("\n")
	}
	sb.WriteString("\nDelete with /delalert <id>")
	return c.Send(sb.String())
}

func formatAlertLine(a Alert) string {
	switch a.AlertType {
	case "price":
		return fmt.Sprintf("#%d  %s price %s $%s", a.ID, strings.ToUpper(a.Coin), a.Direction, formatMoney(a.Threshold))
	case "change":
		return fmt.Sprintf("#%d  %s 24h change ≥ %.2f%%", a.ID, strings.ToUpper(a.Coin), a.Threshold)
	case "spread":
		return fmt.Sprintf("#%d  %s spread ≥ %.2f%%", a.ID, strings.ToUpper(a.Coin), a.Threshold)
	}
	return fmt.Sprintf("#%d  %s", a.ID, strings.ToUpper(a.Coin))
}

func HandleDelAlert(c telebot.Context) error {
	userID := c.Sender().ID
	if err := EnsureUser(userID, c.Sender().Username); err != nil {
		return c.Send(fmt.Sprintf("Failed to register user: %v", err))
	}
	args := c.Args()
	if len(args) == 0 {
		return c.Send("Usage: /delalert <id>\nExample: /delalert 3")
	}
	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return c.Send("Invalid alert id. Example: /delalert 3")
	}
	if err := DeleteAlert(userID, id); err != nil {
		return c.Send(err.Error())
	}
	return c.Send(fmt.Sprintf("✅ Alert #%d deleted.", id))
}

func HandleUpgrade(c telebot.Context) error {
	userID := c.Sender().ID
	if err := EnsureUser(userID, c.Sender().Username); err != nil {
		return c.Send(fmt.Sprintf("Failed: %v", err))
	}
	premium, err := IsUserPremium(userID)
	if err != nil {
		return c.Send(fmt.Sprintf("Failed: %v", err))
	}
	if premium {
		return c.Send("You already have premium — unlimited alerts. 🎉")
	}
	invoice := &telebot.Invoice{
		Title:       "CoinPing Premium",
		Description: "Unlimited alerts — no cap, no limits. One-time purchase, permanent upgrade.",
		Payload:     "premium_upgrade",
		Currency:    "XTR",
		Token:       "",
		Prices: []telebot.Price{
			{Label: "Premium Upgrade", Amount: 100},
		},
	}
	return c.Send(invoice)
}

func HandleCheckout(c telebot.Context) error {
	return c.Accept()
}

func HandlePayment(c telebot.Context) error {
	payment := c.Message().Payment
	userID := c.Sender().ID

	if payment.Payload == "premium_upgrade" {
		if err := SetUserPremium(userID, true); err != nil {
			return c.Send(fmt.Sprintf("Payment succeeded but upgrade failed — contact support. (%v)", err))
		}
		return c.Send("✅ Payment received! You now have **CoinPing Premium** — unlimited alerts, no limits. Enjoy!")
	}
	return c.Send("✅ Payment received! Thank you for your support.")
}

func HandleDigest(c telebot.Context) error {
	userID := c.Sender().ID
	if err := EnsureUser(userID, c.Sender().Username); err != nil {
		return c.Send(fmt.Sprintf("Failed to register user: %v", err))
	}
	args := c.Args()
	if len(args) == 0 {
		s, err := GetSettings(userID)
		if err != nil {
			return c.Send(fmt.Sprintf("Failed to read settings: %v", err))
		}
		state := "off"
		if s.DailyDigest {
			state = "on"
		}
		return c.Send(fmt.Sprintf("Daily digest is %s (at %s %s).\nToggle with /digest on or /digest off\nSet time with /digest time HH:00 (whole hour)", state, s.DigestTime, s.Timezone))
	}
	switch strings.ToLower(args[0]) {
	case "on":
		if err := SetDigest(userID, true); err != nil {
			return c.Send(fmt.Sprintf("Failed: %v", err))
		}
		return c.Send("✅ Daily digest enabled. You'll receive a market summary each day.")
	case "off":
		if err := SetDigest(userID, false); err != nil {
			return c.Send(fmt.Sprintf("Failed: %v", err))
		}
		return c.Send("✅ Daily digest disabled.")
	case "time":
		return handleDigestTime(c, userID, args[1:])
	default:
		return c.Send("Usage: /digest on|off|time <HH:00>")
	}
}

func handleDigestTime(c telebot.Context, userID int64, args []string) error {
	if len(args) == 0 {
		return c.Send("Usage: /digest time <HH:00>\nExample: /digest time 09:00")
	}
	raw := strings.TrimSpace(args[0])
	parts := strings.SplitN(raw, ":", 2)
	if len(parts) != 2 || parts[1] != "00" {
		return c.Send("Time must be on the hour, e.g. 09:00 or 22:00")
	}
	hour, err := strconv.Atoi(parts[0])
	if err != nil || hour < 0 || hour > 23 {
		return c.Send("Hour must be 00–23. Example: /digest time 09:00")
	}
	formatted := fmt.Sprintf("%02d:00", hour)
	if err := SetDigestTime(userID, formatted); err != nil {
		return c.Send(fmt.Sprintf("Failed: %v", err))
	}
	return c.Send(fmt.Sprintf("✅ Daily digest time set to %s UTC.", formatted))
}

func HandleHistory(c telebot.Context) error {
	userID := c.Sender().ID
	if err := EnsureUser(userID, c.Sender().Username); err != nil {
		return c.Send(fmt.Sprintf("Failed to register user: %v", err))
	}
	const historyLimit = 20
	alerts, err := GetTriggeredAlerts(userID, historyLimit)
	if err != nil {
		return c.Send(fmt.Sprintf("Failed to load history: %v", err))
	}
	if len(alerts) == 0 {
		return c.Send("No triggered alerts yet.\nSet an alert with /alert and wait for it to fire.")
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📜 Last %d triggered alerts:\n\n", len(alerts)))
	for _, a := range alerts {
		sb.WriteString(formatHistoryLine(a))
		sb.WriteString("\n")
	}
	return c.Send(sb.String())
}

func formatHistoryLine(a Alert) string {
	if !a.TriggeredAt.Valid {
		return fmt.Sprintf("#%d  %s (no trigger time)", a.ID, strings.ToUpper(a.Coin))
	}
	t := a.TriggeredAt.String
	if len(t) > 16 {
		t = t[:16]
	}
	switch a.AlertType {
	case "price":
		return fmt.Sprintf("#%d  %s %s $%s  ·  fired %s", a.ID, strings.ToUpper(a.Coin), a.Direction, formatMoney(a.Threshold), t)
	case "change":
		return fmt.Sprintf("#%d  %s 24h change ≥ %.2f%%  ·  fired %s", a.ID, strings.ToUpper(a.Coin), a.Threshold, t)
	case "spread":
		return fmt.Sprintf("#%d  %s spread ≥ %.2f%%  ·  fired %s", a.ID, strings.ToUpper(a.Coin), a.Threshold, t)
	}
	return fmt.Sprintf("#%d  %s  ·  fired %s", a.ID, strings.ToUpper(a.Coin), t)
}
