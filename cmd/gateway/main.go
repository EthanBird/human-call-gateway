package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/EthanBird/human-call-gateway/internal/adapter"
	"github.com/EthanBird/human-call-gateway/internal/config"
	httpHandler "github.com/EthanBird/human-call-gateway/internal/http"
)

func main() {
	configPath := flag.String("config", "", "Path to configuration file")
	flag.Parse()

	if *configPath == "" {
		*configPath = os.Getenv("CONFIG_PATH")
		if *configPath == "" {
			*configPath = "./config.yaml"
		}
	}

	log.Printf("Loading configuration from: %s", *configPath)
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	registry := adapter.NewRegistry()
	httpHelper := adapter.NewHTTPHelper(registry.GetHTTPClient())

	registry.Register("slack", adapter.NewSlackAdapter(httpHelper))
	registry.Register("telegram", adapter.NewTelegramAdapter(httpHelper))
	registry.Register("feishu", adapter.NewFeishuAdapter(httpHelper))
	registry.Register("qq", adapter.NewQQAdapter(httpHelper))
	registry.Register("webhook", adapter.NewWebhookAdapter(httpHelper))

	handler := httpHandler.NewHandler(cfg, registry)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	addr := fmt.Sprintf(":%s", port)
	log.Printf("Starting Human Call Gateway on %s", addr)
	log.Printf("Configured channels: %d", len(cfg.Channels))
	if cfg.DefaultChannelID != "" {
		log.Printf("Default channel: %s", cfg.DefaultChannelID)
	}

	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
