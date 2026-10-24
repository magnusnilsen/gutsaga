// Package app runs the GutSaga companion: install the addon, sync quests, and
// report completions from the chat log and SavedVariables. It never sends input
// to the game.
package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/magnusnilsen/gutsaga/internal/backend"
	"github.com/magnusnilsen/gutsaga/internal/chatlog"
	"github.com/magnusnilsen/gutsaga/internal/config"
	"github.com/magnusnilsen/gutsaga/internal/contract"
	"github.com/magnusnilsen/gutsaga/internal/install"
	"github.com/magnusnilsen/gutsaga/internal/queue"
	"github.com/magnusnilsen/gutsaga/internal/savedvars"
	"github.com/magnusnilsen/gutsaga/internal/screenshot"
	"github.com/magnusnilsen/gutsaga/internal/syncdata"
	"github.com/magnusnilsen/gutsaga/internal/update"
	"github.com/magnusnilsen/gutsaga/internal/version"
	"github.com/magnusnilsen/gutsaga/internal/wowpath"
)

type App struct {
	Config     config.Config
	Log        *slog.Logger
	Client     *backend.Client
	Queue      *queue.Queue
	WowDir     string
	NoUpdate   bool
	Prompt     func() (string, error)
	sync       contract.SyncResponse
	follow     follower
	mu         sync.Mutex
	loginMu    sync.Mutex
	loginTried bool
}

type follower struct {
	path    string
	offset  int64
	partial string
}

func New(cfg config.Config, queuePath string, log *slog.Logger) (*App, error) {
	if log == nil {
		log = slog.Default()
	}
	q, err := queue.Open(queuePath)
	if err != nil {
		return nil, err
	}
	cfg.Normalize()
	return &App{
		Config: cfg,
		Log:    log,
		Client: &backend.Client{BaseURL: cfg.APIBaseURL, APIKey: cfg.Token()},
		Queue:  q,
	}, nil
}

func (a *App) Run(ctx context.Context) {
	a.bootstrap(ctx)
	if !a.NoUpdate {
		a.maybeUpdate(ctx)
	}
	tailTick := time.NewTicker(time.Second)
	flushTick := time.NewTicker(5 * time.Second)
	syncTick := time.NewTicker(10 * time.Minute)
	updateTick := time.NewTicker(4 * time.Hour)
	defer tailTick.Stop()
	defer flushTick.Stop()
	defer syncTick.Stop()
	defer updateTick.Stop()
	a.Flush(ctx)
	for {
		select {
		case <-ctx.Done():
			if err := a.Queue.Save(); err != nil {
				a.Log.Error("kunne ikke lagre køen", "err", err)
			}
			return
		case <-tailTick.C:
			a.pollChat()
			a.pollSavedVars()
		case <-flushTick.C:
			a.Flush(ctx)
		case <-syncTick.C:
			a.bootstrap(ctx)
		case <-updateTick.C:
			if !a.NoUpdate {
				a.maybeUpdate(ctx)
			}
		}
	}
}

func (a *App) SyncNow(ctx context.Context) error {
	return a.bootstrap(ctx)
}

func (a *App) bootstrap(ctx context.Context) error {
	if a.WowDir == "" {
		dir, err := wowpath.DetectClient(a.Config.WowPath, wowpath.Candidates(), a.Config.Client)
		if err != nil {
			a.Log.Warn(err.Error())
		} else {
			a.WowDir = dir
			a.follow.path = wowpath.ChatLog(dir)
			a.Log.Info("fant WoW", "path", dir)
			if a.Config.WowPath == "" {
				a.Config.WowPath = dir
				if err := config.Save(a.Config); err != nil {
					a.Log.Warn("kunne ikke lagre wow_path", "err", err)
				}
			}
		}
	}
	if a.WowDir != "" {
		if err := install.Install(a.WowDir, a.Config.Client); err != nil {
			a.Log.Error("installasjon av tillegget feilet", "err", err)
		}
	}
	if a.Client.APIKey == "" {
		if err := a.ensureLogin(ctx); err != nil {
			a.Log.Warn("ikke innlogget med Smia", "err", err)
			return err
		}
	}
	if err := a.pullSync(ctx); err != nil {
		a.Log.Warn("synkronisering feilet", "err", err)
		return err
	}
	return nil
}

func (a *App) pullSync(ctx context.Context) error {
	if a.Client == nil || a.Config.APIBaseURL == "" {
		return errors.New("API er ikke satt opp")
	}
	sync, err := a.Client.Sync(ctx)
	var httpErr *backend.HTTPError
	if errors.As(err, &httpErr) && httpErr.Status == 401 {
		a.clearSession()
		if err = a.ensureLogin(ctx); err != nil {
			return err
		}
		sync, err = a.Client.Sync(ctx)
	}
	if err != nil {
		return err
	}
	a.sync = sync
	if a.WowDir == "" {
		a.Log.Info("synkronisert, men WoW-mappen er ukjent")
		return nil
	}
	entries := syncdata.Merge(sync)
	addonDir := wowpath.AddonDir(a.WowDir)
	aspects, err := a.writeSagaImages(ctx, sync, addonDir)
	if err != nil {
		a.Log.Warn("kunne ikke hente saga-bilder", "err", err)
	}
	for i := range entries {
		if ar, ok := aspects[entries[i].QuestID]; ok && ar > 0.05 {
			entries[i].ImageAspect = ar
		}
	}
	body := syncdata.WriteLua(sync.BattleTag, sync.GeneratedAt, entries)
	path := filepath.Join(addonDir, "GutSagaData.lua")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		return err
	}
	a.Log.Info("oppdrag oppdatert. Skriv /reload i spillet hvis du er innlogget.", "antall", len(entries))
	return nil
}

func (a *App) writeSagaImages(ctx context.Context, sync contract.SyncResponse, addonDir string) (map[string]float64, error) {
	media := filepath.Join(addonDir, "media")
	if err := os.MkdirAll(media, 0o755); err != nil {
		return nil, err
	}
	keep := map[string]bool{}
	aspects := map[string]float64{}
	for _, q := range sync.Quests {
		if !q.HasImage {
			continue
		}
		_, raw, err := a.Client.SagaImage(ctx, q.ID)
		if err != nil {
			a.Log.Warn("saga-bilde", "quest", q.ID, "err", err)
			continue
		}
		tga, w, h, err := screenshot.ToAddonTexture(raw)
		if err != nil {
			a.Log.Warn("saga-bilde konvertering", "quest", q.ID, "err", err)
			continue
		}
		name := q.ID + ".tga"
		if err := os.WriteFile(filepath.Join(media, name), tga, 0o644); err != nil {
			return aspects, err
		}
		keep[name] = true
		if h > 0 {
			aspects[q.ID] = float64(w) / float64(h)
		}
	}
	entries, err := os.ReadDir(media)
	if err != nil {
		return aspects, nil
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !keep[e.Name()] {
			_ = os.Remove(filepath.Join(media, e.Name()))
		}
	}
	return aspects, nil
}

// HandleLine enqueues a parsed chat line. Duplicate events return false.
func (a *App) HandleLine(line string) bool {
	ev, ok := chatlog.Parse(line)
	if !ok {
		return false
	}
	switch ev.Kind {
	case chatlog.KindComplete:
		return a.Queue.Enqueue(queue.Item{
			Kind: "completion", Key: queue.CompletionKey(ev.QuestID, ev.Character, ev.Realm),
			QuestID: ev.QuestID, QuestVersion: ev.QuestVersion, Character: ev.Character, Realm: ev.Realm,
			Level: ev.Level, CompletedAt: ev.At.UTC().Format(time.RFC3339),
		})
	case chatlog.KindCharacter:
		ok := a.Queue.Enqueue(queue.Item{
			Key:  queue.CharacterKey(ev.Character, ev.Realm, ev.Level, ev.Race, ev.Class, ev.BattleTag),
			Kind: "character", Character: ev.Character, Realm: ev.Realm,
			Level: ev.Level, Race: ev.Race, Class: ev.Class, BattleTag: ev.BattleTag,
		})
		if ok {
			a.Log.Info("karakter", "character", ev.Character, "realm", ev.Realm, "level", ev.Level)
		}
		return ok
	case chatlog.KindLevel:
		return a.Queue.Enqueue(queue.Item{
			Kind: "level", Key: queue.LevelKey(ev.Character, ev.Realm, ev.Level),
			Character: ev.Character, Realm: ev.Realm, Level: ev.Level,
			CompletedAt: ev.At.UTC().Format(time.RFC3339),
		})
	default:
		return false
	}
}

func (a *App) pollChat() {
	if a.follow.path == "" && a.WowDir != "" {
		a.follow.path = wowpath.ChatLog(a.WowDir)
	}
	if a.follow.path == "" {
		return
	}
	for _, line := range a.follow.poll() {
		if a.HandleLine(line) {
			a.Log.Info("lest fra chatloggen", "line", trim(line))
		}
	}
}

// pollSavedVars enqueues completions the addon wrote locally. On Classic Era,
// AddMessage lines never reach WoWChatLog.txt, so this is the reliable path.
func (a *App) pollSavedVars() {
	if a.WowDir == "" {
		return
	}
	list, err := savedvars.ReadCompletions(a.WowDir)
	if err != nil {
		return
	}
	for _, c := range list {
		if _, ok := syncdata.QuestByID(a.sync, c.QuestID); !ok {
			continue
		}
		key := queue.CompletionKey(c.QuestID, c.Character, c.Realm)
		if !a.serverHasCompletion(c.QuestID, c.Character, c.Realm) {
			a.Queue.ForgetDone(key)
		}
		item := queue.Item{
			Kind: "completion", Key: key,
			QuestID: c.QuestID, QuestVersion: c.QuestVersion, Character: c.Character, Realm: c.Realm,
			Level: c.Level, CompletedAt: c.At.UTC().Format(time.RFC3339),
			Instance: c.Instance, GroupMembers: c.GroupMembers,
		}
		ok := a.Queue.Enqueue(item)
		if !ok && len(c.GroupMembers) > 0 && !a.Queue.SetGroup(key, c.Instance, c.GroupMembers) {
			item.Key = "group\n" + key
			ok = a.Queue.Enqueue(item)
		}
		if ok {
			a.Log.Info("lest fra SavedVariables", "character", c.Character, "quest", c.QuestID)
		}
	}
}

func (a *App) serverHasCompletion(questID, character, realm string) bool {
	for _, c := range a.sync.Completions {
		if c.QuestID != questID {
			continue
		}
		if !strings.EqualFold(c.Character, character) {
			continue
		}
		if realm != "" && c.Realm != "" && !strings.EqualFold(c.Realm, realm) {
			continue
		}
		return true
	}
	for _, p := range a.sync.Progress {
		if p.QuestID != questID || p.Status != "completed" {
			continue
		}
		ch, ok := characterByID(a.sync, p.CharacterID)
		if !ok {
			continue
		}
		if !strings.EqualFold(ch.Name, character) {
			continue
		}
		if realm != "" && ch.Realm != "" && !strings.EqualFold(ch.Realm, realm) {
			continue
		}
		return true
	}
	return false
}

func characterByID(sync contract.SyncResponse, id string) (contract.Character, bool) {
	for _, ch := range sync.Characters {
		if ch.ID == id {
			return ch, true
		}
	}
	return contract.Character{}, false
}

func (a *App) Flush(ctx context.Context) {
	now := time.Now()
	for _, item := range a.Queue.Due(now) {
		if err := a.send(ctx, item); err != nil {
			var httpErr *backend.HTTPError
			permanent := errors.As(err, &httpErr) && httpErr.Permanent()
			a.Queue.Fail(item.Key, err.Error(), permanent, now)
			a.Log.Warn("kunne ikke sende hendelse", "key", item.Kind, "err", err)
			continue
		}
		a.Queue.Succeed(item.Key)
		a.Log.Info("sendt til API", "kind", item.Kind, "character", item.Character, "quest", item.QuestID)
	}
	if err := a.Queue.Save(); err != nil {
		a.Log.Error("kunne ikke lagre køen", "err", err)
	}
}

func (a *App) send(ctx context.Context, item queue.Item) error {
	if item.Kind == "character" {
		level := item.Level
		if level < 1 {
			level = 1
		}
		_, err := a.Client.UpsertCharacter(ctx, contract.CharacterUpsert{
			Name: item.Character, Realm: item.Realm, Level: level,
			Race: item.Race, Class: item.Class, BattleTag: item.BattleTag,
		})
		if err != nil {
			return err
		}
		return a.pullSync(ctx)
	}
	if len(a.sync.Characters) == 0 {
		_ = a.pullSync(ctx)
	}
	if item.CharacterID == "" {
		ch, ok := syncdata.FindCharacter(a.sync, item.Character, item.Realm)
		if !ok {
			return errors.New("fant ikke karakteren " + item.Character + " på " + item.Realm + " i synken")
		}
		item.CharacterID = ch.ID
	}
	tag := a.sync.BattleTag
	if tag == "" {
		tag = a.Config.BattleTag
	}
	if item.Kind == "level" {
		_, err := a.Client.Level(ctx, contract.LevelRequest{
			CharacterID: item.CharacterID, BattleTag: tag, Level: item.Level,
		})
		return err
	}
	if item.QuestTitle == "" {
		if q, ok := syncdata.QuestByID(a.sync, item.QuestID); ok {
			item.QuestTitle = q.Title
		}
	}
	a.Queue.Update(item)
	_, err := a.Client.Complete(ctx, contract.CompletionRequest{
		CharacterID: item.CharacterID, QuestID: item.QuestID, QuestVersion: item.QuestVersion,
		BattleTag: tag, Level: item.Level, CompletedAt: item.CompletedAt,
		Instance: item.Instance, GroupMembers: item.GroupMembers,
	}, nil)
	if err != nil {
		return err
	}
	_ = a.pullSync(ctx)
	return nil
}

func (a *App) maybeUpdate(ctx context.Context) {
	repo := a.Config.Update.Repo
	if repo == "" || containsDev(version.Version) {
		return
	}
	client := &update.Client{Token: a.Config.Update.Token}
	rel, err := client.Latest(ctx, repo)
	if err != nil {
		a.Log.Warn("fant ikke oppdatering", "err", err)
		return
	}
	if !update.Newer(rel.Tag, version.Version) {
		return
	}
	a.Log.Info("laster ned oppdatering", "versjon", rel.Tag)
	if asset, ok := update.FindAsset(rel, update.AddonZip); ok && a.WowDir != "" {
		body, err := client.Download(ctx, asset)
		if err != nil {
			a.Log.Warn("kunne ikke laste ned tillegget", "err", err)
		} else if err := installZip(a.WowDir, body); err != nil {
			a.Log.Warn("kunne ikke pakke ut tillegget", "err", err)
		}
	}
	asset, ok := update.FindAsset(rel, update.AssetName(osName(), archName()))
	if !ok {
		return
	}
	body, err := client.Download(ctx, asset)
	if err != nil {
		a.Log.Warn("kunne ikke laste ned programmet", "err", err)
		return
	}
	if err := update.ReplaceExecutable(body); err != nil {
		a.Log.Warn("kunne ikke bytte ut programfilen", "err", err)
		return
	}
	a.Log.Info("starter GutSaga på nytt")
	restart()
}

func (f *follower) poll() []string {
	if f.path == "" {
		return nil
	}
	file, err := os.Open(f.path)
	if err != nil {
		return nil
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil
	}
	if info.Size() < f.offset {
		f.offset = 0
		f.partial = ""
	}
	if _, err := file.Seek(f.offset, io.SeekStart); err != nil {
		return nil
	}
	buf, err := io.ReadAll(file)
	if err != nil || len(buf) == 0 && f.partial == "" {
		return nil
	}
	f.offset += int64(len(buf))
	text := f.partial + string(buf)
	parts := splitLines(text)
	f.partial = parts[len(parts)-1]
	return parts[:len(parts)-1]
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			line := s[start:i]
			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			out = append(out, line)
			start = i + 1
		}
	}
	out = append(out, s[start:])
	return out
}

func trim(s string) string {
	if len(s) > 180 {
		return s[:180]
	}
	return s
}

func containsDev(v string) bool {
	return len(v) >= 3 && (v == "dev" || has(v, "dev"))
}

func has(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && contains(s, sub))
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
