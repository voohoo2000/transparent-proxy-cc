package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/voohoo2000/transparent-proxy-cc/internal/config"
	"github.com/voohoo2000/transparent-proxy-cc/internal/origdst"
	"github.com/voohoo2000/transparent-proxy-cc/internal/proxy"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "serve" {
		runServe()
		return
	}
	log.Fatalf("usage: transparent-proxy serve")
}

func runServe() {
	cfg, err := config.FromEnv()
	if err != nil {
		log.Fatalf("configuration error: %v", err)
	}
	log.Printf("starting transparent proxy mode=%s upstream=%s:%d listen=%d", cfg.Mode, cfg.UpstreamProxyHost, cfg.UpstreamProxyPort, cfg.ListenPort)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	server := proxy.Server{Config: cfg, Resolver: origdst.LinuxResolver{}}
	if err := server.ListenAndServe(ctx); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
