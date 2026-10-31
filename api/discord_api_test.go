package main

import (
	"context"
	"encoding/base64"
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

	"github.com/sagalore/sagalore/internal/contract"
	"github.com/sagalore/sagalore/internal/discord"
)

func TestAdminDiscordNotices(t *testing.T) {
	resetDB(t)
	fake := &fakeDiscord{}
	srv := httptest.NewServer((&Server{store: NewStore(testPool), adminToken: "admin-test", discord: fake}).Handler())
	t.Cleanup(srv.Close)
	admin := apiClient{base: srv.URL, token: "admin-test", http: srv.Client()}

	playerRec, token := devSession(t, "bnet-magnus", "Magnus#2231", "Magnus")
	devGroup(t, playerRec.ID, "tutorial", "Tutorial")
	created := contract.CreatePlayerResponse{Player: playerRec, APIKey: token}
	author := apiClient{base: srv.URL, token: token, http: srv.Client()}
	var resp *http.Response
	var err error
	var magni contract.Character
	resp, err = admin.do(http.MethodPost, "/v1/admin/characters", map[string]any{
		"player_id": created.Player.ID, "name": "Magni", "realm": "Argent Dawn",
		"race": "Dwarf", "class": "Paladin", "level": 12,
	}, &magni)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("character %d %v", statusOf(resp), err)
	}

	png := tinyPNG(t)
	var quest contract.Quest
	resp, err = author.do(http.MethodPost, "/v1/sagas", map[string]any{
		"title":       "Første vakt",
		"description": "Stå vakt ved porten.\n\nMer som ikke trengs i kortteksten.",
		"objectives": []map[string]any{
			{"id": "meet", "type": "interact", "text": "Møt opp", "npc": "Vakt"},
			{"id": "stay", "type": "location", "text": "Hold ut", "zone": "Elwynn Forest"},
		},
		"objectives_ordered":     true,
		"min_level":              10,
		"party":                  true,
		"assigned_character_ids": []string{magni.ID},
		"image_base64":           base64.StdEncoding.EncodeToString(png),
	}, &quest)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("quest %d %v", statusOf(resp), err)
	}
	if !quest.HasImage || !strings.Contains(quest.ImageURL, "/image") {
		t.Fatalf("cover %+v", quest)
	}
	fake.mu.Lock()
	if len(fake.sagas) != 1 {
		t.Fatalf("sagas %d", len(fake.sagas))
	}
	note := fake.sagas[0]
	fake.mu.Unlock()
	if note.Kind != discord.KindAnnounced || note.Title != "Første vakt" || !note.Ordered {
		t.Fatalf("note %+v", note)
	}
	if note.CreatedBy != "Magnus#2231" || note.Description == "" {
		t.Fatalf("meta %+v", note)
	}
	if len(note.Objectives) != 2 || note.Objectives[0].Text != "Møt opp" || len(note.Image) == 0 {
		t.Fatalf("objectives/image %+v img %d", note.Objectives, len(note.Image))
	}

	resp, err = author.do(http.MethodPost, "/v1/sagas", map[string]any{
		"title": "Stille utkast", "description": "Ikke klar.",
		"objectives": []map[string]any{{"id": "x", "type": "interact", "text": "Vent", "npc": "Vakt"}},
		"active":     false,
	}, &contract.Quest{})
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("draft %d %v", statusOf(resp), err)
	}
	resp, err = author.do(http.MethodPost, "/v1/sagas", map[string]any{
		"title": "Hemmelig", "description": "Ikke post.",
		"objectives": []map[string]any{{"id": "x", "type": "interact", "text": "Vent", "npc": "Vakt"}},
		"announce":   false,
	}, &contract.Quest{})
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("quiet %d %v", statusOf(resp), err)
	}
	fake.mu.Lock()
	if len(fake.sagas) != 1 {
		t.Fatalf("drafts must stay quiet, sagas %d", len(fake.sagas))
	}
	fake.mu.Unlock()

	resp, err = author.do(http.MethodPost, "/v1/sagas", map[string]any{
		"title": "Ødelagt", "description": "x",
		"objectives":   []map[string]any{{"id": "x", "type": "interact", "text": "Vent", "npc": "Vakt"}},
		"image_base64": "!!!",
	}, nil)
	if err != nil || resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad image %d %v", statusOf(resp), err)
	}

	resp, err = author.do(http.MethodPatch, "/v1/sagas/"+quest.ID, map[string]any{
		"assigned_character_ids": []string{magni.ID},
		"changes":                []string{"Tildelt Magni"},
	}, &contract.Quest{})
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("assign %d %v", statusOf(resp), err)
	}
	fake.mu.Lock()
	if len(fake.sagas) != 2 || fake.sagas[1].Kind != discord.KindUpdated || fake.sagas[1].Changes[0] != "Tildelt Magni" {
		t.Fatalf("assignment notice %+v", fake.sagas)
	}
	fake.mu.Unlock()

	var updated contract.Quest
	resp, err = author.do(http.MethodPatch, "/v1/sagas/"+quest.ID, map[string]any{
		"title": "Første vakt, igjen", "changes": []string{"Nytt navn"},
	}, &updated)
	if err != nil || resp.StatusCode != http.StatusOK || updated.Version != 3 {
		t.Fatalf("rename %+v %d %v", updated, statusOf(resp), err)
	}
	fake.mu.Lock()
	if len(fake.sagas) != 3 || fake.sagas[2].Kind != discord.KindUpdated || fake.sagas[2].Version != 3 {
		t.Fatalf("update notices %+v", fake.sagas)
	}
	if fake.sagas[2].UpdatedBy != "Magnus" || fake.sagas[2].Changes[0] != "Nytt navn" {
		t.Fatalf("updater %+v", fake.sagas[2])
	}
	fake.mu.Unlock()

	resp, err = author.do(http.MethodPatch, "/v1/sagas/"+quest.ID, map[string]any{
		"title": "Stille", "announce": false, "changes": []string{"Skjult"},
	}, &contract.Quest{})
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("silent edit %d %v", statusOf(resp), err)
	}
	active := false
	resp, err = author.do(http.MethodPatch, "/v1/sagas/"+quest.ID, map[string]any{
		"active": active, "changes": []string{"Ferdig"},
	}, &contract.Quest{})
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("retire %d %v", statusOf(resp), err)
	}
	fake.mu.Lock()
	if len(fake.sagas) != 4 || fake.sagas[3].Kind != discord.KindRetired {
		t.Fatalf("retire notices %+v", fake.sagas)
	}
	fake.mu.Unlock()

	player := apiClient{base: srv.URL, token: created.APIKey, http: srv.Client()}
	var lvl contract.LevelResponse
	resp, err = player.do(http.MethodPost, "/v1/level", contract.LevelRequest{
		CharacterID: magni.ID, BattleTag: "Magnus#2231", Level: 13,
	}, &lvl)
	if err != nil || !lvl.Changed {
		t.Fatalf("level 13 %+v %v", lvl, err)
	}
	resp, err = player.do(http.MethodPost, "/v1/level", contract.LevelRequest{
		CharacterID: magni.ID, BattleTag: "Magnus#2231", Level: 20,
	}, &lvl)
	if err != nil || lvl.Level != 20 {
		t.Fatalf("level 20 %+v %v", lvl, err)
	}
	fake.mu.Lock()
	if len(fake.levels) != 1 || fake.levels[0].Level != 20 || fake.levels[0].Character != "Magni" || fake.levels[0].Class != "" {
		t.Fatalf("levels %+v", fake.levels)
	}
	fake.mu.Unlock()
}

func TestDiscordFailureDoesNotFailTheAPI(t *testing.T) {
	resetDB(t)
	srv := httptest.NewServer((&Server{store: NewStore(testPool), adminToken: "admin-test", discord: errNotifier{}}).Handler())
	t.Cleanup(srv.Close)
	admin := apiClient{base: srv.URL, token: "admin-test", http: srv.Client()}
	playerRec, token := devSession(t, "bnet-magnus-2", "Magnus#2231", "Magnus")
	devGroup(t, playerRec.ID, "tutorial", "Tutorial")
	created := contract.CreatePlayerResponse{Player: playerRec, APIKey: token}
	author := apiClient{base: srv.URL, token: token, http: srv.Client()}
	var resp *http.Response
	var err error
	var magni contract.Character
	resp, err = admin.do(http.MethodPost, "/v1/admin/characters", map[string]any{
		"player_id": created.Player.ID, "name": "Magni", "realm": "Argent Dawn",
		"race": "Dwarf", "class": "Paladin", "level": 12,
	}, &magni)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatal(err, statusOf(resp))
	}
	var quest contract.Quest
	resp, err = author.do(http.MethodPost, "/v1/sagas", map[string]any{
		"title": "Vakt", "description": "Stå.",
		"objectives":             []map[string]any{{"id": "a", "type": "interact", "text": "Møt", "npc": "Vakt"}},
		"assigned_character_ids": []string{magni.ID},
	}, &quest)
	if err != nil || resp.StatusCode != http.StatusCreated || quest.ID == "" {
		t.Fatalf("create still succeeds: %d %+v %v", statusOf(resp), quest, err)
	}
	player := apiClient{base: srv.URL, token: created.APIKey, http: srv.Client()}
	var done contract.CompletionResponse
	resp, err = player.do(http.MethodPost, "/v1/complete", contract.CompletionRequest{
		CharacterID: magni.ID, QuestID: quest.ID, QuestVersion: 1, BattleTag: "Magnus#2231", Level: 12,
	}, &done)
	if err != nil || resp.StatusCode != http.StatusOK || !done.Completed || done.DiscordPosted {
		t.Fatalf("complete %+v status %d err %v", done, statusOf(resp), err)
	}
	if done.CharacterClass != "Paladin" || done.CharacterLevel != 12 {
		t.Fatalf("echo %+v", done)
	}
}

func TestDiscordQueueDoesNotBlockTheRequest(t *testing.T) {
	resetDB(t)
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	var once sync.Once
	started := make(chan struct{})
	var payload discordWire
	var fileName string
	var fileBody []byte
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, name, body := readWebhook(t, r)
		payload = got
		fileName = name
		fileBody = body
		once.Do(func() { close(started) })
		<-release
		http.Error(w, "down", http.StatusInternalServerError)
	}))
	t.Cleanup(hook.Close)
	q := discord.NewQueue(&discord.Client{
		WebhookURL:  hook.URL,
		HTTP:        hook.Client(),
		MaxAttempts: 1,
		Sleep:       func(context.Context, time.Duration) error { return nil },
	})
	finished := make(chan discord.Result, 4)
	q.After = func(res discord.Result) { finished <- res }

	apiSrv := httptest.NewServer((&Server{store: NewStore(testPool), adminToken: "admin-test", discord: q}).Handler())
	t.Cleanup(apiSrv.Close)
	admin := apiClient{base: apiSrv.URL, token: "admin-test", http: &http.Client{Timeout: 3 * time.Second}}
	playerRec, token := devSession(t, "bnet-magnus-3", "Magnus#2231", "Magnus")
	devGroup(t, playerRec.ID, "tutorial", "Tutorial")
	created := contract.CreatePlayerResponse{Player: playerRec, APIKey: token}
	author := apiClient{base: apiSrv.URL, token: token, http: &http.Client{Timeout: 3 * time.Second}}
	var magni contract.Character
	if resp, err := admin.do(http.MethodPost, "/v1/admin/characters", map[string]any{
		"player_id": created.Player.ID, "name": "Magni", "realm": "Argent Dawn",
		"race": "Dwarf", "class": "Paladin", "level": 12,
	}, &magni); err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatal(err, statusOf(resp))
	}

	start := time.Now()
	var quest contract.Quest
	resp, err := author.do(http.MethodPost, "/v1/sagas", map[string]any{
		"title": "Køtest", "description": "Kort tekst.",
		"objectives": []map[string]any{{"id": "a", "type": "interact", "text": "Møt opp", "npc": "Vakt"}},
		"min_level":  10, "assigned_character_ids": []string{magni.ID},
		"image_base64": base64.StdEncoding.EncodeToString(tinyPNG(t)),
	}, &quest)
	elapsed := time.Since(start)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("create %d %v", statusOf(resp), err)
	}
	if elapsed > 1500*time.Millisecond {
		t.Fatalf("create waited on discord for %s", elapsed)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("webhook was not called")
	}
	if payload.Username != "Sagalore" || !strings.Contains(payload.Content, "created a new saga") {
		t.Fatalf("payload %+v", payload)
	}
	if len(payload.Embeds) != 1 || payload.Embeds[0].Title != "Køtest" || payload.Embeds[0].Color != discord.ColorSaga || payload.Embeds[0].Image == nil {
		t.Fatalf("embed %+v", payload.Embeds)
	}
	if payload.Embeds[0].Image.URL != "attachment://"+fileName || fileName != "cover.png" || len(fileBody) == 0 {
		t.Fatalf("file %s %d image %+v", fileName, len(fileBody), payload.Embeds[0].Image)
	}
	unblock()

	player := apiClient{base: apiSrv.URL, token: created.APIKey, http: &http.Client{Timeout: 3 * time.Second}}
	var done contract.CompletionResponse
	resp, err = player.do(http.MethodPost, "/v1/complete", contract.CompletionRequest{
		CharacterID: magni.ID, QuestID: quest.ID, QuestVersion: 1, BattleTag: "Magnus#2231", Level: 12,
	}, &done)
	if err != nil || resp.StatusCode != http.StatusOK || !done.Completed || !done.DiscordPosted {
		t.Fatalf("complete while discord is down: %+v %d %v", done, statusOf(resp), err)
	}
	got := 0
	deadline := time.After(3 * time.Second)
	for got < 2 {
		select {
		case res := <-finished:
			if res.Err == nil {
				t.Fatalf("expected delivery error, %+v", res)
			}
			got++
		case <-deadline:
			t.Fatalf("deliveries %d", got)
		}
	}
}

type errNotifier struct{}

func (errNotifier) PostCompletion(context.Context, discord.Completion) (discord.Message, error) {
	return discord.Message{}, errDiscord
}
func (errNotifier) PostSaga(context.Context, discord.SagaNotice) (discord.Message, error) {
	return discord.Message{}, errDiscord
}
func (errNotifier) PostLevel(context.Context, discord.LevelNotice) (discord.Message, error) {
	return discord.Message{}, errDiscord
}

type errString string

func (e errString) Error() string { return string(e) }

const errDiscord errString = "discord down"

func encodePNG(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

type discordWire struct {
	Username string `json:"username"`
	Content  string `json:"content"`
	Embeds   []struct {
		Title string `json:"title"`
		Color int    `json:"color"`
		Image *struct {
			URL string `json:"url"`
		} `json:"image"`
	} `json:"embeds"`
}

func readWebhook(t *testing.T, r *http.Request) (discordWire, string, []byte) {
	t.Helper()
	var payload discordWire
	var name string
	var body []byte
	_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		t.Error(err)
		return payload, "", nil
	}
	mr := multipart.NewReader(r.Body, params["boundary"])
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Error(err)
			break
		}
		b, _ := io.ReadAll(part)
		switch part.FormName() {
		case "payload_json":
			if err := json.Unmarshal(b, &payload); err != nil {
				t.Error(err)
			}
		case "files[0]":
			name = part.FileName()
			body = b
		}
	}
	return payload, name, body
}
