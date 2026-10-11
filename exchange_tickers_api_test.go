package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFetchExchangeTickersJSON_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"tickers":[{"pair":"HMC/USDT","last":8850216}]}`))
	}))
	defer srv.Close()
	t.Setenv("HACKME_EXCHANGE_TICKERS_URL", srv.URL)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	raw, err := fetchExchangeTickersJSON(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["ok"] != true {
		t.Fatalf("ok=%v", payload["ok"])
	}
}

func TestHandleExchangeTickers_Caches(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"tickers":[{"pair":"HMC/USDT","last":100000000}]}`))
	}))
	defer srv.Close()
	t.Setenv("HACKME_EXCHANGE_TICKERS_URL", srv.URL)
	globalExchangeTickersCache = exchangeTickersCache{}

	a := &app{}
	req := httptest.NewRequest(http.MethodGet, "/api/exchange/tickers", nil)
	rr := httptest.NewRecorder()
	a.handleExchangeTickers(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	req2 := httptest.NewRequest(http.MethodGet, "/api/exchange/tickers", nil)
	rr2 := httptest.NewRecorder()
	a.handleExchangeTickers(rr2, req2)
	if hits != 1 {
		t.Fatalf("expected 1 upstream hit (cache), got %d", hits)
	}
	var payload map[string]any
	if err := json.Unmarshal(rr2.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload["ok"] != true {
		t.Fatalf("ok=%v", payload["ok"])
	}
}
