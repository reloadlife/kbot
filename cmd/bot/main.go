package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"kubectl-bot/internal/bot"
	"kubectl-bot/internal/config"
	"kubectl-bot/internal/k8s"
)

func main() {
	log.Println("Starting Kubernetes Telegram Bot...")

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	log.Printf("Loaded config with %d bootstrap admin(s)", len(cfg.AdminTelegramIDs))

	// Create Kubernetes client
	k8sClient, err := k8s.NewClient()
	if err != nil {
		log.Fatalf("Failed to create Kubernetes client: %v", err)
	}

	log.Println("Connected to Kubernetes cluster")

	// Create bot
	telegramBot, err := bot.NewBot(cfg, k8sClient)
	if err != nil {
		log.Fatalf("Failed to create bot: %v", err)
	}

	// Setup context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Received shutdown signal")
		cancel()
	}()

	// Start bot
	log.Println("Bot started successfully")
	if err := telegramBot.Start(ctx); err != nil {
		log.Fatalf("Bot error: %v", err)
	}

	log.Println("Bot shutdown complete")
}
