package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestARMCreateResourceGroup(t *testing.T) {
	mux := NewMux(nil)
	req := httptest.NewRequest(
		http.MethodPut,
		"/subscriptions/sub-123/resourceGroups/rg-test?api-version=2021-04-01",
		strings.NewReader(`{"location":"westus2","tags":{"env":"test","owner":"unit"}}`),
	)
	req.Host = "management.azure.com"
	req.Header.Set("Content-Type", "application/json")

	rw := httptest.NewRecorder()
	mux.ServeHTTP(rw, req)

	if rw.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%q", rw.Code, http.StatusOK, rw.Body.String())
	}
	if got := rw.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("content-type = %q, want %q", got, "application/json")
	}

	var got struct {
		ID         string            `json:"id"`
		Name       string            `json:"name"`
		Type       string            `json:"type"`
		Location   string            `json:"location"`
		Tags       map[string]string `json:"tags"`
		Properties struct {
			ProvisioningState string `json:"provisioningState"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(rw.Body.Bytes(), &got); err != nil {
		t.Fatalf("json decode error: %v; body=%q", err, rw.Body.String())
	}

	if got.ID != "/subscriptions/sub-123/resourceGroups/rg-test" {
		t.Fatalf("id = %q, want %q", got.ID, "/subscriptions/sub-123/resourceGroups/rg-test")
	}
	if got.Name != "rg-test" {
		t.Fatalf("name = %q, want %q", got.Name, "rg-test")
	}
	if got.Type != "Microsoft.Resources/resourceGroups" {
		t.Fatalf("type = %q, want %q", got.Type, "Microsoft.Resources/resourceGroups")
	}
	if got.Location != "westus2" {
		t.Fatalf("location = %q, want %q", got.Location, "westus2")
	}
	if got.Tags["env"] != "test" || got.Tags["owner"] != "unit" {
		t.Fatalf("tags = %#v, want env=test and owner=unit", got.Tags)
	}
	if got.Properties.ProvisioningState != "Succeeded" {
		t.Fatalf("properties.provisioningState = %q, want %q", got.Properties.ProvisioningState, "Succeeded")
	}
}
