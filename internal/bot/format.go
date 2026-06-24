package bot

import (
	"log"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// tgMaxLen is Telegram's hard message length limit.
const tgMaxLen = 4096

// htmlEscape escapes the three characters that are special in Telegram HTML
// parse mode. Valid Kubernetes resource names never contain these, but log
// output and error strings can, so all dynamic content must pass through here.
func htmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// bold wraps escaped text in a bold tag.
func bold(s string) string { return "<b>" + htmlEscape(s) + "</b>" }

// code wraps escaped text in an inline code tag.
func code(s string) string { return "<code>" + htmlEscape(s) + "</code>" }

// pre wraps escaped text in a preformatted block (used for logs / dumps).
func pre(s string) string { return "<pre>" + htmlEscape(s) + "</pre>" }

// truncateRunes cuts s to at most max runes on a rune boundary, appending an
// ellipsis marker when truncation happened. Used for log output so we never
// split a multi-byte UTF-8 sequence.
func truncateRunes(s string, max int) (string, bool) {
	r := []rune(s)
	if len(r) <= max {
		return s, false
	}
	return string(r[:max]), true
}

// stripTags removes the small set of HTML tags we emit, for the plain-text
// retry fallback when a formatted send is rejected by Telegram.
var tagReplacer = strings.NewReplacer(
	"<b>", "", "</b>", "",
	"<i>", "", "</i>", "",
	"<code>", "", "</code>", "",
	"<pre>", "", "</pre>", "",
	"&amp;", "&", "&lt;", "<", "&gt;", ">",
)

func stripTags(s string) string { return tagReplacer.Replace(s) }

// chunkByLines splits text into pieces no longer than tgMaxLen, breaking only
// on newline boundaries so each chunk keeps its HTML tags balanced. A single
// line longer than the limit is hard-split on a rune boundary as a last resort.
func chunkByLines(text string) []string {
	if len(text) <= tgMaxLen {
		return []string{text}
	}

	var chunks []string
	var b strings.Builder
	for _, line := range strings.SplitAfter(text, "\n") {
		if b.Len()+len(line) > tgMaxLen {
			if b.Len() > 0 {
				chunks = append(chunks, b.String())
				b.Reset()
			}
			// Line itself too long: hard-split on rune boundaries.
			for len(line) > tgMaxLen {
				head, _ := truncateRunes(line, tgMaxLen)
				chunks = append(chunks, head)
				line = line[len(head):]
			}
		}
		b.WriteString(line)
	}
	if b.Len() > 0 {
		chunks = append(chunks, b.String())
	}
	return chunks
}

// sendDocument uploads content as a file attachment with an HTML caption.
// Used for payloads (e.g. full logs) too large to render inline.
func (b *Bot) sendDocument(chatID int64, filename, caption string, content []byte) {
	doc := tgbotapi.NewDocument(chatID, tgbotapi.FileBytes{Name: filename, Bytes: content})
	doc.Caption = caption
	doc.ParseMode = tgbotapi.ModeHTML
	if _, err := b.api.Send(doc); err != nil {
		log.Printf("Failed to send document %q: %v", filename, err)
		b.send(chatID, caption+"\n(failed to attach file; see bot logs)")
	}
}

// send delivers an HTML-formatted message, chunking past the length limit and
// falling back to plain text if Telegram rejects the formatted payload.
func (b *Bot) send(chatID int64, html string) {
	for _, chunk := range chunkByLines(html) {
		msg := tgbotapi.NewMessage(chatID, chunk)
		msg.ParseMode = tgbotapi.ModeHTML
		msg.DisableWebPagePreview = true
		if _, err := b.api.Send(msg); err != nil {
			log.Printf("Failed to send HTML message (%v); retrying as plain text", err)
			plain := tgbotapi.NewMessage(chatID, stripTags(chunk))
			if _, err2 := b.api.Send(plain); err2 != nil {
				log.Printf("Failed to send plain message: %v", err2)
			}
		}
	}
}
