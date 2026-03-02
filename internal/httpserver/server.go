package httpserver

import (
	"context"
	"net"
	"net/http"
	"time"
)

type Server struct {
	httpServer *http.Server
	listener   net.Listener
}

type Options struct {
	ListenAddr string
	Handler    http.Handler
}

func New(opts Options) (*Server, error) {
	ln, err := net.Listen("tcp", opts.ListenAddr)
	if err != nil {
		return nil, err
	}

	s := &http.Server{
		Handler:           opts.Handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}

	return &Server{httpServer: s, listener: ln}, nil
}

func (s *Server) Addr() string {
	return s.listener.Addr().String()
}

func (s *Server) Serve() error {
	return s.httpServer.Serve(s.listener)
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}
