package bot

import (
	"context"
	"log"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"kubectl-bot/internal/config"
	"kubectl-bot/internal/k8s"
	"kubectl-bot/internal/rbac"
)

type Bot struct {
	api       *tgbotapi.BotAPI
	k8sClient *k8s.Client
	rbac      *rbac.Manager
	validator *rbac.Validator
	config    *config.Config
}

// NewBot creates a new Telegram bot
func NewBot(cfg *config.Config, k8sClient *k8s.Client) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(cfg.TelegramBotToken)
	if err != nil {
		return nil, err
	}

	log.Printf("Authorized on account %s", api.Self.UserName)

	rbacManager := rbac.NewManager(k8sClient, cfg)
	validator := rbac.NewValidator(rbacManager, k8sClient)

	return &Bot{
		api:       api,
		k8sClient: k8sClient,
		rbac:      rbacManager,
		validator: validator,
		config:    cfg,
	}, nil
}

// Start starts the bot and begins processing updates
func (b *Bot) Start(ctx context.Context) error {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := b.api.GetUpdatesChan(u)

	log.Println("Bot is running...")

	for {
		select {
		case <-ctx.Done():
			log.Println("Shutting down bot...")
			b.api.StopReceivingUpdates()
			return nil
		case update := <-updates:
			if update.Message == nil {
				continue
			}

			// Handle the message
			go b.handleMessage(ctx, update.Message)
		}
	}
}

// handleMessage processes incoming messages
func (b *Bot) handleMessage(ctx context.Context, message *tgbotapi.Message) {
	userID := message.From.ID

	log.Printf("Received message from user %d: %s", userID, message.Text)

	if !message.IsCommand() {
		b.sendMessage(message.Chat.ID, "Please use a command. Type /help for available commands.")
		return
	}

	// Route commands to handlers
	switch message.Command() {
	case "start":
		b.handleStart(ctx, message)
	case "help":
		b.handleHelp(ctx, message)
	case "namespaces":
		b.handleNamespaces(ctx, message)
	case "pods":
		b.handlePods(ctx, message)
	case "deployments":
		b.handleDeployments(ctx, message)
	case "services":
		b.handleServices(ctx, message)
	case "logs":
		b.handleLogs(ctx, message)
	case "restart":
		b.handleRestart(ctx, message)
	case "rollback":
		b.handleRollback(ctx, message)
	case "scale":
		b.handleScale(ctx, message)
	case "grant":
		b.handleGrant(ctx, message)
	case "revoke":
		b.handleRevoke(ctx, message)
	case "permissions":
		b.handlePermissions(ctx, message)
	default:
		b.sendMessage(message.Chat.ID, "Unknown command. Type /help for available commands.")
	}
}

// sendMessage sends a text message to a chat
func (b *Bot) sendMessage(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	_, err := b.api.Send(msg)
	if err != nil {
		log.Printf("Failed to send message: %v", err)
	}
}

// getUserRole returns the user's role for display
func (b *Bot) getUserRole(ctx context.Context, userID int64) string {
	if b.rbac.IsBootstrapAdmin(userID) {
		return "admin (bootstrap)"
	}

	permission, err := b.rbac.GetUserPermission(ctx, userID)
	if err != nil {
		return "none"
	}

	return permission.Spec.Role
}
