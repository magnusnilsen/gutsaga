package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/magnusnilsen/gutsaga/internal/contract"
	"github.com/magnusnilsen/gutsaga/internal/discord"
	"github.com/magnusnilsen/gutsaga/internal/screenshot"
)

// The production notifier is a discord.Queue so HTTP handlers return
// before Discord answers. A synchronous Client also satisfies Notifier.
var (
	_ Notifier = (*discord.Client)(nil)
	_ Notifier = (*discord.Queue)(nil)
)

// Discord notices are best-effort. postDiscord logs provider failures and
// never returns them to the HTTP handler. Queue.Post returns before Discord
// answers, so a slow or rate-limited webhook does not hold the request.
func (s *Server) postDiscord(ctx context.Context, fn func(context.Context, Notifier) (discord.Message, error)) (discord.Message, bool) {
	if s == nil || s.discord == nil {
		return discord.Message{}, false
	}
	msg, err := fn(ctx, s.discord)
	if err != nil {
		if !errors.Is(err, discord.ErrNotConfigured) {
			slog.Error("discord", "err", err)
		}
		return discord.Message{}, false
	}
	return msg, true
}

func (s *Server) announceCreated(ctx context.Context, q contract.Quest, flag *bool, cover []byte, coverType string) {
	if !announceCreate(flag, q.Active) || s.discord == nil {
		return
	}
	img, ct := cover, coverType
	if len(img) == 0 && q.ID != "" {
		if c, b, err := s.store.QuestImage(ctx, q.ID); err == nil {
			img, ct = b, c
		}
	}
	notice := sagaNotice(ctx, s.store, discord.KindAnnounced, q, "", img, ct)
	s.attachGroupDiscord(ctx, q.ID, &notice.WebhookURL, &notice.Locale)
	s.postDiscord(ctx, func(ctx context.Context, n Notifier) (discord.Message, error) {
		return n.PostSaga(ctx, notice)
	})
}

// announceEdited posts an update or retirement. actor is a short display name
// (e.g. "Magnus"); empty falls back to the quest's created_by BattleTag name.
func (s *Server) announceEdited(ctx context.Context, before, after contract.Quest, flag *bool, actor string, changes []string) {
	if before.ID == "" || after.ID == "" || !announceUpdate(flag) || s.discord == nil {
		return
	}
	kind := ""
	switch {
	case before.Active && !after.Active:
		kind = discord.KindRetired
	case after.Active && after.Version > before.Version:
		kind = discord.KindUpdated
	default:
		return
	}
	notice := sagaNotice(ctx, s.store, kind, after, actor, nil, "")
	notice.Changes = changes
	s.attachGroupDiscord(ctx, after.ID, &notice.WebhookURL, &notice.Locale)
	s.postDiscord(ctx, func(ctx context.Context, n Notifier) (discord.Message, error) {
		return n.PostSaga(ctx, notice)
	})
}

func (s *Server) announceLevel(ctx context.Context, before contract.Character, level int) {
	if s.discord == nil || strings.TrimSpace(before.Name) == "" || level <= before.Level || !discord.Milestone(level) {
		return
	}
	s.postDiscord(ctx, func(ctx context.Context, n Notifier) (discord.Message, error) {
		return n.PostLevel(ctx, discord.LevelNotice{
			Character: before.Name,
			Level:     level,
			When:      time.Now().UTC(),
		})
	})
}

func sagaNotice(_ context.Context, _ *Store, kind string, q contract.Quest, actor string, image []byte, contentType string) discord.SagaNotice {
	lines := make([]discord.ObjectiveLine, 0, len(q.Objectives))
	for _, o := range q.Objectives {
		lines = append(lines, discord.ObjectiveLine{Text: o.Text})
	}
	updatedBy := strings.TrimSpace(actor)
	if updatedBy == "" {
		updatedBy = displayActor("", "", q.CreatedBy)
	}
	return discord.SagaNotice{
		Kind:        kind,
		Title:       q.Title,
		Description: q.Description,
		Objectives:  lines,
		Ordered:     q.ObjectivesOrdered,
		CreatedBy:   q.CreatedBy,
		UpdatedBy:   updatedBy,
		Version:     q.Version,
		When:        time.Now().UTC(),
		Image:       image,
		ContentType: contentType,
	}
}

// displayActor picks a short name for Discord lines. Prefer a real display
// name, then the BattleTag name, then the email local-part.
func displayActor(displayName, email, battleTag string) string {
	name := strings.TrimSpace(displayName)
	if name != "" && !strings.Contains(name, "@") {
		return name
	}
	if tag := strings.TrimSpace(battleTag); tag != "" {
		if i := strings.IndexByte(tag, '#'); i > 0 {
			return tag[:i]
		}
		return tag
	}
	if name != "" {
		if i := strings.IndexByte(name, '@'); i > 0 {
			return name[:i]
		}
		return name
	}
	if mail := strings.TrimSpace(email); mail != "" {
		if i := strings.IndexByte(mail, '@'); i > 0 {
			return mail[:i]
		}
		return mail
	}
	return ""
}

func (s *Server) attachGroupDiscord(ctx context.Context, questID string, webhook, locale *string) {
	if s == nil || s.store == nil || questID == "" {
		return
	}
	hook, lang, err := s.store.GroupDiscordForQuest(ctx, questID)
	if err != nil {
		return
	}
	if webhook != nil {
		*webhook = hook
	}
	if locale != nil {
		*locale = lang
	}
}

// announceCreate reports whether a new saga should be posted.
// An omitted flag announces active sagas. announce:false skips, announce:true posts even a draft.
func announceCreate(flag *bool, active bool) bool {
	if flag != nil {
		return *flag
	}
	return active
}

// announceUpdate reports whether a patch may post a version note or a retirement.
func announceUpdate(flag *bool) bool {
	return flag == nil || *flag
}

// significantQuestChange is a text, goal, level, or party change.
func significantQuestChange(before, after contract.Quest) bool {
	return questContentKey(before) != questContentKey(after)
}

func questContentKey(q contract.Quest) string {
	raw, _ := json.Marshal(struct {
		Title       string               `json:"title"`
		Description string               `json:"description"`
		Objectives  []contract.Objective `json:"objectives"`
		Ordered     bool                 `json:"ordered"`
		MinLevel    *int                 `json:"min_level"`
		Party       bool                 `json:"party"`
	}{
		Title: q.Title, Description: q.Description, Objectives: q.Objectives,
		Ordered: q.ObjectivesOrdered, MinLevel: q.MinLevel, Party: q.PartyQuest,
	})
	return string(raw)
}

func decodeCover(b64 string) ([]byte, string, error) {
	b64 = strings.TrimSpace(b64)
	if b64 == "" {
		return nil, "", nil
	}
	compact := strings.Map(func(r rune) rune {
		switch r {
		case '\n', '\r', ' ', '\t':
			return -1
		default:
			return r
		}
	}, b64)
	raw, err := base64.StdEncoding.DecodeString(compact)
	if err != nil {
		raw, err = base64.RawStdEncoding.DecodeString(compact)
	}
	if err != nil {
		return nil, "", errors.New("image_base64")
	}
	return screenshot.NormalizeUpload(raw)
}

// SetQuestCover stores a cover without bumping the saga version.
func (s *Store) SetQuestCover(ctx context.Context, questID, contentType string, image []byte) error {
	if err := validUUID(questID); err != nil {
		return err
	}
	if len(image) == 0 {
		return errors.New("empty image")
	}
	ct := strings.TrimSpace(contentType)
	if ct == "" {
		ct = "application/octet-stream"
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE quests SET image_bytes=$2, image_content_type=$3, updated_at=now() WHERE id=$1`,
		questID, image, ct)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
