package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/magnusnilsen/gutsaga/internal/contract"
	"github.com/magnusnilsen/gutsaga/internal/discord"
	"github.com/magnusnilsen/gutsaga/internal/screenshot"
	"github.com/magnusnilsen/gutsaga/internal/smia"
)

type Notifier interface {
	PostCompletion(ctx context.Context, character, questTitle, contentType string, image []byte) (discord.Message, error)
}

type Server struct {
	store      *Store
	adminToken string
	discord    Notifier
	smia       smia.Verifier
}

type ctxKey int

const playerCtx ctxKey = 1

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("POST /v1/auth/session", s.session)
	mux.HandleFunc("PUT /v1/characters", s.withPlayer(s.upsertCharacter))
	mux.HandleFunc("GET /v1/sync", s.withPlayer(s.sync))
	mux.HandleFunc("POST /v1/complete", s.withPlayer(s.complete))
	mux.HandleFunc("POST /v1/level", s.withPlayer(s.level))
	mux.HandleFunc("POST /v1/sagas", s.withPlayer(s.createSaga))
	mux.HandleFunc("GET /v1/sagas", s.withPlayer(s.listSagas))
	mux.HandleFunc("GET /v1/sagas/{id}", s.withPlayer(s.getSaga))
	mux.HandleFunc("PATCH /v1/sagas/{id}", s.withPlayer(s.patchSaga))
	mux.HandleFunc("PUT /v1/sagas/{id}/image", s.withPlayer(s.putSagaImage))
	mux.HandleFunc("DELETE /v1/sagas/{id}/image", s.withPlayer(s.deleteSagaImage))
	mux.HandleFunc("GET /v1/sagas/{id}/image", s.withPlayer(s.getSagaImage))
	mux.HandleFunc("POST /v1/admin/players", s.withAdmin(s.createPlayer))
	mux.HandleFunc("GET /v1/admin/players", s.withAdmin(s.listPlayers))
	mux.HandleFunc("POST /v1/admin/players/{id}/rotate-key", s.withAdmin(s.rotateKey))
	mux.HandleFunc("POST /v1/admin/characters", s.withAdmin(s.createCharacter))
	mux.HandleFunc("GET /v1/admin/characters", s.withAdmin(s.listCharacters))
	mux.HandleFunc("GET /v1/admin/characters/{id}", s.withAdmin(s.getCharacter))
	mux.HandleFunc("PATCH /v1/admin/characters/{id}", s.withAdmin(s.patchCharacter))
	mux.HandleFunc("DELETE /v1/admin/characters/{id}", s.withAdmin(s.deleteCharacter))
	mux.HandleFunc("POST /v1/admin/quests", s.withAdmin(s.createQuest))
	mux.HandleFunc("GET /v1/admin/quests", s.withAdmin(s.listQuests))
	mux.HandleFunc("GET /v1/admin/quests/{id}", s.withAdmin(s.getQuest))
	mux.HandleFunc("PATCH /v1/admin/quests/{id}", s.withAdmin(s.patchQuest))
	mux.HandleFunc("GET /v1/admin/log", s.withAdmin(s.listLog))
	mux.HandleFunc("POST /v1/admin/log", s.withAdmin(s.createLog))
	return mux
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	if err := s.store.Ping(r.Context()); err != nil {
		writeErr(w, http.StatusServiceUnavailable, "unhealthy", "database unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) withPlayer(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := bearer(r)
		if key == "" {
			writeErr(w, http.StatusUnauthorized, "unauthorized", "missing API key")
			return
		}
		p, err := s.store.PlayerByKey(r.Context(), key)
		if errors.Is(err, ErrForbidden) {
			writeErr(w, http.StatusUnauthorized, "unauthorized", "invalid API key")
			return
		}
		if err != nil {
			s.fail(w, err)
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), playerCtx, p)))
	}
}

func (s *Server) withAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.adminToken == "" {
			writeErr(w, http.StatusServiceUnavailable, "admin_disabled", "ADMIN_TOKEN is not set")
			return
		}
		got := bearer(r)
		if got == "" || subtleConstantTimeString(got, s.adminToken) == 0 {
			writeErr(w, http.StatusUnauthorized, "unauthorized", "invalid admin token")
			return
		}
		next(w, r)
	}
}

func playerFrom(r *http.Request) contract.Player {
	p, _ := r.Context().Value(playerCtx).(contract.Player)
	return p
}

func (s *Server) sync(w http.ResponseWriter, r *http.Request) {
	p := playerFrom(r)
	if q := strings.TrimSpace(r.URL.Query().Get("battletag")); q != "" && !strings.EqualFold(q, p.BattleTag) {
		writeErr(w, http.StatusForbidden, "forbidden", "battletag does not match the API key")
		return
	}
	out, err := s.store.Sync(r.Context(), p.ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) complete(w http.ResponseWriter, r *http.Request) {
	p := playerFrom(r)
	in, image, contentType, err := readCompletion(w, r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid", err.Error())
		return
	}
	if len(image) > 0 {
		image, contentType, err = screenshot.NormalizeUpload(image)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "invalid", err.Error())
			return
		}
	}
	res, err := s.store.Complete(r.Context(), p.ID, in, image, contentType)
	if err != nil {
		if errors.Is(err, ErrForbidden) {
			writeJSON(w, http.StatusForbidden, res)
			return
		}
		s.fail(w, err)
		return
	}
	if res.Completed && !res.AlreadyCompleted && s.discord != nil {
		msg, derr := s.discord.PostCompletion(r.Context(), res.CharacterName, res.QuestTitle, contentType, image)
		if derr == nil {
			res.DiscordPosted = true
			if err := s.store.MarkDiscord(r.Context(), res.ProgressID, msg.AttachmentURL); err != nil {
				slog.Error("mark discord", "err", err)
			}
		} else if !errors.Is(derr, discord.ErrNotConfigured) {
			slog.Error("discord post", "err", derr)
		}
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) level(w http.ResponseWriter, r *http.Request) {
	p := playerFrom(r)
	var in contract.LevelRequest
	if err := readJSON(r, &in, 1<<20); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid", "bad json")
		return
	}
	res, err := s.store.SetLevel(r.Context(), p.ID, in)
	if err != nil {
		if errors.Is(err, ErrForbidden) {
			writeJSON(w, http.StatusForbidden, res)
			return
		}
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) createPlayer(w http.ResponseWriter, r *http.Request) {
	var body struct {
		BattleTag   string `json:"battletag"`
		DisplayName string `json:"display_name"`
	}
	if err := readJSON(r, &body, 1<<20); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid", "bad json")
		return
	}
	out, err := s.store.CreatePlayer(r.Context(), body.BattleTag, body.DisplayName)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) listPlayers(w http.ResponseWriter, r *http.Request) {
	out, err := s.store.ListPlayers(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	if out == nil {
		out = []contract.Player{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"players": out})
}

func (s *Server) rotateKey(w http.ResponseWriter, r *http.Request) {
	out, err := s.store.RotateKey(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) createCharacter(w http.ResponseWriter, r *http.Request) {
	var body characterBody
	if err := readJSON(r, &body, 1<<20); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid", "bad json")
		return
	}
	level := 0
	if body.Level != nil {
		level = *body.Level
	}
	backstory := ""
	if body.Backstory != nil {
		backstory = *body.Backstory
	}
	out, err := s.store.CreateCharacter(r.Context(), CharacterInput{
		PlayerID: body.PlayerID, BattleTag: body.BattleTag, Name: body.Name, Realm: body.Realm,
		Race: body.Race, Class: body.Class, Level: level, Backstory: backstory,
	})
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) listCharacters(w http.ResponseWriter, r *http.Request) {
	out, err := s.store.ListCharacters(r.Context(), r.URL.Query().Get("battletag"))
	if err != nil {
		s.fail(w, err)
		return
	}
	if out == nil {
		out = []contract.Character{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"characters": out})
}

func (s *Server) getCharacter(w http.ResponseWriter, r *http.Request) {
	out, err := s.store.GetCharacter(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) patchCharacter(w http.ResponseWriter, r *http.Request) {
	var body characterBody
	if err := readJSON(r, &body, 1<<20); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid", "bad json")
		return
	}
	out, err := s.store.UpdateCharacter(r.Context(), r.PathValue("id"), CharacterPatch{
		Name: body.NamePtr, Realm: body.RealmPtr, Race: body.RacePtr, Class: body.ClassPtr,
		Level: body.Level, Backstory: body.Backstory,
	})
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) deleteCharacter(w http.ResponseWriter, r *http.Request) {
	if err := s.store.DeleteCharacter(r.Context(), r.PathValue("id")); err != nil {
		s.fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) createQuest(w http.ResponseWriter, r *http.Request) {
	var body questBody
	if err := readJSON(r, &body, 1<<20); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid", "bad json")
		return
	}
	out, err := s.store.CreateQuest(r.Context(), QuestInput{
		Title: body.Title, Description: body.Description, Objectives: body.Objectives,
		ObjectivesOrdered: body.ObjectivesOrdered, Reward: body.Reward,
		MinLevel: body.MinLevel, PartyQuest: body.PartyQuest, Active: body.Active, CreatedBy: body.CreatedBy,
		AssignedCharacterIDs: body.AssignedCharacterIDs,
	})
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, withSagaImageURL(out))
}

func (s *Server) listQuests(w http.ResponseWriter, r *http.Request) {
	out, err := s.store.ListQuests(r.Context(), r.URL.Query().Get("character_id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"quests": out})
}

func (s *Server) getQuest(w http.ResponseWriter, r *http.Request) {
	out, err := s.store.GetQuest(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, err)
		return
	}
	out.Quest = withSagaImageURL(out.Quest)
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) patchQuest(w http.ResponseWriter, r *http.Request) {
	var body questPatchBody
	if err := readJSON(r, &body, 1<<20); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid", "bad json")
		return
	}
	patch := QuestPatch{
		Title: body.Title, Description: body.Description, Objectives: body.Objectives,
		ObjectivesOrdered: body.ObjectivesOrdered, Reward: body.Reward,
		PartyQuest: body.PartyQuest, Active: body.Active, CreatedBy: body.CreatedBy,
		AssignedCharacterIDs: body.AssignedCharacterIDs,
	}
	if len(body.MinLevel) > 0 {
		patch.MinLevelSet = true
		if string(body.MinLevel) != "null" {
			var n int
			if err := json.Unmarshal(body.MinLevel, &n); err != nil {
				writeErr(w, http.StatusBadRequest, "invalid", "min_level")
				return
			}
			if n != 0 {
				patch.MinLevel = &n
			}
		}
	}
	out, err := s.store.UpdateQuest(r.Context(), r.PathValue("id"), patch)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, withSagaImageURL(out))
}

func (s *Server) listLog(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	out, err := s.store.ListLog(r.Context(), r.URL.Query().Get("battletag"), r.URL.Query().Get("character_id"), limit)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": out})
}

func (s *Server) createLog(w http.ResponseWriter, r *http.Request) {
	var body struct {
		BattleTag   string `json:"battletag"`
		PlayerID    string `json:"player_id"`
		CharacterID string `json:"character_id"`
		QuestID     string `json:"quest_id"`
		Type        string `json:"type"`
		Title       string `json:"title"`
		Body        string `json:"body"`
	}
	if err := readJSON(r, &body, 1<<20); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid", "bad json")
		return
	}
	playerID := body.PlayerID
	if playerID == "" && body.BattleTag != "" {
		id, err := s.store.resolvePlayerID(r.Context(), s.store.pool, "", body.BattleTag)
		if err != nil {
			s.fail(w, err)
			return
		}
		playerID = id
	}
	out, err := s.store.AddLog(r.Context(), playerID, body.CharacterID, body.QuestID, body.Type, body.Title, body.Body)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		writeErr(w, http.StatusNotFound, "not_found", "not found")
	case errors.Is(err, ErrConflict):
		writeErr(w, http.StatusConflict, "conflict", "already exists")
	case errors.Is(err, ErrInvalid):
		writeErr(w, http.StatusBadRequest, "invalid", err.Error())
	case errors.Is(err, ErrForbidden):
		writeErr(w, http.StatusForbidden, "forbidden", "forbidden")
	case errors.Is(err, smia.ErrUnauthorized):
		writeErr(w, http.StatusUnauthorized, "unauthorized", "Smia avslo tokenet")
	case errors.Is(err, smia.ErrNotConfigured):
		writeErr(w, http.StatusServiceUnavailable, "smia_not_configured", "Smia-innlogging er ikke satt opp")
	case errors.Is(err, smia.ErrUnavailable):
		writeErr(w, http.StatusBadGateway, "smia_unavailable", "fikk ikke svar fra Smia")
	default:
		slog.Error("request failed", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal", "internal error")
	}
}

type characterBody struct {
	PlayerID  string  `json:"player_id"`
	BattleTag string  `json:"battletag"`
	Name      string  `json:"name"`
	Realm     string  `json:"realm"`
	Race      string  `json:"race"`
	Class     string  `json:"class"`
	Level     *int    `json:"level"`
	Backstory *string `json:"backstory"`
	NamePtr   *string `json:"-"`
	RealmPtr  *string `json:"-"`
	RacePtr   *string `json:"-"`
	ClassPtr  *string `json:"-"`
}

func (b *characterBody) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	assign := func(key string, dst any) error {
		if v, ok := raw[key]; ok {
			return json.Unmarshal(v, dst)
		}
		return nil
	}
	_ = assign("player_id", &b.PlayerID)
	_ = assign("battletag", &b.BattleTag)
	if _, ok := raw["name"]; ok {
		var v string
		if err := json.Unmarshal(raw["name"], &v); err != nil {
			return err
		}
		b.Name = v
		b.NamePtr = &v
	}
	if _, ok := raw["realm"]; ok {
		var v string
		if err := json.Unmarshal(raw["realm"], &v); err != nil {
			return err
		}
		b.Realm = v
		b.RealmPtr = &v
	}
	if _, ok := raw["race"]; ok {
		var v string
		if err := json.Unmarshal(raw["race"], &v); err != nil {
			return err
		}
		b.Race = v
		b.RacePtr = &v
	}
	if _, ok := raw["class"]; ok {
		var v string
		if err := json.Unmarshal(raw["class"], &v); err != nil {
			return err
		}
		b.Class = v
		b.ClassPtr = &v
	}
	if _, ok := raw["level"]; ok {
		var v int
		if err := json.Unmarshal(raw["level"], &v); err != nil {
			return err
		}
		b.Level = &v
	}
	if _, ok := raw["backstory"]; ok {
		var v string
		if err := json.Unmarshal(raw["backstory"], &v); err != nil {
			return err
		}
		b.Backstory = &v
	}
	return nil
}

type questBody struct {
	Title                string               `json:"title"`
	Description          string               `json:"description"`
	Objectives           []contract.Objective `json:"objectives"`
	ObjectivesOrdered    *bool                `json:"objectives_ordered"`
	Reward               string               `json:"reward"`
	MinLevel             *int                 `json:"min_level"`
	PartyQuest           bool                 `json:"party_quest"`
	Active               *bool                `json:"active"`
	CreatedBy            string               `json:"created_by"`
	AssignedCharacterIDs []string             `json:"assigned_character_ids"`
}

type questPatchBody struct {
	Title                *string               `json:"title"`
	Description          *string               `json:"description"`
	Objectives           *[]contract.Objective `json:"objectives"`
	ObjectivesOrdered    *bool                 `json:"objectives_ordered"`
	Reward               *string               `json:"reward"`
	MinLevel             json.RawMessage       `json:"min_level"`
	PartyQuest           *bool                 `json:"party_quest"`
	Active               *bool                 `json:"active"`
	CreatedBy            *string               `json:"created_by"`
	AssignedCharacterIDs *[]string             `json:"assigned_character_ids"`
}

func readCompletion(w http.ResponseWriter, r *http.Request) (contract.CompletionRequest, []byte, string, error) {
	var in contract.CompletionRequest
	ct := r.Header.Get("Content-Type")
	if strings.HasPrefix(ct, "multipart/") {
		r.Body = http.MaxBytesReader(w, r.Body, 12<<20)
		if err := r.ParseMultipartForm(12 << 20); err != nil {
			return in, nil, "", err
		}
		in.CharacterID = r.FormValue("character_id")
		in.QuestID = r.FormValue("quest_id")
		in.QuestVersion, _ = strconv.Atoi(r.FormValue("quest_version"))
		in.BattleTag = r.FormValue("battletag")
		in.Level, _ = strconv.Atoi(r.FormValue("level"))
		in.CompletedAt = r.FormValue("completed_at")
		file, hdr, err := r.FormFile("screenshot")
		if err == nil {
			defer file.Close()
			b, err := io.ReadAll(io.LimitReader(file, 8<<20+1))
			if err != nil {
				return in, nil, "", err
			}
			return in, b, hdr.Header.Get("Content-Type"), nil
		}
		return in, nil, "", nil
	}
	if err := readJSON(r, &in, 12<<20); err != nil {
		return in, nil, "", err
	}
	if in.ScreenshotBase64 != "" {
		b, err := base64.StdEncoding.DecodeString(in.ScreenshotBase64)
		if err != nil {
			return in, nil, "", err
		}
		return in, b, "application/octet-stream", nil
	}
	return in, nil, "", nil
}

func readJSON(r *http.Request, dst any, max int64) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, max))
	return dec.Decode(dst)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, contract.ErrorBody{Error: code, Message: msg})
}

func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	const p = "Bearer "
	if len(h) < len(p) || !strings.EqualFold(h[:len(p)], p[:len(p)]) {
		return ""
	}
	return strings.TrimSpace(h[len(p):])
}
