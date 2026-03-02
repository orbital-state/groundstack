package httpserver

import (
	"encoding/json"
	"net/http"
)

type HealthzResponse struct {
	Status string `json:"status"`
	DB     string `json:"db"`
}

func NewMux(dbStatus func() string) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		resp := HealthzResponse{Status: "ok"}
		if dbStatus != nil {
			resp.DB = dbStatus()
		} else {
			resp.DB = "disabled"
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})
	return mux
}
