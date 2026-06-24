package main

import (
	"fmt"
	"os"
)

type Config struct {
	BotToken         string
	DBPath           string
	CoinGeckoBaseURL string
}

func LoadConfig() Config {
	cfg := Config{
		BotToken:         os.Getenv("BOT_TOKEN"),
		DBPath:           os.Getenv("DB_PATH"),
		CoinGeckoBaseURL: os.Getenv("COINGECKO_BASE_URL"),
	}
	if cfg.DBPath == "" {
		cfg.DBPath = "./data/bot.db"
	}
	if cfg.CoinGeckoBaseURL == "" {
		cfg.CoinGeckoBaseURL = "https://api.coingecko.com/api/v3"
	}
	if cfg.BotToken == "" {
		fmt.Println("WARNING: BOT_TOKEN not set, running in placeholder mode")
	}
	return cfg
}
