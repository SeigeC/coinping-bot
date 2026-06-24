package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
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

	engine := NewAlertEngine(b, cg)

	b.Handle("/start", HandleStart)
	b.Handle("/help", HandleHelp)
	b.Handle("/price", func(c telebot.Context) error { return HandlePrice(c, cg) })
	b.Handle("/top", func(c telebot.Context) error { return HandleTop(c, cg) })
	b.Handle("/alert", func(c telebot.Context) error { return HandleAlert(c, engine) })
	b.Handle("/alerts", HandleAlerts)
	b.Handle("/delalert", HandleDelAlert)
	b.Handle("/digest", HandleDigest)

	stopCh := make(chan struct{})
	go func() {
		<-sigCh
		fmt.Println("shutting down")
		close(stopCh)
		b.Stop()
	}()

	go runAlertChecker(engine, stopCh)
	go runDailyDigest(engine, stopCh)

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
