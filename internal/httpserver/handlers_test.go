package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthz_OK(t *testing.T) {
	mux := NewMux(func() string { return "connected" })
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rw := httptest.NewRecorder()

	mux.ServeHTTP(rw, req)
	if rw.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rw.Code, http.StatusOK)
	}

	var got HealthzResponse
	if err := json.Unmarshal(rw.Body.Bytes(), &got); err != nil {
		t.Fatalf("json decode error: %v; body=%q", err, rw.Body.String())
	}
	if got.Status != "ok" {
		t.Fatalf("status field = %q, want %q", got.Status, "ok")
	}
	if got.DB != "connected" {
		t.Fatalf("db field = %q, want %q", got.DB, "connected")
	}
}

func TestHealthz_DBDisabled(t *testing.T) {
	mux := NewMux(nil)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rw := httptest.NewRecorder()

	mux.ServeHTTP(rw, req)
	if rw.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rw.Code, http.StatusOK)
	}

	var got HealthzResponse
	if err := json.Unmarshal(rw.Body.Bytes(), &got); err != nil {
		t.Fatalf("json decode error: %v", err)
	}
	if got.DB != "disabled" {
		t.Fatalf("db field = %q, want %q", got.DB, "disabled")
	}
}
