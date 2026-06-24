package bot

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"kubectl-bot/internal/rbac"
)

// confirmTTL is how long a pending destructive action stays valid.
const confirmTTL = 2 * time.Minute

// Destructive action kinds requiring confirmation.
const (
	actionRestart    = "restart"
	actionRollback   = "rollback"
	actionScale      = "scale"
	actionSelfUpdate = "selfupdate"
)

// pendingAction is a destructive operation awaiting button confirmation.
type pendingAction struct {
	kind      string
	namespace string
	name      string
	replicas  int32
	userID    int64
	chatID    int64
	created   time.Time
}

// requestConfirmation stores a pending action and prompts the user with
// inline Confirm / Cancel buttons.
func (b *Bot) requestConfirmation(chatID, userID int64, pa *pendingAction, summary string) {
	pa.userID = userID
	pa.chatID = chatID
	pa.created = time.Now()

	b.mu.Lock()
	b.tokenSeq++
	token := strconv.FormatUint(b.tokenSeq, 36)
	b.pending[token] = pa
	b.mu.Unlock()

	kb := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✅ Confirm", "c:"+token),
			tgbotapi.NewInlineKeyboardButtonData("✖️ Cancel", "x:"+token),
		),
	)

	msg := tgbotapi.NewMessage(chatID, "⚠️ <b>Confirm action</b>\n\n"+summary+"\n\nThis expires in 2 minutes.")
	msg.ParseMode = tgbotapi.ModeHTML
	msg.ReplyMarkup = kb
	if _, err := b.api.Send(msg); err != nil {
		log.Printf("Failed to send confirmation prompt: %v", err)
	}
}

// takePending atomically retrieves and removes a pending action by token,
// also pruning any that have expired.
func (b *Bot) takePending(token string) *pendingAction {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	for k, p := range b.pending {
		if now.Sub(p.created) > confirmTTL {
			delete(b.pending, k)
		}
	}

	pa, ok := b.pending[token]
	if !ok {
		return nil
	}
	delete(b.pending, token)
	if now.Sub(pa.created) > confirmTTL {
		return nil
	}
	return pa
}

// handleCallback processes inline-keyboard button presses.
func (b *Bot) handleCallback(ctx context.Context, cq *tgbotapi.CallbackQuery) {
	if cq.From == nil || len(cq.Data) < 3 {
		return
	}
	// Always acknowledge so the client stops showing a spinner.
	defer func() { _, _ = b.api.Request(tgbotapi.NewCallback(cq.ID, "")) }()

	prefix, token := cq.Data[:2], cq.Data[2:]
	pa := b.takePending(token)

	editTo := func(text string) {
		if cq.Message == nil {
			return
		}
		edit := tgbotapi.NewEditMessageText(cq.Message.Chat.ID, cq.Message.MessageID, text)
		edit.ParseMode = tgbotapi.ModeHTML
		if _, err := b.api.Send(edit); err != nil {
			log.Printf("Failed to edit confirmation message: %v", err)
		}
	}

	if pa == nil {
		editTo("⌛ This action has expired or was already handled.")
		return
	}
	// Only the original requester may confirm or cancel.
	if cq.From.ID != pa.userID {
		editTo("❌ Only the user who requested this action can respond to it.")
		return
	}

	if prefix == "x:" {
		editTo("✖️ Cancelled.")
		return
	}

	result, err := b.executeAction(ctx, pa)
	if err != nil {
		log.Printf("Action %q on %s/%s failed: %v", pa.kind, pa.namespace, pa.name, err)
		editTo("❌ " + htmlEscape(pa.kind) + " failed. Check the bot logs for details.")
		return
	}
	editTo(result)
}

// executeAction re-checks authorization, performs the operation, and writes an
// audit Event. Returns the success message to display.
func (b *Bot) executeAction(ctx context.Context, pa *pendingAction) (string, error) {
	// Re-verify permission at execution time (it may have changed since the
	// confirmation prompt was shown).
	if pa.kind != actionSelfUpdate {
		verb := pa.kind
		dec, err := b.validator.CheckPermission(ctx, rbac.PermissionCheck{
			TelegramUserID: pa.userID,
			Namespace:      pa.namespace,
			Resource:       "deployments",
			Verb:           verb,
			ResourceName:   pa.name,
		})
		if err != nil || !dec.Allowed {
			return "", fmt.Errorf("authorization revoked: %s", dec.Reason)
		}
	} else if !b.isAdmin(ctx, pa.userID) {
		return "", fmt.Errorf("admin access required")
	}

	switch pa.kind {
	case actionRestart:
		if err := b.k8sClient.RestartDeployment(ctx, pa.namespace, pa.name); err != nil {
			return "", err
		}
		b.audit(ctx, pa, "Restarted")
		return fmt.Sprintf("✅ Deployment %s restarted in %s", code(pa.name), code(pa.namespace)), nil

	case actionRollback:
		if err := b.k8sClient.RollbackDeployment(ctx, pa.namespace, pa.name); err != nil {
			return "", err
		}
		b.audit(ctx, pa, "RolledBack")
		return fmt.Sprintf("✅ Deployment %s rolled back in %s", code(pa.name), code(pa.namespace)), nil

	case actionScale:
		if err := b.k8sClient.ScaleDeployment(ctx, pa.namespace, pa.name, pa.replicas); err != nil {
			return "", err
		}
		b.audit(ctx, pa, fmt.Sprintf("ScaledTo%d", pa.replicas))
		return fmt.Sprintf("✅ Deployment %s scaled to %d replicas in %s", code(pa.name), pa.replicas, code(pa.namespace)), nil

	case actionSelfUpdate:
		if err := b.k8sClient.RestartDeployment(ctx, pa.namespace, pa.name); err != nil {
			return "", err
		}
		return "✅ Self-update triggered. The bot will restart and pull the latest image.", nil
	}

	return "", fmt.Errorf("unknown action %q", pa.kind)
}

// audit records a Kubernetes Event attributing the action to the Telegram user.
func (b *Bot) audit(ctx context.Context, pa *pendingAction, reason string) {
	msg := fmt.Sprintf("Telegram user %d performed %s on deployment %s", pa.userID, pa.kind, pa.name)
	if err := b.k8sClient.RecordEvent(ctx, pa.namespace, "Deployment", pa.name, reason, msg); err != nil {
		log.Printf("Failed to record audit event: %v", err)
	}
}
