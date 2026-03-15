package httpserver

import (
	"io"
	"net/http"
	"strings"
	"time"
)

func handleStorageDataPlane(w http.ResponseWriter, r *http.Request) {
	// We are intentionally permissive for MVP: accept any auth header and return 200.
	w.Header().Set("x-ms-version", "2020-10-02")
	w.Header().Set("x-ms-request-id", "groundstack")
	w.Header().Set("Date", time.Now().UTC().Format(http.TimeFormat))

	q := r.URL.Query()
	comp := strings.ToLower(q.Get("comp"))
	restype := strings.ToLower(q.Get("restype"))

	// AzureRM provider uses these to wait until services become available.
	// File service: /?restype=service&comp=properties
	if r.Method == http.MethodGet && restype == "service" && comp == "properties" {
		writeStorageServicePropertiesXML(w)
		return
	}

	// Blob service properties: /?comp=properties
	if r.Method == http.MethodGet && comp == "properties" {
		writeStorageServicePropertiesXML(w)
		return
	}

	// Generic liveness
	if r.Method == http.MethodHead || r.Method == http.MethodGet {
		w.WriteHeader(http.StatusOK)
		return
	}

	w.WriteHeader(http.StatusNotFound)
	_, _ = io.WriteString(w, "")
}

func writeStorageServicePropertiesXML(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	// Minimal XML structure that Azure storage SDKs can parse.
	_, _ = io.WriteString(w, "<?xml version=\"1.0\" encoding=\"utf-8\"?>\n<StorageServiceProperties>\n  <HourMetrics><Version>1.0</Version><Enabled>false</Enabled></HourMetrics>\n  <MinuteMetrics><Version>1.0</Version><Enabled>false</Enabled></MinuteMetrics>\n</StorageServiceProperties>")
}
