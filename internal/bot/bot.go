package bot

import (
	"context"
	"log"
	"sync"
	"time"

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

	mu       sync.Mutex
	pending  map[string]*pendingAction
	tokenSeq uint64
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
		pending:   make(map[string]*pendingAction),
	}, nil
}

// Start starts the bot and begins processing updates
func (b *Bot) Start(ctx context.Context) error {
	// Set bot commands for UI
	if err := b.setupCommands(); err != nil {
		log.Printf("Warning: Failed to set bot commands: %v", err)
	}

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
			switch {
			case update.CallbackQuery != nil:
				go b.safeGo("callback", func() { b.handleCallback(ctx, update.CallbackQuery) })
			case update.Message != nil:
				go b.safeGo("message", func() { b.handleMessage(ctx, update.Message) })
			}
		}
	}
}

// safeGo runs fn in the current goroutine, recovering from panics so one bad
// update can never crash the whole bot process.
func (b *Bot) safeGo(kind string, fn func()) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Recovered from panic while handling %s: %v", kind, r)
		}
	}()
	fn()
}

// handleMessage processes incoming messages
func (b *Bot) handleMessage(ctx context.Context, message *tgbotapi.Message) {
	// Anonymous group admins and channel posts carry no From; ignore them
	// rather than dereferencing a nil pointer (which would panic).
	if message.From == nil {
		return
	}

	userID := message.From.ID
	isGroup := message.Chat.IsGroup() || message.Chat.IsSuperGroup()

	log.Printf("Received message from user %d in chat %d (group=%v): %s",
		userID, message.Chat.ID, isGroup, message.Text)

	// Check if user has any permissions (for groups)
	if isGroup {
		hasPermission := b.hasAnyPermission(ctx, userID)
		if !hasPermission {
			// Silently ignore messages from unauthorized users in groups
			log.Printf("Ignoring message from unauthorized user %d in group %d", userID, message.Chat.ID)
			return
		}
	}

	if !message.IsCommand() {
		// Only respond to non-command messages in private chats
		if !isGroup {
			b.send(message.Chat.ID, "Please use a command. Type /help for available commands.")
		}
		return
	}

	// Each command runs with a bounded timeout so a hung API call can't leak a
	// goroutine forever.
	cmdCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Route commands to handlers
	switch message.Command() {
	case "start", "whoami":
		b.handleStart(cmdCtx, message)
	case "help":
		b.handleHelp(cmdCtx, message)
	case "namespaces":
		b.handleNamespaces(cmdCtx, message)
	case "pods":
		b.handlePods(cmdCtx, message)
	case "deployments":
		b.handleDeployments(cmdCtx, message)
	case "services":
		b.handleServices(cmdCtx, message)
	case "logs":
		b.handleLogs(cmdCtx, message)
	case "describe":
		b.handleDescribe(cmdCtx, message)
	case "events":
		b.handleEvents(cmdCtx, message)
	case "top":
		b.handleTop(cmdCtx, message)
	case "restart":
		b.handleRestart(cmdCtx, message)
	case "rollback":
		b.handleRollback(cmdCtx, message)
	case "scale":
		b.handleScale(cmdCtx, message)
	case "grant":
		b.handleGrant(cmdCtx, message)
	case "revoke":
		b.handleRevoke(cmdCtx, message)
	case "role":
		b.handleRole(cmdCtx, message)
	case "permissions":
		b.handlePermissions(cmdCtx, message)
	case "users":
		b.handleUsers(cmdCtx, message)
	case "selfupdate":
		b.handleSelfUpdate(cmdCtx, message)
	default:
		b.send(message.Chat.ID, "Unknown command. Type /help for available commands.")
	}
}

// isAdmin reports whether the user is a bootstrap admin or has the admin role.
func (b *Bot) isAdmin(ctx context.Context, userID int64) bool {
	if b.rbac.IsBootstrapAdmin(userID) {
		return true
	}
	permission, err := b.rbac.GetUserPermission(ctx, userID)
	return err == nil && permission.Spec.Role == "admin"
}

// fail logs the full error server-side and sends the user a short, generic
// message. Raw Kubernetes/API errors can leak cluster internals, so they are
// never forwarded verbatim to Telegram.
func (b *Bot) fail(chatID int64, action string, err error) {
	log.Printf("Operation %q failed: %v", action, err)
	b.send(chatID, "❌ "+htmlEscape(action)+" failed. Check the bot logs for details.")
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

// hasAnyPermission checks if user has any permissions (bootstrap admin or CRD permissions)
func (b *Bot) hasAnyPermission(ctx context.Context, userID int64) bool {
	// Check bootstrap admin
	if b.rbac.IsBootstrapAdmin(userID) {
		return true
	}

	// Check CRD permissions
	permission, err := b.rbac.GetUserPermission(ctx, userID)
	if err != nil {
		return false
	}

	return permission.Spec.Role != "" ||
		len(permission.Spec.RoleBindings) > 0 ||
		len(permission.Spec.Permissions) > 0
}

// setupCommands sets up bot commands for Telegram UI
func (b *Bot) setupCommands() error {
	commands := []tgbotapi.BotCommand{
		{Command: "start", Description: "Start the bot and check your permissions"},
		{Command: "whoami", Description: "Show your identity, roles, and what you can do"},
		{Command: "help", Description: "Show help and available commands"},
		{Command: "namespaces", Description: "List accessible namespaces"},
		{Command: "pods", Description: "List pods in a namespace"},
		{Command: "deployments", Description: "List deployments in a namespace"},
		{Command: "services", Description: "List services in a namespace"},
		{Command: "logs", Description: "Get pod logs (-c container, --tail N, --previous, --since N)"},
		{Command: "describe", Description: "Describe a pod or deployment"},
		{Command: "events", Description: "Show recent events in a namespace"},
		{Command: "top", Description: "Show pod CPU/memory usage"},
		{Command: "restart", Description: "Restart a deployment"},
		{Command: "rollback", Description: "Rollback a deployment"},
		{Command: "scale", Description: "Scale a deployment"},
		{Command: "grant", Description: "Grant permissions to a user (admin only)"},
		{Command: "revoke", Description: "Revoke permissions from a user (admin only)"},
		{Command: "role", Description: "Set a user's role: /role <id> <viewer|operator|admin|none> [-n ns] (admin only)"},
		{Command: "permissions", Description: "View user permissions"},
		{Command: "users", Description: "List all users with permissions (admin only)"},
		{Command: "selfupdate", Description: "Update bot to latest image (admin only)"},
	}

	cfg := tgbotapi.NewSetMyCommands(commands...)
	_, err := b.api.Request(cfg)
	if err != nil {
		return err
	}

	log.Println("Bot commands registered successfully")
	return nil
}
