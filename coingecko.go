package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type CoinGeckoClient struct {
	baseURL string
	client  *http.Client
}

type CoinInfo struct {
	ID                       string  `json:"id"`
	Symbol                   string  `json:"symbol"`
	Name                     string  `json:"name"`
	CurrentPrice             float64 `json:"current_price"`
	PriceChangePercentage24h float64 `json:"price_change_percentage_24h"`
}

func NewCoinGeckoClient(baseURL string) *CoinGeckoClient {
	return &CoinGeckoClient{
		baseURL: baseURL,
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *CoinGeckoClient) GetSimplePrice(coinID string) (float64, error) {
	url := fmt.Sprintf("%s/simple/price?ids=%s&vs_currencies=usd", c.baseURL, coinID)
	body, err := c.get(url)
	if err != nil {
		return 0, err
	}

	var resp map[string]map[string]float64
	if err := json.Unmarshal(body, &resp); err != nil {
		return 0, fmt.Errorf("parse price response: %w", err)
	}
	entry, ok := resp[coinID]
	if !ok {
		return 0, fmt.Errorf("coin %q not found in response", coinID)
	}
	price, ok := entry["usd"]
	if !ok {
		return 0, fmt.Errorf("usd price missing for %q", coinID)
	}
	return price, nil
}

func (c *CoinGeckoClient) GetTopCoins(limit int) ([]CoinInfo, error) {
	url := fmt.Sprintf("%s/coins/markets?vs_currency=usd&order=market_cap_desc&per_page=%d&page=1", c.baseURL, limit)
	body, err := c.get(url)
	if err != nil {
		return nil, err
	}

	var coins []CoinInfo
	if err := json.Unmarshal(body, &coins); err != nil {
		return nil, fmt.Errorf("parse markets response: %w", err)
	}
	return coins, nil
}

func (c *CoinGeckoClient) Get24hChange(coinID string) (float64, error) {
	url := fmt.Sprintf("%s/simple/price?ids=%s&vs_currencies=usd&include_24hr_change=true", c.baseURL, coinID)
	body, err := c.get(url)
	if err != nil {
		return 0, err
	}

	var resp map[string]map[string]float64
	if err := json.Unmarshal(body, &resp); err != nil {
		return 0, fmt.Errorf("parse change response: %w", err)
	}
	entry, ok := resp[coinID]
	if !ok {
		return 0, fmt.Errorf("coin %q not found in response", coinID)
	}
	change, ok := entry["usd_24h_change"]
	if !ok {
		return 0, fmt.Errorf("24h change missing for %q", coinID)
	}
	return change, nil
}

func (c *CoinGeckoClient) GetPriceMultiExchange(coinSymbol string) (map[string]float64, error) {
	cgPrice, err := c.GetSimplePrice(resolveCoinID(coinSymbol))
	if err != nil {
		return nil, fmt.Errorf("coingecko: %w", err)
	}

	binancePrice, binanceErr := c.getBinancePrice(strings.ToUpper(coinSymbol) + "USDT")
	if binanceErr != nil {
		return map[string]float64{"coingecko": cgPrice}, nil
	}
	return map[string]float64{
		"coingecko": cgPrice,
		"binance":   binancePrice,
	}, nil
}

func (c *CoinGeckoClient) getBinancePrice(symbol string) (float64, error) {
	url := "https://api.binance.com/api/v3/ticker/price?symbol=" + symbol
	body, err := c.get(url)
	if err != nil {
		return 0, fmt.Errorf("binance: %w", err)
	}

	var resp struct {
		Symbol string `json:"symbol"`
		Price  string `json:"price"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return 0, fmt.Errorf("parse binance response: %w", err)
	}
	price, err := strconv.ParseFloat(resp.Price, 64)
	if err != nil {
		return 0, fmt.Errorf("parse binance price: %w", err)
	}
	return price, nil
}

func (c *CoinGeckoClient) get(url string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("coin gecko request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("coin gecko returned status %d: %s", resp.StatusCode, string(body))
	}
	return io.ReadAll(resp.Body)
}
