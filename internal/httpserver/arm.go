package httpserver

import (
	"encoding/json"
	"io"
	"net/http"
	"path"
	"strings"
)

type cloudError struct {
	Error cloudErrorBody `json:"error"`
}

type cloudErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeCloudError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(cloudError{Error: cloudErrorBody{Code: code, Message: message}})
}

func storeKeyForRequest(r *http.Request) string {
	// Azure endpoints are effectively case-insensitive for path segments.
	return strings.ToLower(r.URL.Path)
}

func handleARM(store *memStore, w http.ResponseWriter, r *http.Request) {
	// Minimal subscription endpoint used by some tooling.
	if r.Method == http.MethodGet && strings.HasPrefix(strings.ToLower(r.URL.Path), "/subscriptions/") {
		// fall through to route-specific logic
	}

	// Provider registration endpoints (make everything look registered).
	// IMPORTANT: don't match real resource IDs that include /providers/{namespace}/...
	p := strings.ToLower(r.URL.Path)
	if isProviderEndpoint(p) {
		handleARMProviders(w, r)
		return
	}

	// Storage listKeys endpoint.
	if r.Method == http.MethodPost && strings.HasSuffix(p, "/listkeys") {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"keys":[{"keyName":"key1","permissions":"FULL","value":"ZHVtbXkta2V5"}]}`)
		return
	}

	switch r.Method {
	case http.MethodPut:
		handleARMPut(store, w, r)
		return
	case http.MethodGet:
		handleARMGet(store, w, r)
		return
	case http.MethodDelete:
		store.Delete(storeKeyForRequest(r))
		w.WriteHeader(http.StatusOK)
		return
	default:
		writeCloudError(w, http.StatusNotFound, "NotFound", "unsupported method")
		return
	}
}

func isProviderEndpoint(p string) bool {
	parts := strings.Split(strings.Trim(p, "/"), "/")
	// /subscriptions/{sub}/providers
	// /subscriptions/{sub}/providers/{namespace}
	// /subscriptions/{sub}/providers/{namespace}/register
	if len(parts) < 3 {
		return false
	}
	if parts[0] != "subscriptions" {
		return false
	}
	if parts[2] != "providers" {
		return false
	}
	if len(parts) == 3 {
		return true
	}
	if len(parts) == 4 {
		return true
	}
	if len(parts) == 5 && parts[4] == "register" {
		return true
	}
	return false
}

func handleARMProviders(w http.ResponseWriter, r *http.Request) {
	p := strings.ToLower(r.URL.Path)
	w.Header().Set("Content-Type", "application/json")

	// Register endpoints are typically POST.
	if strings.HasSuffix(p, "/register") {
		_, _ = io.WriteString(w, `{"registrationState":"Registered"}`)
		return
	}

	// List providers under subscription: /subscriptions/{sub}/providers
	if strings.HasSuffix(p, "/providers") {
		_, _ = io.WriteString(w, `{"value":[{"namespace":"Microsoft.Resources","registrationState":"Registered"},{"namespace":"Microsoft.Storage","registrationState":"Registered"}]}`)
		return
	}

	// Provider get: /subscriptions/{sub}/providers/{namespace}
	parts := strings.Split(strings.Trim(p, "/"), "/")
	ns := "Microsoft.Unknown"
	if len(parts) >= 4 && parts[len(parts)-2] == "providers" {
		ns = parts[len(parts)-1]
	}
	_, _ = io.WriteString(w, `{"namespace":"`+ns+`","registrationState":"Registered"}`)
}

func handleARMPut(store *memStore, w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var in map[string]any
	_ = json.Unmarshal(body, &in)

	location, _ := in["location"].(string)
	if location == "" {
		location = "eastus"
	}

	name := path.Base(r.URL.Path)
	resourceType := guessARMType(r.URL.Path)
	pl := strings.ToLower(r.URL.Path)
	resp := map[string]any{
		"id":       r.URL.Path,
		"name":     name,
		"type":     resourceType,
		"location": location,
		"tags":     in["tags"],
		"properties": map[string]any{
			"provisioningState": "Succeeded",
		},
	}

	// Carry through common storage-account fields if present.
	if v, ok := in["sku"]; ok {
		resp["sku"] = v
	}
	if v, ok := in["kind"]; ok {
		resp["kind"] = v
	}
	if v, ok := in["properties"]; ok {
		resp["properties"] = v
		if props, ok := v.(map[string]any); ok {
			props["provisioningState"] = "Succeeded"
		}
	}

	// Storage account: provide a few fields the provider commonly reads.
	if strings.Contains(pl, "/providers/microsoft.storage/storageaccounts/") {
		if _, ok := resp["kind"]; !ok {
			resp["kind"] = "StorageV2"
		}
		if _, ok := resp["sku"]; !ok {
			resp["sku"] = map[string]any{"name": "Standard_LRS", "tier": "Standard"}
		}
		props, _ := resp["properties"].(map[string]any)
		if props == nil {
			props = map[string]any{}
			resp["properties"] = props
		}
		props["primaryEndpoints"] = map[string]any{
			"blob":  "https://" + name + ".blob.core.windows.net/",
			"queue": "https://" + name + ".queue.core.windows.net/",
			"table": "https://" + name + ".table.core.windows.net/",
			"file":  "https://" + name + ".file.core.windows.net/",
			"web":   "https://" + name + ".web.core.windows.net/",
			"dfs":   "https://" + name + ".dfs.core.windows.net/",
		}
	}

	out, _ := json.Marshal(resp)
	store.Put(storeKeyForRequest(r), out)

	w.Header().Set("Content-Type", "application/json")
	// Azure ARM often uses PUT for create/update; returning 200 keeps clients permissive.
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(out)
}

func handleARMGet(store *memStore, w http.ResponseWriter, r *http.Request) {
	// Subscription GET: return a minimal shape.
	p := strings.ToLower(r.URL.Path)
	if strings.HasPrefix(p, "/subscriptions/") && !strings.Contains(p, "/resourcegroups/") && !strings.Contains(p, "/providers/") {
		// /subscriptions/{id}
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) == 2 {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"id":"/subscriptions/`+parts[1]+`","subscriptionId":"`+parts[1]+`","displayName":"groundstack","state":"Enabled"}`)
			return
		}
	}

	if v, ok := store.Get(storeKeyForRequest(r)); ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(v)
		return
	}

	writeCloudError(w, http.StatusNotFound, "ResourceNotFound", "resource not found")
}

func guessARMType(p string) string {
	pl := strings.ToLower(p)
	if strings.Contains(pl, "/providers/microsoft.storage/storageaccounts/") {
		return "Microsoft.Storage/storageAccounts"
	}
	if strings.Contains(pl, "/resourcegroups/") {
		return "Microsoft.Resources/resourceGroups"
	}
	return "Microsoft.Unknown/unknown"
}
