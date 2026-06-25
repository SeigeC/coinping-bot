package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type PricePoint struct {
	Price     float64
	Timestamp time.Time
}

type BinanceFeed struct {
	mu     sync.RWMutex
	prices map[string]PricePoint // coin (lowercase) → latest trade price
	done   chan struct{}
}

func NewBinanceFeed() *BinanceFeed {
	return &BinanceFeed{
		prices: make(map[string]PricePoint),
		done:   make(chan struct{}),
	}
}

func (bf *BinanceFeed) Start(coins []string) {
	go bf.connect(coins)
}

func (bf *BinanceFeed) Stop() {
	close(bf.done)
}

func (bf *BinanceFeed) GetPrice(coin string) (float64, bool) {
	bf.mu.RLock()
	defer bf.mu.RUnlock()
	pp, ok := bf.prices[strings.ToLower(coin)]
	if !ok {
		return 0, false
	}
	return pp.Price, true
}

func (bf *BinanceFeed) GetPriceFresh(coin string, maxAge time.Duration) (float64, bool) {
	bf.mu.RLock()
	defer bf.mu.RUnlock()
	pp, ok := bf.prices[strings.ToLower(coin)]
	if !ok {
		return 0, false
	}
	if time.Since(pp.Timestamp) > maxAge {
		return 0, false
	}
	return pp.Price, true
}

func (bf *BinanceFeed) ActiveCoins() []string {
	bf.mu.RLock()
	defer bf.mu.RUnlock()
	coins := make([]string, 0, len(bf.prices))
	for c := range bf.prices {
		coins = append(coins, c)
	}
	return coins
}

func (bf *BinanceFeed) connect(coins []string) {
	baseURL := "wss://data-stream.binance.vision:443/ws"
	restBase := "https://data-api.binance.vision/api/v3/ticker/price"

	for {
		select {
		case <-bf.done:
			return
		default:
		}

		conn, _, err := websocket.DefaultDialer.Dial(baseURL, nil)
		if err != nil {
			log.Printf("binance ws: dial: %v, falling back to REST", err)
			bf.pollRest(restBase, coins)
			time.Sleep(30 * time.Second)
			continue
		}

		if err := bf.subscribe(conn, coins); err != nil {
			log.Printf("binance ws: subscribe: %v", err)
			conn.Close()
			time.Sleep(10 * time.Second)
			continue
		}

		log.Printf("binance ws: connected, subscribed to %d coins", len(coins))
		bf.readLoop(conn)
		conn.Close()

		select {
		case <-bf.done:
			return
		default:
		}
		log.Printf("binance ws: disconnected, reconnecting in 5s...")
		time.Sleep(5 * time.Second)
	}
}

func (bf *BinanceFeed) subscribe(conn *websocket.Conn, coins []string) error {
	streams := make([]string, 0, len(coins))
	for _, coin := range coins {
		streams = append(streams, strings.ToLower(coin)+"usdt@trade")
	}
	msg := map[string]interface{}{
		"method": "SUBSCRIBE",
		"params": streams,
		"id":     1,
	}
	return conn.WriteJSON(msg)
}

func (bf *BinanceFeed) readLoop(conn *websocket.Conn) {
	for {
		select {
		case <-bf.done:
			return
		default:
		}

		_, raw, err := conn.ReadMessage()
		if err != nil {
			log.Printf("binance ws: read: %v", err)
			return
		}

		var msg struct {
			Stream string `json:"stream"`
			Data   struct {
				Symbol string `json:"s"`
				Price  string `json:"p"`
			} `json:"data"`
		}
		if err := json.Unmarshal(raw, &msg); err != nil {
			continue
		}
		if msg.Data.Symbol == "" || msg.Data.Price == "" {
			continue
		}

		price, err := strconv.ParseFloat(msg.Data.Price, 64)
		if err != nil {
			continue
		}

		coin := strings.TrimSuffix(strings.ToLower(msg.Data.Symbol), "usdt")
		bf.mu.Lock()
		bf.prices[coin] = PricePoint{Price: price, Timestamp: time.Now()}
		bf.mu.Unlock()
	}
}

func (bf *BinanceFeed) pollRest(restBase string, coins []string) {
	for _, coin := range coins {
		symbol := strings.ToUpper(coin) + "USDT"
		url := fmt.Sprintf("%s?symbol=%s", restBase, symbol)
		resp, err := httpClient.Get(url)
		if err != nil {
			log.Printf("binance rest: %s: %v", symbol, err)
			continue
		}
		var r struct {
			Symbol string `json:"symbol"`
			Price  string `json:"price"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
			resp.Body.Close()
			continue
		}
		resp.Body.Close()
		price, err := strconv.ParseFloat(r.Price, 64)
		if err != nil {
			continue
		}
		coinKey := strings.TrimSuffix(strings.ToLower(r.Symbol), "usdt")
		bf.mu.Lock()
		bf.prices[coinKey] = PricePoint{Price: price, Timestamp: time.Now()}
		bf.mu.Unlock()
	}
}

var httpClient = &http.Client{Timeout: 10 * time.Second}
