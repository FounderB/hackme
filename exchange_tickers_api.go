package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	defaultExchangeTickersURL = "https://exchange-api.hackme.tech/tickers"
	exchangeTickersCacheTTL   = 2 * time.Second
	exchangeTickersFetchTO    = 6 * time.Second
	exchangeTickersMaxBytes   = 1 << 20
)

type exchangeTickersCache struct {
	mu      sync.Mutex
	body    []byte
	fetched time.Time
	err     string
}

var globalExchangeTickersCache exchangeTickersCache

func exchangeTickersUpstreamURL() string {
	if u := strings.TrimSpace(os.Getenv("HACKME_EXCHANGE_TICKERS_URL")); u != "" {
		return u
	}
	return defaultExchangeTickersURL
}

func fetchExchangeTickersJSON(ctx context.Context) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, exchangeTickersUpstreamURL(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "HackMe-Node/exchange-tickers-proxy")
	client := &http.Client{Timeout: exchangeTickersFetchTO}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, exchangeTickersMaxBytes))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &httpStatusError{code: resp.StatusCode, body: string(raw)}
	}
	if !json.Valid(raw) {
		return nil, errInvalidJSON
	}
	return raw, nil
}

type httpStatusError struct {
	code int
	body string
}

func (e *httpStatusError) Error() string {
	if e == nil {
		return "http error"
	}
	msg := strings.TrimSpace(e.body)
	if len(msg) > 120 {
		msg = msg[:120]
	}
	if msg == "" {
		return "upstream HTTP " + http.StatusText(e.code)
	}
	return "upstream HTTP " + http.StatusText(e.code) + ": " + msg
}

var errInvalidJSON = errString("invalid upstream json")

type errString string

func (e errString) Error() string { return string(e) }

func cachedExchangeTickers(ctx context.Context) ([]byte, string, bool) {
	c := &globalExchangeTickersCache
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.body) > 0 && time.Since(c.fetched) < exchangeTickersCacheTTL {
		return append([]byte(nil), c.body...), c.err, true
	}
	raw, err := fetchExchangeTickersJSON(ctx)
	if err != nil {
		c.err = err.Error()
		if len(c.body) > 0 {
			// Serve last good body briefly when upstream blips.
			return append([]byte(nil), c.body...), c.err, true
		}
		return nil, c.err, false
	}
	c.body = raw
	c.fetched = time.Now()
	c.err = ""
	return append([]byte(nil), raw...), "", true
}

// handleExchangeTickers proxies public paper-desk tickers for same-origin dashboard
// price widgets (wallet portfolio, mining calculator). Upstream has no browser CORS.
func (a *app) handleExchangeTickers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !a.allowRate("exchange_tickers:"+clientIP(r), 8) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), exchangeTickersFetchTO)
	defer cancel()
	raw, staleErr, ok := cachedExchangeTickers(ctx)
	if !ok {
		writeJSON(w, map[string]any{
			"ok":     false,
			"error":  staleErr,
			"source": exchangeTickersUpstreamURL(),
		})
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if staleErr != "" {
		w.Header().Set("X-HackMe-Tickers-Stale", "1")
		w.Header().Set("X-HackMe-Tickers-Error", staleErr)
	}
	w.Header().Set("X-HackMe-Tickers-Source", exchangeTickersUpstreamURL())
	_, _ = w.Write(raw)
}
