// Package discord posts OurSagas notices to an incoming Discord webhook.
// The username is always OurSagas. Text follows the group's locale (default English).
//
// Messages are embeds with a shared palette:
//
//	ColorSaga   gold    a new saga
//	ColorDone   green   a completion
//	ColorUpdate blue    a new saga version
//	ColorRetire gray    a saga retired
//	ColorLevel  amber   a milestone level
//
// Images are attached as webhook files and referenced as attachment://
// so Discord renders them without a public OurSagas URL.
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
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/magnusnilsen/gutsaga/internal/i18n"
	"github.com/magnusnilsen/gutsaga/internal/screenshot"
)

const username = "OurSagas"

// Palette shared by every OurSagas embed.
const (
	ColorSaga   = 0xC6A15B
	ColorDone   = 0x3BA55D
	ColorUpdate = 0x3E6F9A
	ColorRetire = 0x6B6258
	ColorLevel  = 0xD0892A
)

// Notice kinds recorded on a delivery result.
const (
	KindAnnounced  = "announced"
	KindUpdated    = "updated"
	KindRetired    = "retired"
	KindCompletion = "completion"
	KindLevel      = "level"
)

// ErrNotConfigured is returned when DISCORD_WEBHOOK_URL is empty.
var ErrNotConfigured = errors.New("discord webhook is not configured")

// CompletionText is the line friends see above a completion embed.
// The saga title lives on the embed; this line stays short. Locale defaults to English.
func CompletionText(character, questTitle string) string {
	return CompletionTextLang(character, questTitle, "")
}

// CompletionTextLang is CompletionText in the group's locale.
func CompletionTextLang(character, questTitle, lang string) string {
	_ = questTitle
	name := strings.TrimSpace(character)
	if name == "" {
		name = i18n.T(lang, "discord.someone")
	}
	return bold(name) + " " + i18n.T(lang, "discord.completed")
}

// Milestone reports whether a level is worth a Discord note.
// Every 10th level from 10 upward (10, 20, …) counts.
func Milestone(level int) bool {
	return level >= 10 && level%10 == 0
}

// ObjectiveLine is one Norwegian goal line, already written for players.
type ObjectiveLine struct {
	Text string
}

// Completion is a character finishing a saga.
// Race, Class, and Level are only shown when all three are set (fresh from the game).
// Objectives are listed with checkmarks when present (the version that was completed).
type Completion struct {
	ProgressID  string
	Character   string
	Race        string
	Class       string
	Level       int
	Title       string
	Objectives  []ObjectiveLine
	When        time.Time
	Image       []byte
	ContentType string
	// WebhookURL overrides the client URL. Empty uses the client default.
	WebhookURL string
	// Locale is the group's language (en or nb). Empty is English.
	Locale string
}

// SagaNotice is a new saga, a new version, or a retirement.
type SagaNotice struct {
	Kind        string
	Title       string
	Description string
	Objectives  []ObjectiveLine
	Ordered     bool
	// UpdatedBy is who changed the saga (display name). Used for update/retire lines.
	UpdatedBy   string
	CreatedBy   string
	Changes     []string
	Version     int
	When        time.Time
	Image       []byte
	ContentType string
	WebhookURL  string
	Locale      string
}

// LevelNotice is a milestone level-up. Class is only shown when set (fresh).
type LevelNotice struct {
	Character  string
	Class      string
	Level      int
	When       time.Time
	WebhookURL string
	Locale     string
}

// Message is what the webhook accepted, or (for Queue) what was queued.
type Message struct {
	Content       string
	AttachmentURL string
	// Accepted is true when a background worker will deliver the message.
	// Posted synchronously, it stays false and Content is the Discord content.
	Accepted bool
}

// Client posts webhook messages and retries rate limits and transient faults.
// Post methods block until Discord accepts the message or the attempts are spent.
// Use Queue when the API request must not wait.
type Client struct {
	WebhookURL  string
	HTTP        *http.Client
	MaxAttempts int
	// Sleep waits between attempts. Tests replace it. Nil uses a timer.
	Sleep func(ctx context.Context, d time.Duration) error
}

func (c *Client) http() *http.Client {
	if c != nil && c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 20 * time.Second}
}

func (c *Client) maxAttempts() int {
	if c != nil && c.MaxAttempts > 0 {
		return c.MaxAttempts
	}
	return 5
}

func (c *Client) sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	if c != nil && c.Sleep != nil {
		return c.Sleep(ctx, d)
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// PostCompletion sends a completion embed. Image bytes, when present, are the embed image.
func (c *Client) PostCompletion(ctx context.Context, n Completion) (Message, error) {
	payload, file, err := completionPayload(n)
	if err != nil {
		return Message{}, err
	}
	return c.send(ctx, payload, file)
}

// PostSaga sends an announcement, a version note, or a retirement.
// Cover bytes are attached only for KindAnnounced.
func (c *Client) PostSaga(ctx context.Context, n SagaNotice) (Message, error) {
	payload, file, err := sagaPayload(n)
	if err != nil {
		return Message{}, err
	}
	return c.send(ctx, payload, file)
}

// PostLevel sends a milestone level-up. Other levels should not be passed in.
func (c *Client) PostLevel(ctx context.Context, n LevelNotice) (Message, error) {
	if !Milestone(n.Level) {
		return Message{}, fmt.Errorf("level %d is not a milestone", n.Level)
	}
	payload, err := levelPayload(n)
	if err != nil {
		return Message{}, err
	}
	return c.send(ctx, payload, nil)
}

func (c *Client) send(ctx context.Context, payload []byte, file *attach) (Message, error) {
	if c == nil || strings.TrimSpace(c.WebhookURL) == "" {
		return Message{}, ErrNotConfigured
	}
	endpoint, err := withWait(c.WebhookURL)
	if err != nil {
		return Message{}, err
	}
	body, contentType, err := multipartBody(payload, file)
	if err != nil {
		return Message{}, err
	}
	var last error
	attempts := c.maxAttempts()
	for attempt := 1; attempt <= attempts; attempt++ {
		status, respBody, hdr, err := c.do(ctx, endpoint, contentType, body)
		if err != nil {
			last = err
			if attempt == attempts {
				break
			}
			if err := c.sleep(ctx, backoff(attempt)); err != nil {
				return Message{}, err
			}
			continue
		}
		if status == http.StatusTooManyRequests {
			last = fmt.Errorf("discord status 429: %s", snippet(respBody))
			if attempt == attempts {
				break
			}
			if err := c.sleep(ctx, retryAfter(hdr, respBody)); err != nil {
				return Message{}, err
			}
			continue
		}
		if status >= 500 {
			last = fmt.Errorf("discord status %d: %s", status, snippet(respBody))
			if attempt == attempts {
				break
			}
			if err := c.sleep(ctx, backoff(attempt)); err != nil {
				return Message{}, err
			}
			continue
		}
		if status < 200 || status >= 300 {
			return Message{}, fmt.Errorf("discord status %d: %s", status, snippet(respBody))
		}
		msg := Message{Content: payloadContent(payload)}
		var parsed struct {
			Attachments []struct {
				URL string `json:"url"`
			} `json:"attachments"`
		}
		if json.Unmarshal(respBody, &parsed) == nil && len(parsed.Attachments) > 0 {
			msg.AttachmentURL = parsed.Attachments[0].URL
		}
		return msg, nil
	}
	if last == nil {
		last = errors.New("discord post failed")
	}
	return Message{}, last
}

func (c *Client) do(ctx context.Context, endpoint, contentType string, body []byte) (int, []byte, http.Header, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, nil, nil, err
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http().Do(req)
	if err != nil {
		return 0, nil, nil, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode, respBody, resp.Header.Clone(), nil
}

func withWait(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("wait", "true")
	u.RawQuery = q.Encode()
	return u.String(), nil
}

type attach struct {
	Name string
	Body []byte
}

func multipartBody(payload []byte, file *attach) ([]byte, string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	field, err := w.CreateFormField("payload_json")
	if err != nil {
		return nil, "", err
	}
	if _, err := field.Write(payload); err != nil {
		return nil, "", err
	}
	if file != nil && len(file.Body) > 0 {
		part, err := w.CreateFormFile("files[0]", file.Name)
		if err != nil {
			return nil, "", err
		}
		if _, err := part.Write(file.Body); err != nil {
			return nil, "", err
		}
	}
	if err := w.Close(); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), w.FormDataContentType(), nil
}

func backoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	d := 300 * time.Millisecond * time.Duration(int(1)<<(attempt-1))
	if d > 5*time.Second || d <= 0 {
		return 5 * time.Second
	}
	return d
}

func retryAfter(hdr http.Header, body []byte) time.Duration {
	var parsed struct {
		RetryAfter float64 `json:"retry_after"`
	}
	if json.Unmarshal(body, &parsed) == nil && parsed.RetryAfter > 0 {
		return capWait(time.Duration(parsed.RetryAfter * float64(time.Second)))
	}
	if hdr != nil {
		if v := strings.TrimSpace(hdr.Get("Retry-After")); v != "" {
			if secs, err := strconv.ParseFloat(v, 64); err == nil && secs > 0 {
				return capWait(time.Duration(secs * float64(time.Second)))
			}
			if when, err := http.ParseTime(v); err == nil {
				return capWait(time.Until(when))
			}
		}
		if v := strings.TrimSpace(hdr.Get("X-RateLimit-Reset-After")); v != "" {
			if secs, err := strconv.ParseFloat(v, 64); err == nil && secs > 0 {
				return capWait(time.Duration(secs * float64(time.Second)))
			}
		}
	}
	return 500 * time.Millisecond
}

func capWait(d time.Duration) time.Duration {
	if d < 0 {
		return 0
	}
	if d > 20*time.Second {
		return 20 * time.Second
	}
	return d
}

func snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	s = strings.Join(strings.Fields(s), " ")
	return clipRunes(s, 160)
}

func payloadContent(payload []byte) string {
	var parsed struct {
		Content string `json:"content"`
	}
	_ = json.Unmarshal(payload, &parsed)
	return parsed.Content
}

type dcPayload struct {
	Username        string    `json:"username"`
	Content         string    `json:"content,omitempty"`
	Embeds          []dcEmbed `json:"embeds"`
	AllowedMentions dcAllowed `json:"allowed_mentions"`
}

type dcAllowed struct {
	Parse []string `json:"parse"`
}

type dcEmbed struct {
	Title       string    `json:"title,omitempty"`
	Description string    `json:"description,omitempty"`
	Color       int       `json:"color"`
	Fields      []dcField `json:"fields,omitempty"`
	Timestamp   string    `json:"timestamp,omitempty"`
	Image       *dcImage  `json:"image,omitempty"`
	Footer      *dcFooter `json:"footer,omitempty"`
}

type dcField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline,omitempty"`
}

type dcImage struct {
	URL string `json:"url"`
}

type dcFooter struct {
	Text string `json:"text"`
}

func completionPayload(n Completion) ([]byte, *attach, error) {
	embed := dcEmbed{
		Title:       clipRunes(displayTitle(n.Title), 256),
		Description: completionDetail(n),
		Color:       ColorDone,
	}
	var file *attach
	if len(n.Image) > 0 {
		file = &attach{Name: "screenshot" + screenshot.Extension(n.ContentType), Body: n.Image}
		embed.Image = &dcImage{URL: "attachment://" + file.Name}
	}
	raw, err := marshalPayload(CompletionTextLang(n.Character, n.Title, n.Locale), embed)
	return raw, file, err
}

// completionDetail is completed goals (checkmarks) plus race/class/level when fresh.
func completionDetail(n Completion) string {
	var b strings.Builder
	goals := CompletedObjectivesSummary(n.Objectives)
	if goals != "" {
		b.WriteString(goals)
	}
	race := strings.TrimSpace(n.Race)
	className := DisplayClass(n.Class)
	if race != "" && className != "" && n.Level >= 1 {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(escapeMD(race))
		b.WriteString(" · ")
		b.WriteString(escapeMD(className))
		b.WriteString(" · Level ")
		b.WriteString(strconv.Itoa(n.Level))
	}
	return clipRunes(b.String(), 4096)
}

// CompletedObjectivesSummary lists finished goals with checkmarks.
func CompletedObjectivesSummary(lines []ObjectiveLine) string {
	if len(lines) == 0 {
		return ""
	}
	const maxLines = 8
	show := len(lines)
	if show > maxLines {
		show = maxLines
	}
	var b strings.Builder
	for i := 0; i < show; i++ {
		if i > 0 {
			b.WriteByte('\n')
		}
		text := strings.TrimSpace(lines[i].Text)
		if text == "" {
			text = i18n.T("", "discord.goal")
		}
		b.WriteString("✓ ")
		b.WriteString(escapeMD(clipRunes(text, 200)))
	}
	if len(lines) > show {
		fmt.Fprintf(&b, "\n"+i18n.T("", "discord.more_checked"), len(lines)-show)
	}
	return clipRunes(b.String(), 3500)
}

func sagaPayload(n SagaNotice) ([]byte, *attach, error) {
	title := displayTitle(n.Title)
	var embed dcEmbed
	var content string
	lang := n.Locale
	switch n.Kind {
	case KindAnnounced:
		if actor := announceActor(n); actor != "" {
			content = fmt.Sprintf(i18n.T(lang, "discord.created"), escapeMD(actor))
		} else {
			content = i18n.T(lang, "discord.created_anon")
		}
		embed = dcEmbed{
			Title:       clipRunes(title, 256),
			Description: announcedDescription(n),
			Color:       ColorSaga,
		}
	case KindUpdated:
		version := n.Version
		if version < 1 {
			version = 1
		}
		if actor := strings.TrimSpace(n.UpdatedBy); actor != "" {
			content = fmt.Sprintf(i18n.T(lang, "discord.updated"), escapeMD(actor))
		} else {
			content = i18n.T(lang, "discord.updated_anon")
		}
		embed = dcEmbed{
			Title:       clipRunes(title, 256),
			Description: updateDescription(version, n.Changes, lang),
			Color:       ColorUpdate,
		}
	case KindRetired:
		if actor := strings.TrimSpace(n.UpdatedBy); actor != "" {
			content = fmt.Sprintf(i18n.T(lang, "discord.retired"), escapeMD(actor))
		} else {
			content = i18n.T(lang, "discord.retired_anon")
		}
		embed = dcEmbed{
			Title:       clipRunes(title, 256),
			Description: i18n.T(lang, "discord.retired_body"),
			Color:       ColorRetire,
		}
	default:
		return nil, nil, fmt.Errorf("unknown saga notice %q", n.Kind)
	}
	var file *attach
	if n.Kind == KindAnnounced && len(n.Image) > 0 {
		file = &attach{Name: "cover" + screenshot.Extension(n.ContentType), Body: n.Image}
		embed.Image = &dcImage{URL: "attachment://" + file.Name}
	}
	raw, err := marshalPayload(content, embed)
	return raw, file, err
}

func updateDescription(version int, changes []string, lang string) string {
	var b strings.Builder
	fmt.Fprintf(&b, i18n.T(lang, "discord.version"), version)
	for _, line := range changes {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		b.WriteString("\n• ")
		b.WriteString(escapeMD(clipRunes(line, 200)))
	}
	return clipRunes(b.String(), 4096)
}

// announcedDescription is the goal list, then the description heading, then the body.
func announcedDescription(n SagaNotice) string {
	goals := ObjectivesSummaryLang(n.Objectives, n.Ordered, n.Locale)
	body := strings.TrimSpace(n.Description)
	var b strings.Builder
	b.WriteString(goals)
	if body != "" {
		b.WriteString("\n\n**" + i18n.T(n.Locale, "discord.description") + "**\n")
		b.WriteString(escapeMD(clipRunes(body, 3500)))
	}
	out := strings.TrimSpace(b.String())
	if out == "" {
		return i18n.T(n.Locale, "discord.ready")
	}
	return clipRunes(out, 4096)
}

func levelPayload(n LevelNotice) ([]byte, error) {
	name := strings.TrimSpace(n.Character)
	if name == "" {
		name = i18n.T(n.Locale, "discord.someone")
	}
	desc := escapeMD(name)
	if className := DisplayClass(n.Class); className != "" {
		desc = escapeMD(name) + " · " + escapeMD(className)
	}
	embed := dcEmbed{
		Title:       "Level " + strconv.Itoa(n.Level),
		Description: desc,
		Color:       ColorLevel,
	}
	return marshalPayload(bold(name)+" "+i18n.T(n.Locale, "discord.level"), embed)
}

func marshalPayload(content string, embed dcEmbed) ([]byte, error) {
	return json.Marshal(dcPayload{
		Username:        username,
		Content:         content,
		Embeds:          []dcEmbed{embed},
		AllowedMentions: dcAllowed{Parse: []string{}},
	})
}

// ObjectivesSummary is the goal list for an announcement embed (English).
func ObjectivesSummary(lines []ObjectiveLine, ordered bool) string {
	return ObjectivesSummaryLang(lines, ordered, "")
}

// ObjectivesSummaryLang is ObjectivesSummary in the group's locale.
func ObjectivesSummaryLang(lines []ObjectiveLine, ordered bool, lang string) string {
	if len(lines) == 0 {
		return i18n.T(lang, "discord.no_goals")
	}
	const maxLines = 8
	show := len(lines)
	if show > maxLines {
		show = maxLines
	}
	var b strings.Builder
	for i := 0; i < show; i++ {
		if i > 0 {
			b.WriteByte('\n')
		}
		text := strings.TrimSpace(lines[i].Text)
		if text == "" {
			text = i18n.T(lang, "discord.goal")
		}
		text = escapeMD(clipRunes(text, 200))
		if ordered {
			fmt.Fprintf(&b, "%d. %s", i+1, text)
		} else {
			b.WriteString("• ")
			b.WriteString(text)
		}
	}
	if len(lines) > show {
		fmt.Fprintf(&b, "\n"+i18n.T(lang, "discord.more_goals"), len(lines)-show)
	}
	return clipRunes(b.String(), 1024)
}

// announceActor is the short name in "X opprettet/oppdaterte …".
func announceActor(n SagaNotice) string {
	if a := strings.TrimSpace(n.UpdatedBy); a != "" {
		return a
	}
	by := strings.TrimSpace(n.CreatedBy)
	if by == "" {
		return ""
	}
	if i := strings.IndexByte(by, '#'); i > 0 {
		return by[:i]
	}
	return by
}

func displayTitle(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "Saga"
	}
	return clipRunes(s, 180)
}

func bold(s string) string { return "**" + escapeMD(strings.TrimSpace(s)) + "**" }

func escapeMD(s string) string {
	return strings.NewReplacer(
		"\\", "\\\\",
		"*", "\\*",
		"_", "\\_",
		"`", "\\`",
		"|", "\\|",
	).Replace(s)
}

func clipRunes(s string, max int) string {
	r := []rune(s)
	if max <= 0 {
		return ""
	}
	if len(r) <= max {
		return s
	}
	if max < 2 {
		return string(r[:max])
	}
	return strings.TrimSpace(string(r[:max-1])) + "…"
}
