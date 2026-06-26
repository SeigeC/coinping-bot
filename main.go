package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"gopkg.in/telebot.v3"
)

func main() {
	cfg := LoadConfig()

	_, err := InitDB(cfg.DBPath)
	if err != nil {
		log.Fatalf("init db: %v", err)
	}
	defer CloseDB()

	cg := NewCoinGeckoClient(cfg.CoinGeckoBaseURL)

	binanceFeed := NewBinanceFeed()
	priceFeed := NewPriceFeed(binanceFeed, cg)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	if cfg.BotToken == "" {
		fmt.Println("BOT_TOKEN not set, running in dry mode")
		<-sigCh
		fmt.Println("shutting down")
		return
	}

	pref := telebot.Settings{
		Token:  cfg.BotToken,
		Poller: &telebot.LongPoller{Timeout: 10 * time.Second},
	}
	b, err := telebot.NewBot(pref)
	if err != nil {
		log.Fatalf("create bot: %v", err)
	}

	engine := NewAlertEngine(b, priceFeed)

	b.Handle("/start", HandleStart)
	b.Handle("/help", HandleHelp)
	b.Handle("/price", func(c telebot.Context) error { return HandlePrice(c, cg) })
	b.Handle("/top", func(c telebot.Context) error { return HandleTop(c, cg) })
	b.Handle("/alert", func(c telebot.Context) error { return HandleAlert(c, engine) })
	b.Handle("/alerts", HandleAlerts)
	b.Handle("/delalert", HandleDelAlert)
	b.Handle("/digest", HandleDigest)
	b.Handle("/history", HandleHistory)
	b.Handle("/upgrade", HandleUpgrade)
	b.Handle("/pro", HandlePro)
	b.Handle(telebot.OnCheckout, HandleCheckout)
	b.Handle(telebot.OnPayment, HandlePayment)

	stopCh := make(chan struct{})
	go func() {
		<-sigCh
		fmt.Println("shutting down")
		close(stopCh)
		b.Stop()
	}()

	go runAlertChecker(engine, stopCh)
	go runDailyDigest(engine, stopCh)
	go runBinanceCoinRefresh(binanceFeed, stopCh)
	go runMultiExchangeSpreadCheck(engine, stopCh)

	// Start Binance WS with initial coins from active alerts
	initialCoins := activeAlertCoins()
	binanceFeed.Start(initialCoins)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	go func() {
		http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, `{"status":"ok"}`)
		})
		log.Printf("health server listening on :%s", port)
		if err := http.ListenAndServe(":"+port, nil); err != nil {
			log.Printf("health server: %v", err)
		}
	}()

	log.Println("bot started")
	b.Start()
}

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

func runDailyDigest(engine *AlertEngine, stopCh <-chan struct{}) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			sendDigestsDue(engine)
		}
	}
}

func sendDigestsDue(engine *AlertEngine) {
	now := time.Now().UTC()
	currentTime := fmt.Sprintf("%02d:%02d", now.Hour(), now.Minute())
	subscribers, err := GetDigestSubscribers(currentTime)
	if err != nil {
		log.Printf("digest: load subscribers: %v", err)
		return
	}
	for _, userID := range subscribers {
		engine.SendDailyDigest(userID)
	}
}

func activeAlertCoins() []string {
	alerts, err := GetActiveAlerts()
	if err != nil {
		log.Printf("binance: load active alerts: %v", err)
		return nil
	}
	seen := map[string]bool{}
	var coins []string
	for _, a := range alerts {
		coin := strings.ToLower(a.Coin)
		if !seen[coin] {
			seen[coin] = true
			coins = append(coins, coin)
		}
	}
	return coins
}

func runBinanceCoinRefresh(bf *BinanceFeed, stopCh <-chan struct{}) {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			coins := activeAlertCoins()
			if len(coins) > 0 {
				bf.Start(coins)
			}
		}
	}
}

func runMultiExchangeSpreadCheck(engine *AlertEngine, stopCh <-chan struct{}) {
	ticker := time.NewTicker(multiExchangeCheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stopCh:
			return
		case <-ticker.C:
			engine.CheckMultiExchangeSpreads()
		}
	}
}
