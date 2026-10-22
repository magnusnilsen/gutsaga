package discord

import (
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPostCompletionNorwegianFile(t *testing.T) {
	var gotContent, gotUser, gotFile string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil {
			t.Fatal(err)
		}
		mr := multipart.NewReader(r.Body, params["boundary"])
		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			b, _ := io.ReadAll(part)
			switch part.FormName() {
			case "payload_json":
				gotContent = string(b)
			case "files[0]":
				gotFile = string(b)
				gotUser = part.FileName()
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"attachments":[{"url":"https://cdn.example/shot.png"}]}`)
	}))
	defer srv.Close()

	msg, err := (&Client{WebhookURL: srv.URL, HTTP: srv.Client()}).PostCompletion(
		context.Background(), "Magni", "En prøve", "image/png", []byte("PNGDATA"),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotContent, `"username":"GutSaga"`) {
		t.Fatalf("payload %s", gotContent)
	}
	if !strings.Contains(gotContent, `**Magni** fullførte *En prøve*!`) {
		t.Fatalf("text %s", gotContent)
	}
	if gotFile != "PNGDATA" || gotUser != "screenshot.png" {
		t.Fatalf("file %q name %q", gotFile, gotUser)
	}
	if msg.AttachmentURL != "https://cdn.example/shot.png" {
		t.Fatalf("url %q", msg.AttachmentURL)
	}
}

func TestCompletionTextEscapesMarkdown(t *testing.T) {
	got := CompletionText("Magni", "Stjerne *kveld*")
	if !strings.Contains(got, `Stjerne \*kveld\*`) {
		t.Fatal(got)
	}
}
