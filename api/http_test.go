package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/magnusnilsen/gutsaga/internal/contract"
	"github.com/magnusnilsen/gutsaga/internal/discord"
	"github.com/magnusnilsen/gutsaga/internal/smia"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://gutsaga:gutsaga@127.0.0.1:5432/gutsaga_test?sslmode=disable"
	}
	u, err := url.Parse(dsn)
	if err != nil || !strings.Contains(u.Path, "test") {
		fmt.Fprintln(os.Stderr, "refusing to run API tests against a database whose name does not contain \"test\"")
		os.Exit(1)
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := pool.Ping(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "postgres:", err)
		os.Exit(1)
	}
	if err := migrate(ctx, pool); err != nil {
		fmt.Fprintln(os.Stderr, "migrate:", err)
		os.Exit(1)
	}
	if err := migrate(ctx, pool); err != nil {
		fmt.Fprintln(os.Stderr, "migrate twice:", err)
		os.Exit(1)
	}
	testPool = pool
	code := m.Run()
	pool.Close()
	os.Exit(code)
}

func resetDB(t *testing.T) {
	t.Helper()
	_, err := testPool.Exec(context.Background(), `
		TRUNCATE smia_token_cache, adventure_log, progress, screenshots, quest_versions, quest_assignments, quests, characters, sessions, players CASCADE`)
	if err != nil {
		t.Fatal(err)
	}
}

type fakeDiscord struct {
	mu    sync.Mutex
	calls int
	last  string
	image int
}

func (f *fakeDiscord) PostCompletion(_ context.Context, character, questTitle, _ string, image []byte) (discord.Message, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.last = discord.CompletionText(character, questTitle)
	f.image = len(image)
	return discord.Message{Content: f.last, AttachmentURL: "https://cdn.example/shot.png"}, nil
}

type apiClient struct {
	base  string
	token string
	http  *http.Client
}

func (c apiClient) do(method, path string, body any, out any) (*http.Response, error) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.base+path, rdr)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return resp, fmt.Errorf("status %d decode %s: %w", resp.StatusCode, raw, err)
		}
	}
	resp.Body = io.NopCloser(bytes.NewReader(raw))
	return resp, nil
}

func TestHealthz(t *testing.T) {
	srv := httptest.NewServer((&Server{store: NewStore(testPool), adminToken: "admin"}).Handler())
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

func TestCompletionIdempotentAndQuestVersions(t *testing.T) {
	resetDB(t)
	fake := &fakeDiscord{}
	srv := httptest.NewServer((&Server{store: NewStore(testPool), adminToken: "admin-test", discord: fake}).Handler())
	t.Cleanup(srv.Close)
	admin := apiClient{base: srv.URL, token: "admin-test", http: srv.Client()}

	var created contract.CreatePlayerResponse
	resp, err := admin.do(http.MethodPost, "/v1/admin/players", map[string]string{
		"battletag": "Magnus#2231", "display_name": "Magnus",
	}, &created)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("player %d %v", resp.StatusCode, err)
	}
	var other contract.CreatePlayerResponse
	resp, err = admin.do(http.MethodPost, "/v1/admin/players", map[string]string{
		"battletag": "Other#1", "display_name": "Other",
	}, &other)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatal(err, resp.StatusCode)
	}
	var magni contract.Character
	resp, err = admin.do(http.MethodPost, "/v1/admin/characters", map[string]any{
		"player_id": created.Player.ID, "name": "Magni", "realm": "Argent Dawn",
		"race": "Dwarf", "class": "Paladin", "level": 12,
	}, &magni)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("character %d %v", resp.StatusCode, err)
	}
	var quest contract.Quest
	resp, err = admin.do(http.MethodPost, "/v1/admin/quests", map[string]any{
		"title": "Første vakt", "description": "Stå vakt.",
		"objectives": []map[string]any{
			{"id": "meet", "type": "interact", "text": "Møt opp", "npc": "Vakt"},
			{"id": "stay", "type": "location", "text": "Hold ut", "zone": "Elwynn Forest"},
		},
		"reward": "Ære", "min_level": 10, "party_quest": false, "created_by": "Magnus#2231",
		"assigned_character_ids": []string{magni.ID},
	}, &quest)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("quest %d %v", resp.StatusCode, err)
	}
	if quest.Version != 1 || quest.CreatedBy != "Magnus#2231" {
		t.Fatalf("quest %+v", quest)
	}

	player := apiClient{base: srv.URL, token: created.APIKey, http: srv.Client()}
	pngBytes := tinyPNG(t)
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	for k, v := range map[string]string{
		"character_id": magni.ID, "quest_id": quest.ID, "quest_version": "1",
		"battletag": "Magnus#2231", "level": "12", "completed_at": "2026-10-22T14:44:00Z",
	} {
		if err := mw.WriteField(k, v); err != nil {
			t.Fatal(err)
		}
	}
	fw, err := mw.CreateFormFile("screenshot", "shot.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(pngBytes); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/complete", bytes.NewReader(body.Bytes()))
	req.Header.Set("Authorization", "Bearer "+created.APIKey)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err = player.http.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var first contract.CompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&first); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 || !first.Completed || first.AlreadyCompleted || !first.LogWritten || !first.DiscordPosted {
		t.Fatalf("first %+v status %d", first, resp.StatusCode)
	}
	if first.QuestVersion != 1 || first.QuestTitle != "Første vakt" {
		t.Fatalf("recorded %+v", first)
	}
	if fake.calls != 1 || !strings.Contains(fake.last, "**Magni** fullførte *Første vakt*!") || fake.image == 0 {
		t.Fatalf("discord calls=%d text=%q image=%d", fake.calls, fake.last, fake.image)
	}

	var second contract.CompletionResponse
	resp, err = player.do(http.MethodPost, "/v1/complete", contract.CompletionRequest{
		CharacterID: magni.ID, QuestID: quest.ID, QuestVersion: 1, BattleTag: "Magnus#2231", Level: 12,
	}, &second)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 || !second.AlreadyCompleted || second.LogWritten || second.QuestVersion != 1 {
		t.Fatalf("second %+v status %d", second, resp.StatusCode)
	}
	if fake.calls != 1 {
		t.Fatalf("discord posted again: %d", fake.calls)
	}

	var updated contract.Quest
	resp, err = admin.do(http.MethodPatch, "/v1/admin/quests/"+quest.ID, map[string]any{
		"title": "Første vakt, igjen",
	}, &updated)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("patch %d %v", resp.StatusCode, err)
	}
	if updated.Version != 2 || updated.Title != "Første vakt, igjen" {
		t.Fatalf("updated %+v", updated)
	}
	var detail contract.QuestDetail
	resp, err = admin.do(http.MethodGet, "/v1/admin/quests/"+quest.ID, nil, &detail)
	if err != nil || resp.StatusCode != 200 {
		t.Fatal(err, resp.StatusCode)
	}
	if len(detail.Versions) != 2 || detail.Versions[0].Title != "Første vakt" || detail.Versions[0].Version != 1 {
		t.Fatalf("snapshots %+v", detail.Versions)
	}
	if detail.Versions[1].Version != 2 || detail.Versions[1].Title != "Første vakt, igjen" {
		t.Fatalf("new snapshot %+v", detail.Versions[1])
	}

	var third contract.CompletionResponse
	resp, err = player.do(http.MethodPost, "/v1/complete", contract.CompletionRequest{
		CharacterID: magni.ID, QuestID: quest.ID, QuestVersion: 2, BattleTag: "Magnus#2231",
	}, &third)
	if err != nil {
		t.Fatal(err)
	}
	if !third.AlreadyCompleted || third.QuestVersion != 1 {
		t.Fatalf("version bump must not open a second completion: %+v", third)
	}
	if fake.calls != 1 {
		t.Fatal("discord count")
	}

	resp, err = player.do(http.MethodPost, "/v1/complete", contract.CompletionRequest{
		CharacterID: magni.ID, QuestID: quest.ID, QuestVersion: 99, BattleTag: "Magnus#2231",
	}, &contract.ErrorBody{})
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown version status %d", resp.StatusCode)
	}

	var syncOut contract.SyncResponse
	resp, err = player.do(http.MethodGet, "/v1/sync", nil, &syncOut)
	if err != nil || resp.StatusCode != 200 {
		t.Fatal(err, resp.StatusCode)
	}
	if syncOut.BattleTag != "Magnus#2231" || len(syncOut.Characters) != 1 || len(syncOut.Quests) != 1 || len(syncOut.Progress) != 1 {
		t.Fatalf("sync %+v", syncOut)
	}
	if syncOut.Quests[0].Version != 2 || syncOut.Progress[0].QuestVersion != 1 || syncOut.Progress[0].Status != "completed" {
		t.Fatalf("sync versions quest=%d progress=%+v", syncOut.Quests[0].Version, syncOut.Progress[0])
	}
	if syncOut.Progress[0].ScreenshotURL != "https://cdn.example/shot.png" {
		t.Fatalf("screenshot url %q", syncOut.Progress[0].ScreenshotURL)
	}
	if len(syncOut.Completions) != 1 || syncOut.Completions[0].Character != "Magni" || syncOut.Completions[0].QuestID != quest.ID {
		t.Fatalf("completions %+v", syncOut.Completions)
	}

	var logs struct {
		Entries []contract.LogEntry `json:"entries"`
	}
	resp, err = admin.do(http.MethodGet, "/v1/admin/log?battletag=Magnus%232231", nil, &logs)
	if err != nil || resp.StatusCode != 200 {
		t.Fatal(err, resp.StatusCode)
	}
	if len(logs.Entries) != 1 || !strings.Contains(logs.Entries[0].Body, "Magni fullførte") {
		t.Fatalf("log %+v", logs.Entries)
	}

	var listed struct {
		Quests []contract.QuestListItem `json:"quests"`
	}
	resp, err = admin.do(http.MethodGet, "/v1/admin/quests?character_id="+magni.ID, nil, &listed)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Quests) != 1 || listed.Quests[0].Status == nil || *listed.Quests[0].Status != "completed" || listed.Quests[0].Eligible == nil || !*listed.Quests[0].Eligible {
		t.Fatalf("list %+v", listed.Quests)
	}

	var lvl contract.LevelResponse
	resp, err = player.do(http.MethodPost, "/v1/level", contract.LevelRequest{
		CharacterID: magni.ID, BattleTag: "Magnus#2231", Level: 13,
	}, &lvl)
	if err != nil || !lvl.Changed || lvl.Level != 13 {
		t.Fatalf("level %+v %v status %d", lvl, err, statusOf(resp))
	}
	resp, err = player.do(http.MethodPost, "/v1/level", contract.LevelRequest{
		CharacterID: magni.ID, BattleTag: "Magnus#2231", Level: 13,
	}, &lvl)
	if err != nil || lvl.Changed {
		t.Fatalf("level repeat %+v", lvl)
	}

	resp, err = player.do(http.MethodPost, "/v1/complete", contract.CompletionRequest{
		CharacterID: magni.ID, QuestID: quest.ID, QuestVersion: 1, BattleTag: "Other#1",
	}, nil)
	if err != nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("mismatched tag %d", statusOf(resp))
	}
	otherClient := apiClient{base: srv.URL, token: other.APIKey, http: srv.Client()}
	resp, err = otherClient.do(http.MethodPost, "/v1/complete", contract.CompletionRequest{
		CharacterID: magni.ID, QuestID: quest.ID, QuestVersion: 1, BattleTag: "Other#1",
	}, nil)
	if err != nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("other player %d", statusOf(resp))
	}
	resp, err = http.Get(srv.URL + "/v1/sync")
	if err != nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no key %d", statusOf(resp))
	}
}

type fakeSmia struct{}

func (fakeSmia) Verify(_ context.Context, token string) (smia.Identity, error) {
	switch token {
	case "good":
		return smia.Identity{Subject: "user-1", Email: "magnus@example.com", DisplayName: "Magnus", Member: true, Hub: "wow", Server: "mcp"}, nil
	case "outsider":
		return smia.Identity{Subject: "user-2", Email: "nope@example.com", Member: false, Hub: "wow", Server: "mcp"}, nil
	default:
		return smia.Identity{}, smia.ErrUnauthorized
	}
}

func TestSmiaSessionUpsertsCharacter(t *testing.T) {
	resetDB(t)
	srv := httptest.NewServer((&Server{store: NewStore(testPool), smia: fakeSmia{}}).Handler())
	t.Cleanup(srv.Close)
	client := apiClient{base: srv.URL, token: "outsider", http: srv.Client()}
	resp, err := client.do(http.MethodPost, "/v1/auth/session", map[string]string{"email": "forged@example.com"}, nil)
	if err != nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("outsider %d %v", statusOf(resp), err)
	}
	client.token = "nope"
	resp, err = client.do(http.MethodPost, "/v1/auth/session", nil, nil)
	if err != nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bad token %d", statusOf(resp))
	}
	var session contract.SessionResponse
	client.token = "good"
	resp, err = client.do(http.MethodPost, "/v1/auth/session", map[string]string{"email": "forged@example.com", "sub": "not-trusted"}, &session)
	if err != nil || resp.StatusCode != http.StatusOK || !strings.HasPrefix(session.SessionToken, "gss_") {
		t.Fatalf("session %+v %d %v", session, statusOf(resp), err)
	}
	if session.Player.Email != "magnus@example.com" || session.Player.SmiaSubject != "user-1" {
		t.Fatalf("identity must come from Smia, got %+v", session.Player)
	}
	player := apiClient{base: srv.URL, token: session.SessionToken, http: srv.Client()}
	var ch contract.Character
	resp, err = player.do(http.MethodPut, "/v1/characters", contract.CharacterUpsert{
		Name: "Magni", Realm: "Stormwind", Level: 12, Race: "Dwarf", Class: "Warrior", BattleTag: "Magnus#2231",
	}, &ch)
	if err != nil || resp.StatusCode != http.StatusOK || ch.Name != "Magni" || ch.BattleTag != "Magnus#2231" {
		t.Fatalf("upsert %+v %d %v", ch, statusOf(resp), err)
	}
	resp, err = player.do(http.MethodPut, "/v1/characters", contract.CharacterUpsert{
		Name: "Magni", Realm: "Stormwind", Level: 13, Race: "Dwarf", Class: "Warrior", BattleTag: "Magnus#2231",
	}, &ch)
	if err != nil || ch.Level != 13 || ch.ID == "" {
		t.Fatalf("second %+v %v", ch, err)
	}
	var sync contract.SyncResponse
	resp, err = player.do(http.MethodGet, "/v1/sync", nil, &sync)
	if err != nil || resp.StatusCode != 200 || len(sync.Characters) != 1 || sync.BattleTag != "Magnus#2231" {
		t.Fatalf("sync %+v %d", sync, statusOf(resp))
	}
}

func TestSeedQuests(t *testing.T) {
	resetDB(t)
	store := NewStore(testPool)
	created, updated, err := seed(context.Background(), store)
	if err != nil || created != 3 || updated != 0 {
		t.Fatalf("created %d updated %d err %v", created, updated, err)
	}
	created, updated, err = seed(context.Background(), store)
	if err != nil || created != 0 || updated != 3 {
		t.Fatalf("second created %d updated %d err %v", created, updated, err)
	}
	list, err := store.ListQuests(context.Background(), "")
	if err != nil || len(list) != 3 {
		t.Fatalf("list %d %v", len(list), err)
	}
	for _, q := range list {
		if !q.PartyQuest || len(q.Objectives) == 0 {
			t.Fatalf("%s objectives %+v", q.Title, q.Objectives)
		}
	}
}

func statusOf(resp *http.Response) int {
	if resp == nil {
		return 0
	}
	return resp.StatusCode
}

func tinyPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
