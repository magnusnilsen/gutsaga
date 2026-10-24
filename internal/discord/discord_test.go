package discord

import (
	"context"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPostCompletionEmbed(t *testing.T) {
	when := time.Date(2026, 10, 24, 18, 30, 0, 0, time.UTC)
	payload, file := post(t, func(c *Client) (Message, error) {
		return c.PostCompletion(context.Background(), Completion{
			Character:   "Magni",
			Class:       "Paladin",
			Level:       12,
			Title:       "Første vakt",
			Version:     3,
			Reward:      "Ære",
			When:        when,
			Image:       []byte("PNGDATA"),
			ContentType: "image/png",
		})
	})
	if payload.Username != "GutSaga" {
		t.Fatalf("username %s", payload.Username)
	}
	if payload.Content != "**Magni** fullførte *Første vakt*!" {
		t.Fatalf("content %s", payload.Content)
	}
	if payload.AllowedMentions.Parse == nil || len(payload.AllowedMentions.Parse) != 0 {
		t.Fatalf("mentions %+v", payload.AllowedMentions)
	}
	emb := payload.Embeds[0]
	if emb.Color != ColorDone || emb.Title != "Første vakt" {
		t.Fatalf("embed %+v", emb)
	}
	if emb.Timestamp != "2026-10-24T18:30:00Z" || emb.Footer.Text != "GutSaga" {
		t.Fatalf("stamp %+v footer %+v", emb.Timestamp, emb.Footer)
	}
	if !strings.Contains(emb.Description, "Magni") || !strings.Contains(emb.Description, "Paladin") || !strings.Contains(emb.Description, "12") {
		t.Fatalf("description %s", emb.Description)
	}
	got := fieldMap(emb.Fields)
	if got["Karakter"] != "Magni" || got["Klasse"] != "Paladin" || got["Nivå"] != "12" || got["Versjon"] != "3" || got["Belønning"] != "Ære" {
		t.Fatalf("fields %+v", got)
	}
	if emb.Image == nil || emb.Image.URL != "attachment://screenshot.png" {
		t.Fatalf("image %+v", emb.Image)
	}
	if file.name != "screenshot.png" || file.body != "PNGDATA" {
		t.Fatalf("file %s %q", file.name, file.body)
	}
}

func TestPostSagaAnnounced(t *testing.T) {
	when := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	min := 10
	payload, file := post(t, func(c *Client) (Message, error) {
		return c.PostSaga(context.Background(), SagaNotice{
			Kind:        KindAnnounced,
			Title:       "Første vakt",
			Description: "Stå vakt ved porten.\n\nDette avsnittet skal ikke med.",
			Objectives: []ObjectiveLine{
				{Text: "Møt opp"},
				{Text: "Hold ut"},
			},
			Ordered:     true,
			MinLevel:    &min,
			Reward:      "Ære",
			Party:       true,
			People:      []Person{{Name: "Magni", Class: "Paladin", Level: 12}},
			CreatedBy:   "Magnus#2231",
			Version:     1,
			When:        when,
			Image:       []byte("COVER"),
			ContentType: "image/png",
		})
	})
	if payload.Content != "Ny saga: *Første vakt*" {
		t.Fatalf("content %s", payload.Content)
	}
	emb := payload.Embeds[0]
	if emb.Color != ColorSaga || emb.Title != "Første vakt" {
		t.Fatalf("embed %+v", emb)
	}
	if strings.Contains(emb.Description, "avsnittet") || !strings.Contains(emb.Description, "Stå vakt ved porten.") {
		t.Fatalf("description %s", emb.Description)
	}
	got := fieldMap(emb.Fields)
	if got["Mål"] != "1. Møt opp\n2. Hold ut" {
		t.Fatalf("mål %q", got["Mål"])
	}
	if got["Minste nivå"] != "10" || got["Belønning"] != "Ære" || got["Laget av"] != "Magnus#2231" {
		t.Fatalf("fields %+v", got)
	}
	if !strings.Contains(got["Hvem"], "Parti") || !strings.Contains(got["Hvem"], "Magni (Paladin, nivå 12)") {
		t.Fatalf("hvem %q", got["Hvem"])
	}
	if emb.Image == nil || emb.Image.URL != "attachment://cover.png" || file.name != "cover.png" || file.body != "COVER" {
		t.Fatalf("image %+v file %s %q", emb.Image, file.name, file.body)
	}
	if emb.Timestamp != "2026-10-24T12:00:00Z" {
		t.Fatalf("stamp %s", emb.Timestamp)
	}
}

func TestPostSagaUpdateAndRetireAreMinimal(t *testing.T) {
	when := time.Date(2026, 10, 24, 12, 0, 0, 0, time.UTC)
	updated, file := post(t, func(c *Client) (Message, error) {
		return c.PostSaga(context.Background(), SagaNotice{
			Kind: KindUpdated, Title: "Første vakt", Version: 2, When: when,
			Image: []byte("SHOULD-NOT-ATTACH"), ContentType: "image/png",
		})
	})
	if updated.Content != "*Første vakt* er oppdatert (versjon 2)." {
		t.Fatalf("content %s", updated.Content)
	}
	if updated.Embeds[0].Color != ColorUpdate || updated.Embeds[0].Description != "Ny versjon 2." || len(updated.Embeds[0].Fields) != 0 {
		t.Fatalf("embed %+v", updated.Embeds[0])
	}
	if file.name != "" || updated.Embeds[0].Image != nil {
		t.Fatalf("update should not attach a cover, file %q image %+v", file.name, updated.Embeds[0].Image)
	}

	retired, _ := post(t, func(c *Client) (Message, error) {
		return c.PostSaga(context.Background(), SagaNotice{
			Kind: KindRetired, Title: "Første vakt", When: when, Image: []byte("NO"),
		})
	})
	if retired.Content != "*Første vakt* er lagt bort." {
		t.Fatalf("content %s", retired.Content)
	}
	emb := retired.Embeds[0]
	if emb.Color != ColorRetire || emb.Description != "Sagaen er ikke lenger aktiv." || emb.Image != nil || len(emb.Fields) != 0 {
		t.Fatalf("embed %+v", emb)
	}
}

func TestPostLevelMilestone(t *testing.T) {
	if Milestone(9) || Milestone(11) || Milestone(0) || !Milestone(10) || !Milestone(60) {
		t.Fatal("milestone bounds")
	}
	when := time.Date(2026, 10, 24, 8, 0, 0, 0, time.UTC)
	payload, _ := post(t, func(c *Client) (Message, error) {
		return c.PostLevel(context.Background(), LevelNotice{Character: "Magni", Class: "Paladin", Level: 20, When: when})
	})
	if payload.Content != "**Magni** nådde nivå 20." {
		t.Fatalf("content %s", payload.Content)
	}
	emb := payload.Embeds[0]
	if emb.Color != ColorLevel || emb.Title != "Nivå 20" || emb.Description != "Magni · Paladin" || emb.Timestamp != "2026-10-24T08:00:00Z" {
		t.Fatalf("embed %+v", emb)
	}
	_, err := (&Client{WebhookURL: "http://example.invalid"}).PostLevel(context.Background(), LevelNotice{Level: 13})
	if err == nil {
		t.Fatal("expected non-milestone to fail on the sync client")
	}
}

func TestCompletionTextEscapesMarkdown(t *testing.T) {
	got := CompletionText("A*b", "Stjerne *kveld*")
	if !strings.Contains(got, `A\*b`) || !strings.Contains(got, `Stjerne \*kveld\*`) {
		t.Fatal(got)
	}
}

func TestRetry429UsesRetryAfter(t *testing.T) {
	var (
		mu    sync.Mutex
		hits  int
		waits []time.Duration
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("wait") != "true" {
			t.Errorf("wait query %s", r.URL.RawQuery)
		}
		mu.Lock()
		hits++
		n := hits
		mu.Unlock()
		if n == 1 {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", "9")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, `{"message":"You are being rate limited.","retry_after":0.25,"global":false}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"attachments":[{"url":"https://cdn.example/shot.png"}]}`)
	}))
	defer srv.Close()
	client := &Client{
		WebhookURL: srv.URL,
		HTTP:       srv.Client(),
		Sleep: func(ctx context.Context, d time.Duration) error {
			mu.Lock()
			waits = append(waits, d)
			mu.Unlock()
			return nil
		},
	}
	msg, err := client.PostLevel(context.Background(), LevelNotice{Character: "Magni", Level: 10})
	if err != nil {
		t.Fatal(err)
	}
	if hits != 2 {
		t.Fatalf("hits %d", hits)
	}
	if len(waits) != 1 || waits[0] < 240*time.Millisecond || waits[0] > 260*time.Millisecond {
		t.Fatalf("waits %v", waits)
	}
	if msg.AttachmentURL != "https://cdn.example/shot.png" {
		t.Fatalf("url %q", msg.AttachmentURL)
	}
}

func TestRetryAfterHeaderWhenBodyHasNone(t *testing.T) {
	var waits []time.Duration
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits == 1 {
			w.Header().Set("Retry-After", "3")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, `{"message":"slow down"}`)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	_, err := (&Client{
		WebhookURL: srv.URL,
		HTTP:       srv.Client(),
		Sleep: func(ctx context.Context, d time.Duration) error {
			waits = append(waits, d)
			return nil
		},
	}).PostSaga(context.Background(), SagaNotice{Kind: KindRetired, Title: "Vakt"})
	if err != nil {
		t.Fatal(err)
	}
	if len(waits) != 1 || waits[0] != 3*time.Second {
		t.Fatalf("waits %v", waits)
	}
}

func TestNoRetryOnBadRequest(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		http.Error(w, "bad payload", http.StatusBadRequest)
	}))
	defer srv.Close()
	slept := false
	_, err := (&Client{
		WebhookURL: srv.URL,
		HTTP:       srv.Client(),
		Sleep: func(ctx context.Context, d time.Duration) error {
			slept = true
			return nil
		},
	}).PostSaga(context.Background(), SagaNotice{Kind: KindUpdated, Title: "Vakt", Version: 2})
	if err == nil || !strings.Contains(err.Error(), "400") {
		t.Fatal(err)
	}
	if hits != 1 || slept {
		t.Fatalf("hits %d slept %v", hits, slept)
	}
}

func TestRetryServerErrorThenSuccess(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits == 1 {
			http.Error(w, "boom", http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	_, err := (&Client{
		WebhookURL: srv.URL,
		HTTP:       srv.Client(),
		Sleep:      func(ctx context.Context, d time.Duration) error { return nil },
	}).PostSaga(context.Background(), SagaNotice{Kind: KindUpdated, Title: "Vakt", Version: 4})
	if err != nil || hits != 2 {
		t.Fatalf("err %v hits %d", err, hits)
	}
}

func TestQueueDoesNotBlockOrFailTheCaller(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		once.Do(func() { close(started) })
		<-release
		http.Error(w, "down", http.StatusInternalServerError)
	}))
	defer srv.Close()
	q := NewQueue(&Client{
		WebhookURL:  srv.URL,
		HTTP:        srv.Client(),
		MaxAttempts: 1,
		Sleep:       func(ctx context.Context, d time.Duration) error { return nil },
	})
	done := make(chan Result, 1)
	q.After = func(res Result) { done <- res }

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	msg, err := q.PostSaga(ctx, SagaNotice{Kind: KindAnnounced, Title: "Kø", Description: "Kort."})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if !msg.Accepted {
		t.Fatal("expected accepted")
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("post blocked for %s", elapsed)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("webhook was not called")
	}
	close(release)
	select {
	case res := <-done:
		if res.Err == nil || res.Kind != KindAnnounced {
			t.Fatalf("result %+v", res)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("delivery did not finish")
	}
}

func TestQueueSkipsEmptyWebhook(t *testing.T) {
	q := NewQueue(&Client{})
	_, err := q.PostCompletion(context.Background(), Completion{Character: "Magni", Title: "Saga"})
	if err != ErrNotConfigured {
		t.Fatal(err)
	}
}

func TestObjectivesSummaryCapsLines(t *testing.T) {
	lines := make([]ObjectiveLine, 10)
	for i := range lines {
		lines[i] = ObjectiveLine{Text: "Mål"}
	}
	got := ObjectivesSummary(lines, false)
	if strings.Count(got, "•") != 8 || !strings.Contains(got, "og 2 mål til") {
		t.Fatal(got)
	}
	if ObjectivesSummary(nil, false) != "Ingen mål" {
		t.Fatal("empty")
	}
}

type postedFile struct {
	name string
	body string
}

func post(t *testing.T, call func(*Client) (Message, error)) (dcPayload, postedFile) {
	t.Helper()
	var payload dcPayload
	var file postedFile
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil {
			t.Error(err)
			return
		}
		mr := multipart.NewReader(r.Body, params["boundary"])
		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Error(err)
				return
			}
			b, _ := io.ReadAll(part)
			switch part.FormName() {
			case "payload_json":
				if err := json.Unmarshal(b, &payload); err != nil {
					t.Error(err)
				}
			case "files[0]":
				file.name = part.FileName()
				file.body = string(b)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"1","attachments":[{"url":"https://cdn.example/file.png"}]}`)
	}))
	t.Cleanup(srv.Close)
	if _, err := call(&Client{WebhookURL: srv.URL, HTTP: srv.Client()}); err != nil {
		t.Fatal(err)
	}
	if len(payload.Embeds) != 1 {
		t.Fatalf("embeds %+v", payload.Embeds)
	}
	return payload, file
}

func fieldMap(fields []dcField) map[string]string {
	out := map[string]string{}
	for _, f := range fields {
		out[f.Name] = f.Value
	}
	return out
}
