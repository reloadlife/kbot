package bot

import (
	"context"
	"fmt"
	"log"
)

// auditPermissionChange records a Kubernetes Event attributing a permission
// mutation to the acting admin. Failure to record never fails the operation.
func (b *Bot) auditPermissionChange(ctx context.Context, adminID, targetUserID int64, reason, detail string) {
	name := fmt.Sprintf("user-%d", targetUserID)
	msg := fmt.Sprintf("admin %d %s for user %d", adminID, detail, targetUserID)
	if err := b.k8sClient.RecordEvent(ctx, b.config.BotNamespace, "TelegramBotPermission", name, reason, msg); err != nil {
		log.Printf("Failed to record permission audit event: %v", err)
	}
}
