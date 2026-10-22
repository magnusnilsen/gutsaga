// Package discord posts a completion to a Discord webhook.
package discord

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/magnusnilsen/gutsaga/internal/screenshot"
)

// ErrNotConfigured is returned when DISCORD_WEBHOOK_URL is empty.
var ErrNotConfigured = errors.New("discord webhook is not configured")

// CompletionText is the Norwegian message friends see in Discord.
func CompletionText(character, questTitle string) string {
	return "**" + escapeMD(character) + "** fullførte *" + escapeMD(questTitle) + "*!"
}

func escapeMD(s string) string {
	return strings.NewReplacer(
		"\\", "\\\\",
		"*", "\\*",
		"_", "\\_",
		"`", "\\`",
		"|", "\\|",
	).Replace(s)
}

type Message struct {
	Content       string
	AttachmentURL string
}

// Client posts multipart webhook messages. Username is always GutSaga.
type Client struct {
	WebhookURL string
	HTTP       *http.Client
}

func (c *Client) http() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 20 * time.Second}
}

// PostCompletion attaches png-or-other image bytes when non-nil.
// contentType is used only to pick a filename. An empty webhook URL returns an error.
func (c *Client) PostCompletion(ctx context.Context, character, questTitle, contentType string, image []byte) (Message, error) {
	if strings.TrimSpace(c.WebhookURL) == "" {
		return Message{}, ErrNotConfigured
	}
	content := CompletionText(character, questTitle)
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	payload, err := json.Marshal(map[string]string{
		"username": "GutSaga",
		"content":  content,
	})
	if err != nil {
		return Message{}, err
	}
	field, err := w.CreateFormField("payload_json")
	if err != nil {
		return Message{}, err
	}
	if _, err := field.Write(payload); err != nil {
		return Message{}, err
	}
	if len(image) > 0 {
		name := "screenshot" + screenshot.Extension(contentType)
		file, err := w.CreateFormFile("files[0]", name)
		if err != nil {
			return Message{}, err
		}
		if _, err := file.Write(image); err != nil {
			return Message{}, err
		}
	}
	if err := w.Close(); err != nil {
		return Message{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.WebhookURL+"?wait=true", &buf)
	if err != nil {
		return Message{}, err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	resp, err := c.http().Do(req)
	if err != nil {
		return Message{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Message{}, fmt.Errorf("discord status %d", resp.StatusCode)
	}
	msg := Message{Content: content}
	var parsed struct {
		Attachments []struct {
			URL string `json:"url"`
		} `json:"attachments"`
	}
	if json.Unmarshal(body, &parsed) == nil && len(parsed.Attachments) > 0 {
		msg.AttachmentURL = parsed.Attachments[0].URL
	}
	return msg, nil
}
