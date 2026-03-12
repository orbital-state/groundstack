package httpserver

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

type tokenResponse struct {
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	ExtExpiresIn int    `json:"ext_expires_in"`
	AccessToken  string `json:"access_token"`
}

func handleAAD(w http.ResponseWriter, r *http.Request) {
	// Instance discovery is commonly called by Azure auth libraries.
	if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/common/discovery/instance") {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"tenant_discovery_endpoint":"https://login.microsoftonline.com/{tenantid}/v2.0/.well-known/openid-configuration"}`)
		return
	}

	// Token endpoints used by AzureRM provider:
	// - /{tenant}/oauth2/token
	// - /{tenant}/oauth2/v2.0/token
	if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/token") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"error":"not_found"}`)
		return
	}

	// Create a JWT-like token (alg=none) so any client code that expects
	// a 3-part token doesn't choke. Turquoise itself is permissive for now.
	now := time.Now().Unix()
	payload := map[string]any{
		"iss": "https://login.microsoftonline.com/groundstack/v2.0",
		"aud": "https://management.azure.com/",
		"iat": now,
		"nbf": now,
		"exp": now + 3600,
		"tid": "groundstack",
	}
	header := map[string]any{"alg": "none", "typ": "JWT"}

	encode := func(v any) string {
		b, _ := json.Marshal(v)
		return base64.RawURLEncoding.EncodeToString(b)
	}

	tok := encode(header) + "." + encode(payload) + "."

	resp := tokenResponse{
		TokenType:    "Bearer",
		ExpiresIn:    3600,
		ExtExpiresIn: 3600,
		AccessToken:  tok,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
