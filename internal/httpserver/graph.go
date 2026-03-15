package httpserver

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

var graphAppIDFilterRe = regexp.MustCompile(`appId\s+eq\s+['\"]([0-9a-fA-F-]{36})['\"]`)

func handleGraph(w http.ResponseWriter, r *http.Request) {
	// Minimal subset for azurerm provider: discover service principal object ID.
	// Common call:
	//   GET /v1.0/servicePrincipals?$filter=appId%20eq%20'{clientId}'
	p := strings.Trim(r.URL.Path, "/")
	if r.Method != http.MethodGet {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"error":{"code":"NotFound","message":"unsupported method"}}`)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	// /v1.0/servicePrincipals/{id}
	if strings.HasPrefix(p, "v1.0/serviceprincipals/") {
		id := pathBasePreserveCase(r.URL.Path)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":    id,
			"appId": id,
		})
		return
	}

	if strings.HasPrefix(strings.ToLower(p), "v1.0/serviceprincipals") {
		clientID := extractAppIDFilter(r.URL)
		if clientID == "" {
			_, _ = io.WriteString(w, `{"value":[]}`)
			return
		}
		// Return exactly one result.
		_ = json.NewEncoder(w).Encode(map[string]any{
			"value": []map[string]any{{
				"id":    clientID,
				"appId": clientID,
			}},
		})
		return
	}

	_, _ = io.WriteString(w, `{"value":[]}`)
}

func extractAppIDFilter(u *url.URL) string {
	// Try $filter.
	filter := u.Query().Get("$filter")
	if filter == "" {
		filter = u.Query().Get("filter")
	}
	m := graphAppIDFilterRe.FindStringSubmatch(filter)
	if len(m) != 2 {
		return ""
	}
	return m[1]
}

func pathBasePreserveCase(p string) string {
	p = strings.TrimRight(p, "/")
	if p == "" {
		return ""
	}
	idx := strings.LastIndexByte(p, '/')
	if idx < 0 {
		return p
	}
	return p[idx+1:]
}
