package httpserver

import (
	"io"
	"log"
	"net/http"
	"strings"
)

func newAzureRouter(store *memStore) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if i := strings.IndexByte(host, ':'); i >= 0 {
			host = host[:i]
		}

		// Lightweight request log so Terraform debugging is easy.
		log.Printf("req host=%s method=%s path=%s", host, r.Method, r.URL.RequestURI())

		switch host {
		case "login.microsoftonline.com":
			handleAAD(w, r)
			return
		case "management.azure.com":
			handleARM(store, w, r)
			return
		case "graph.microsoft.com":
			handleGraph(w, r)
			return
		default:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"error":{"code":"HostNotFound","message":"unknown host"}}`)
			return
		}
	})
}
