// Package backend is the companion's client for the GutSaga API.
package backend

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/magnusnilsen/gutsaga/internal/contract"
	"github.com/magnusnilsen/gutsaga/internal/screenshot"
)

type Client struct {
	BaseURL string
	APIKey  string
	HTTP    *http.Client
}

func (c *Client) http() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 60 * time.Second}
}

type HTTPError struct {
	Status  int
	Code    string
	Message string
}

func (e *HTTPError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("API-svar %d", e.Status)
}

func (e *HTTPError) Permanent() bool {
	return e.Status >= 400 && e.Status < 500 && e.Status != http.StatusTooManyRequests && e.Status != http.StatusRequestTimeout
}

func (c *Client) CreateSession(ctx context.Context, smiaAccessToken string) (contract.SessionResponse, error) {
	var out contract.SessionResponse
	err := c.doToken(ctx, smiaAccessToken, http.MethodPost, "/v1/auth/session", nil, "", &out)
	return out, err
}

func (c *Client) UpsertCharacter(ctx context.Context, req contract.CharacterUpsert) (contract.Character, error) {
	raw, err := json.Marshal(req)
	if err != nil {
		return contract.Character{}, err
	}
	var out contract.Character
	err = c.do(ctx, http.MethodPut, "/v1/characters", raw, "application/json", nil, &out)
	return out, err
}

func (c *Client) Sync(ctx context.Context) (contract.SyncResponse, error) {
	var out contract.SyncResponse
	err := c.do(ctx, http.MethodGet, "/v1/sync", nil, "", nil, &out)
	return out, err
}

// SagaImage downloads a saga cover image for the addon media folder.
func (c *Client) SagaImage(ctx context.Context, questID string) (contentType string, body []byte, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.BaseURL, "/")+"/v1/sagas/"+questID+"/image", nil)
	if err != nil {
		return "", nil, err
	}
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	req.Header.Set("User-Agent", "GutSaga")
	resp, err := c.http().Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20+1))
	if err != nil {
		return "", nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", nil, &HTTPError{Status: resp.StatusCode, Message: fmt.Sprintf("image status %d", resp.StatusCode)}
	}
	return resp.Header.Get("Content-Type"), raw, nil
}

func (c *Client) Level(ctx context.Context, req contract.LevelRequest) (contract.LevelResponse, error) {
	raw, err := json.Marshal(req)
	if err != nil {
		return contract.LevelResponse{}, err
	}
	var out contract.LevelResponse
	err = c.do(ctx, http.MethodPost, "/v1/level", raw, "application/json", nil, &out)
	return out, err
}

// Complete uploads the screenshot the player took. The API posts it to Discord.
func (c *Client) Complete(ctx context.Context, req contract.CompletionRequest, image []byte) (contract.CompletionResponse, error) {
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	fields := map[string]string{
		"character_id":  req.CharacterID,
		"quest_id":      req.QuestID,
		"quest_version": strconv.Itoa(req.QuestVersion),
		"battletag":     req.BattleTag,
		"level":         strconv.Itoa(req.Level),
		"completed_at":  req.CompletedAt,
	}
	for k, v := range fields {
		if err := w.WriteField(k, v); err != nil {
			return contract.CompletionResponse{}, err
		}
	}
	contentType := ""
	if len(image) > 0 {
		normalized, ct, err := screenshot.NormalizeUpload(image)
		if err != nil {
			return contract.CompletionResponse{}, err
		}
		contentType = ct
		image = normalized
		fw, err := w.CreateFormFile("screenshot", "screenshot"+screenshot.Extension(ct))
		if err != nil {
			return contract.CompletionResponse{}, err
		}
		if _, err := fw.Write(image); err != nil {
			return contract.CompletionResponse{}, err
		}
	}
	if err := w.Close(); err != nil {
		return contract.CompletionResponse{}, err
	}
	_ = contentType
	var out contract.CompletionResponse
	err := c.do(ctx, http.MethodPost, "/v1/complete", body.Bytes(), w.FormDataContentType(), nil, &out)
	return out, err
}

func (c *Client) do(ctx context.Context, method, path string, body []byte, contentType string, _ any, out any) error {
	return c.doToken(ctx, c.APIKey, method, path, body, contentType, out)
}

func (c *Client) doToken(ctx context.Context, token, method, path string, body []byte, contentType string, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.BaseURL, "/")+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("User-Agent", "GutSaga")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := c.http().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode >= 300 {
		var eb contract.ErrorBody
		_ = json.Unmarshal(raw, &eb)
		return &HTTPError{Status: resp.StatusCode, Code: eb.Error, Message: eb.Message}
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("svar fra API: %w", err)
		}
	}
	return nil
}
