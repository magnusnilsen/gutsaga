package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/magnusnilsen/gutsaga/internal/contract"
)

var (
	ErrNotFound  = errors.New("not found")
	ErrForbidden = errors.New("forbidden")
	ErrConflict  = errors.New("conflict")
	ErrInvalid   = errors.New("invalid")
	uuidPattern  = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
)

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

type querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

func newAPIKey() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "gsk_" + hex.EncodeToString(buf), nil
}

func hashKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

func validUUID(id string) error {
	if !uuidPattern.MatchString(strings.TrimSpace(id)) {
		return fmt.Errorf("%w: id", ErrInvalid)
	}
	return nil
}

func validateBattleTag(tag string) error {
	tag = strings.TrimSpace(tag)
	if tag == "" || len(tag) > 64 || strings.ContainsAny(tag, " \t") || !strings.Contains(tag, "#") {
		return fmt.Errorf("%w: battletag", ErrInvalid)
	}
	return nil
}

func validateName(name string) error {
	n := []rune(strings.TrimSpace(name))
	if len(n) < 2 || len(n) > 24 {
		return fmt.Errorf("%w: character name", ErrInvalid)
	}
	for _, r := range n {
		if r == ' ' || unicode.IsControl(r) {
			return fmt.Errorf("%w: character name", ErrInvalid)
		}
	}
	return nil
}

func validateRealm(realm string) error {
	n := []rune(strings.TrimSpace(realm))
	if len(n) < 1 || len(n) > 64 {
		return fmt.Errorf("%w: realm", ErrInvalid)
	}
	for _, r := range n {
		if unicode.IsControl(r) {
			return fmt.Errorf("%w: realm", ErrInvalid)
		}
	}
	return nil
}

func validateTitle(title string) error {
	n := []rune(strings.TrimSpace(title))
	if len(n) < 1 || len(n) > 200 {
		return fmt.Errorf("%w: title", ErrInvalid)
	}
	return nil
}

func clip(s string, max int) error {
	if len([]rune(s)) > max {
		return fmt.Errorf("%w: text too long", ErrInvalid)
	}
	return nil
}

func nullableInt(v *int) any {
	if v == nil {
		return nil
	}
	return *v
}

func nullInt(n sql.NullInt32) *int {
	if !n.Valid {
		return nil
	}
	v := int(n.Int32)
	return &v
}

func mapPG(err error) error {
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == "23505" {
		return fmt.Errorf("%w", ErrConflict)
	}
	return err
}

func (s *Store) CreatePlayer(ctx context.Context, battletag, displayName string) (contract.CreatePlayerResponse, error) {
	battletag = strings.TrimSpace(battletag)
	displayName = strings.TrimSpace(displayName)
	if err := validateBattleTag(battletag); err != nil {
		return contract.CreatePlayerResponse{}, err
	}
	if displayName == "" || len([]rune(displayName)) > 80 {
		return contract.CreatePlayerResponse{}, fmt.Errorf("%w: display name", ErrInvalid)
	}
	key, err := newAPIKey()
	if err != nil {
		return contract.CreatePlayerResponse{}, err
	}
	var id string
	var created time.Time
	err = s.pool.QueryRow(ctx, `
		INSERT INTO players (battletag, display_name, api_key_hash)
		VALUES ($1, $2, $3)
		RETURNING id::text, created_at`, battletag, displayName, hashKey(key)).Scan(&id, &created)
	if err != nil {
		return contract.CreatePlayerResponse{}, mapPG(err)
	}
	return contract.CreatePlayerResponse{
		APIKey: key,
		Player: contract.Player{ID: id, BattleTag: battletag, DisplayName: displayName, CreatedAt: contract.FormatTime(created)},
	}, nil
}

func (s *Store) RotateKey(ctx context.Context, playerID string) (contract.CreatePlayerResponse, error) {
	if err := validUUID(playerID); err != nil {
		return contract.CreatePlayerResponse{}, err
	}
	key, err := newAPIKey()
	if err != nil {
		return contract.CreatePlayerResponse{}, err
	}
	var tag, name string
	var created time.Time
	err = s.pool.QueryRow(ctx, `
		UPDATE players SET api_key_hash=$2 WHERE id=$1
		RETURNING COALESCE(battletag, ''), display_name, created_at`, playerID, hashKey(key)).Scan(&tag, &name, &created)
	if errors.Is(err, pgx.ErrNoRows) {
		return contract.CreatePlayerResponse{}, ErrNotFound
	}
	if err != nil {
		return contract.CreatePlayerResponse{}, err
	}
	return contract.CreatePlayerResponse{
		APIKey: key,
		Player: contract.Player{ID: playerID, BattleTag: tag, DisplayName: name, CreatedAt: contract.FormatTime(created)},
	}, nil
}

func newSessionToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "gss_" + hex.EncodeToString(buf), nil
}

func (s *Store) PlayerByKey(ctx context.Context, key string) (contract.Player, error) {
	switch {
	case strings.HasPrefix(key, "gss_"):
		return s.playerBySession(ctx, key)
	case strings.HasPrefix(key, "gsk_"):
		return s.playerByAPIKey(ctx, key)
	default:
		return contract.Player{}, ErrForbidden
	}
}

func (s *Store) playerByAPIKey(ctx context.Context, key string) (contract.Player, error) {
	var p contract.Player
	var created time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT id::text, COALESCE(battletag, ''), display_name, COALESCE(email, ''), COALESCE(smia_subject, ''), created_at
		FROM players WHERE api_key_hash=$1`, hashKey(key)).Scan(&p.ID, &p.BattleTag, &p.DisplayName, &p.Email, &p.SmiaSubject, &created)
	if errors.Is(err, pgx.ErrNoRows) {
		return contract.Player{}, ErrForbidden
	}
	if err != nil {
		return contract.Player{}, err
	}
	p.CreatedAt = contract.FormatTime(created)
	return p, nil
}

func (s *Store) playerBySession(ctx context.Context, key string) (contract.Player, error) {
	var p contract.Player
	var created time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT p.id::text, COALESCE(p.battletag, ''), p.display_name, COALESCE(p.email, ''), COALESCE(p.smia_subject, ''), p.created_at
		FROM sessions s
		JOIN players p ON p.id = s.player_id
		WHERE s.token_hash=$1 AND s.expires_at > now()`, hashKey(key)).Scan(&p.ID, &p.BattleTag, &p.DisplayName, &p.Email, &p.SmiaSubject, &created)
	if errors.Is(err, pgx.ErrNoRows) {
		return contract.Player{}, ErrForbidden
	}
	if err != nil {
		return contract.Player{}, err
	}
	p.CreatedAt = contract.FormatTime(created)
	return p, nil
}

func (s *Store) ListPlayers(ctx context.Context) ([]contract.Player, error) {
	rows, err := s.pool.Query(ctx, `SELECT id::text, COALESCE(battletag, ''), display_name, created_at FROM players ORDER BY display_name, battletag`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []contract.Player
	for rows.Next() {
		var p contract.Player
		var created time.Time
		if err := rows.Scan(&p.ID, &p.BattleTag, &p.DisplayName, &created); err != nil {
			return nil, err
		}
		p.CreatedAt = contract.FormatTime(created)
		out = append(out, p)
	}
	return out, rows.Err()
}

type CharacterInput struct {
	PlayerID  string
	BattleTag string
	Name      string
	Realm     string
	Race      string
	Class     string
	Level     int
	Backstory string
}

func (s *Store) resolvePlayerID(ctx context.Context, q querier, playerID, battletag string) (string, error) {
	if playerID != "" {
		if err := validUUID(playerID); err != nil {
			return "", err
		}
		var one int
		err := q.QueryRow(ctx, `SELECT 1 FROM players WHERE id=$1`, playerID).Scan(&one)
		if errors.Is(err, pgx.ErrNoRows) {
			return "", fmt.Errorf("%w: player", ErrNotFound)
		}
		return playerID, err
	}
	battletag = strings.TrimSpace(battletag)
	if err := validateBattleTag(battletag); err != nil {
		return "", err
	}
	var id string
	err := q.QueryRow(ctx, `SELECT id::text FROM players WHERE battletag=$1`, battletag).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("%w: player", ErrNotFound)
	}
	return id, err
}

func (s *Store) CreateCharacter(ctx context.Context, in CharacterInput) (contract.Character, error) {
	if err := validateName(in.Name); err != nil {
		return contract.Character{}, err
	}
	if err := validateRealm(in.Realm); err != nil {
		return contract.Character{}, err
	}
	if err := clip(in.Backstory, 8000); err != nil || clip(in.Race, 40) != nil || clip(in.Class, 40) != nil {
		return contract.Character{}, fmt.Errorf("%w: character text", ErrInvalid)
	}
	if in.Level == 0 {
		in.Level = 1
	}
	if in.Level < 1 || in.Level > 1000 {
		return contract.Character{}, fmt.Errorf("%w: level", ErrInvalid)
	}
	playerID, err := s.resolvePlayerID(ctx, s.pool, in.PlayerID, in.BattleTag)
	if err != nil {
		return contract.Character{}, err
	}
	var id string
	err = s.pool.QueryRow(ctx, `
		INSERT INTO characters (player_id, name, realm, race, class, level, backstory)
		VALUES ($1,$2,$3,$4,$5,$6,$7)
		RETURNING id::text`,
		playerID, strings.TrimSpace(in.Name), strings.TrimSpace(in.Realm), strings.TrimSpace(in.Race), strings.TrimSpace(in.Class), in.Level, in.Backstory,
	).Scan(&id)
	if err != nil {
		return contract.Character{}, mapPG(err)
	}
	return s.GetCharacter(ctx, id)
}

type CharacterPatch struct {
	Name      *string
	Realm     *string
	Race      *string
	Class     *string
	Level     *int
	Backstory *string
}

func (s *Store) UpdateCharacter(ctx context.Context, id string, p CharacterPatch) (contract.Character, error) {
	cur, err := s.GetCharacter(ctx, id)
	if err != nil {
		return contract.Character{}, err
	}
	if p.Name != nil {
		cur.Name = strings.TrimSpace(*p.Name)
	}
	if p.Realm != nil {
		cur.Realm = strings.TrimSpace(*p.Realm)
	}
	if p.Race != nil {
		cur.Race = strings.TrimSpace(*p.Race)
	}
	if p.Class != nil {
		cur.Class = strings.TrimSpace(*p.Class)
	}
	if p.Backstory != nil {
		cur.Backstory = *p.Backstory
	}
	if p.Level != nil {
		cur.Level = *p.Level
	}
	if err := validateName(cur.Name); err != nil {
		return contract.Character{}, err
	}
	if err := validateRealm(cur.Realm); err != nil {
		return contract.Character{}, err
	}
	if cur.Level < 1 || cur.Level > 1000 {
		return contract.Character{}, fmt.Errorf("%w: level", ErrInvalid)
	}
	if err := clip(cur.Backstory, 8000); err != nil {
		return contract.Character{}, err
	}
	tag, err := s.pool.Exec(ctx, `
		UPDATE characters
		SET name=$2, realm=$3, race=$4, class=$5, level=$6, backstory=$7, updated_at=now()
		WHERE id=$1`, id, cur.Name, cur.Realm, cur.Race, cur.Class, cur.Level, cur.Backstory)
	if err != nil {
		return contract.Character{}, mapPG(err)
	}
	if tag.RowsAffected() == 0 {
		return contract.Character{}, ErrNotFound
	}
	return s.GetCharacter(ctx, id)
}

func (s *Store) DeleteCharacter(ctx context.Context, id string) error {
	if err := validUUID(id); err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx, `DELETE FROM characters WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) GetCharacter(ctx context.Context, id string) (contract.Character, error) {
	if err := validUUID(id); err != nil {
		return contract.Character{}, err
	}
	rows, err := s.pool.Query(ctx, characterSelect+` WHERE c.id=$1`, id)
	if err != nil {
		return contract.Character{}, err
	}
	defer rows.Close()
	list, err := scanCharacters(rows)
	if err != nil {
		return contract.Character{}, err
	}
	if len(list) == 0 {
		return contract.Character{}, ErrNotFound
	}
	return list[0], nil
}

func (s *Store) ListCharacters(ctx context.Context, battletag string) ([]contract.Character, error) {
	q := characterSelect
	var args []any
	if strings.TrimSpace(battletag) != "" {
		q += ` WHERE p.battletag=$1`
		args = append(args, strings.TrimSpace(battletag))
	}
	q += ` ORDER BY c.name, c.realm`
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCharacters(rows)
}

const characterSelect = `
	SELECT c.id::text, c.player_id::text, c.name, c.realm, c.level, c.race, c.class, COALESCE(p.battletag, ''), c.backstory
	FROM characters c JOIN players p ON p.id = c.player_id`

func scanCharacters(rows pgx.Rows) ([]contract.Character, error) {
	var out []contract.Character
	for rows.Next() {
		var c contract.Character
		if err := rows.Scan(&c.ID, &c.PlayerID, &c.Name, &c.Realm, &c.Level, &c.Race, &c.Class, &c.BattleTag, &c.Backstory); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

type QuestInput struct {
	Title                string
	Description          string
	Objectives           []contract.Objective
	Reward               string
	MinLevel             *int
	PartyQuest           bool
	Active               *bool
	CreatedBy            string
	AssignedCharacterIDs []string
}

type QuestPatch struct {
	Title                *string
	Description          *string
	Objectives           *[]contract.Objective
	Reward               *string
	MinLevel             *int
	MinLevelSet          bool
	PartyQuest           *bool
	Active               *bool
	CreatedBy            *string
	AssignedCharacterIDs *[]string
}

func resolveCreator(ctx context.Context, q querier, createdBy string) (tag, playerID string, err error) {
	createdBy = strings.TrimSpace(createdBy)
	if err := validateBattleTag(createdBy); err != nil {
		return "", "", fmt.Errorf("%w: created_by must be a BattleTag", ErrInvalid)
	}
	err = q.QueryRow(ctx, `
		SELECT id::text, battletag FROM players WHERE battletag=$1`, createdBy).Scan(&playerID, &tag)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", fmt.Errorf("%w: created_by player not found", ErrInvalid)
	}
	if err != nil {
		return "", "", err
	}
	return tag, playerID, nil
}

func (s *Store) EnsurePlayer(ctx context.Context, battletag, displayName string) (contract.Player, error) {
	battletag = strings.TrimSpace(battletag)
	displayName = strings.TrimSpace(displayName)
	if err := validateBattleTag(battletag); err != nil {
		return contract.Player{}, err
	}
	if displayName == "" {
		displayName = battletag
	}
	var p contract.Player
	var created time.Time
	err := s.pool.QueryRow(ctx, `
		SELECT id::text, COALESCE(battletag, ''), display_name, created_at
		FROM players WHERE battletag=$1`, battletag).Scan(&p.ID, &p.BattleTag, &p.DisplayName, &created)
	if err == nil {
		p.CreatedAt = contract.FormatTime(created)
		return p, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return contract.Player{}, err
	}
	out, err := s.CreatePlayer(ctx, battletag, displayName)
	if err != nil {
		return contract.Player{}, err
	}
	return out.Player, nil
}

func (s *Store) CreateQuest(ctx context.Context, in QuestInput) (contract.Quest, error) {
	objectives, err := contract.NormalizeObjectives(in.Objectives, false)
	if err != nil {
		return contract.Quest{}, fmt.Errorf("%w: %s", ErrInvalid, err.Error())
	}
	in.Objectives = objectives
	if err := validateQuestText(in.Title, in.Description, in.Reward, in.CreatedBy); err != nil {
		return contract.Quest{}, err
	}
	if in.MinLevel != nil && (*in.MinLevel < 1 || *in.MinLevel > 1000) {
		return contract.Quest{}, fmt.Errorf("%w: min_level", ErrInvalid)
	}
	active := true
	if in.Active != nil {
		active = *in.Active
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return contract.Quest{}, err
	}
	defer tx.Rollback(ctx)
	creatorTag, creatorID, err := resolveCreator(ctx, tx, in.CreatedBy)
	if err != nil {
		return contract.Quest{}, err
	}
	if err := ensureCharacters(ctx, tx, in.AssignedCharacterIDs); err != nil {
		return contract.Quest{}, err
	}
	objJSON, err := objectivesJSON(in.Objectives)
	if err != nil {
		return contract.Quest{}, err
	}
	var id string
	err = tx.QueryRow(ctx, `
		INSERT INTO quests (title, description, objectives, reward, min_level, party, active, version, created_by, created_by_player_id)
		VALUES ($1,$2,$3::jsonb,$4,$5,$6,$7,1,$8,$9)
		RETURNING id::text`,
		strings.TrimSpace(in.Title), in.Description, objJSON, in.Reward, nullableInt(in.MinLevel), in.PartyQuest, active, creatorTag, creatorID,
	).Scan(&id)
	if err != nil {
		return contract.Quest{}, err
	}
	q := contract.Quest{
		ID: id, Title: strings.TrimSpace(in.Title), Description: in.Description, Objectives: in.Objectives,
		Reward: in.Reward, MinLevel: in.MinLevel, PartyQuest: in.PartyQuest, Active: active, Version: 1,
		CreatedBy: creatorTag, AssignedTo: refs(in.AssignedCharacterIDs),
	}
	if err := replaceAssignments(ctx, tx, id, in.AssignedCharacterIDs); err != nil {
		return contract.Quest{}, err
	}
	if err := insertSnapshot(ctx, tx, q); err != nil {
		return contract.Quest{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return contract.Quest{}, err
	}
	return q, nil
}

func (s *Store) UpdateQuest(ctx context.Context, id string, p QuestPatch) (contract.Quest, error) {
	if err := validUUID(id); err != nil {
		return contract.Quest{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return contract.Quest{}, err
	}
	defer tx.Rollback(ctx)
	q, err := loadQuest(ctx, tx, id)
	if err != nil {
		return contract.Quest{}, err
	}
	if p.Title != nil {
		q.Title = strings.TrimSpace(*p.Title)
	}
	if p.Description != nil {
		q.Description = *p.Description
	}
	if p.Objectives != nil {
		objectives, err := contract.NormalizeObjectives(*p.Objectives, false)
		if err != nil {
			return contract.Quest{}, fmt.Errorf("%w: %s", ErrInvalid, err.Error())
		}
		q.Objectives = objectives
	}
	if p.Reward != nil {
		q.Reward = *p.Reward
	}
	var creatorID any
	if p.CreatedBy != nil {
		tag, pid, err := resolveCreator(ctx, tx, *p.CreatedBy)
		if err != nil {
			return contract.Quest{}, err
		}
		q.CreatedBy = tag
		creatorID = pid
	} else {
		var existing sql.NullString
		if err := tx.QueryRow(ctx, `SELECT created_by_player_id::text FROM quests WHERE id=$1`, id).Scan(&existing); err != nil {
			return contract.Quest{}, err
		}
		if existing.Valid {
			creatorID = existing.String
		}
	}
	if p.PartyQuest != nil {
		q.PartyQuest = *p.PartyQuest
	}
	if p.Active != nil {
		q.Active = *p.Active
	}
	if p.MinLevelSet {
		q.MinLevel = p.MinLevel
	}
	if p.AssignedCharacterIDs != nil {
		if err := ensureCharacters(ctx, tx, *p.AssignedCharacterIDs); err != nil {
			return contract.Quest{}, err
		}
		q.AssignedTo = refs(*p.AssignedCharacterIDs)
	}
	if err := validateQuestText(q.Title, q.Description, q.Reward, q.CreatedBy); err != nil {
		return contract.Quest{}, err
	}
	if q.MinLevel != nil && (*q.MinLevel < 1 || *q.MinLevel > 1000) {
		return contract.Quest{}, fmt.Errorf("%w: min_level", ErrInvalid)
	}
	q.Version++
	objJSON, err := objectivesJSON(q.Objectives)
	if err != nil {
		return contract.Quest{}, err
	}
	_, err = tx.Exec(ctx, `
		UPDATE quests
		SET title=$2, description=$3, objectives=$4::jsonb, reward=$5, min_level=$6, party=$7, active=$8, version=$9, created_by=$10, created_by_player_id=$11, updated_at=now()
		WHERE id=$1`,
		id, q.Title, q.Description, objJSON, q.Reward, nullableInt(q.MinLevel), q.PartyQuest, q.Active, q.Version, q.CreatedBy, creatorID)
	if err != nil {
		return contract.Quest{}, err
	}
	if p.AssignedCharacterIDs != nil {
		if err := replaceAssignments(ctx, tx, id, *p.AssignedCharacterIDs); err != nil {
			return contract.Quest{}, err
		}
	}
	if err := insertSnapshot(ctx, tx, q); err != nil {
		return contract.Quest{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return contract.Quest{}, err
	}
	return q, nil
}

func objectivesJSON(list []contract.Objective) (string, error) {
	if list == nil {
		list = []contract.Objective{}
	}
	raw, err := json.Marshal(list)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func validateQuestText(title, description, reward, createdBy string) error {
	if err := validateTitle(title); err != nil {
		return err
	}
	if err := clip(description, 8000); err != nil {
		return err
	}
	if err := clip(reward, 2000); err != nil {
		return err
	}
	if err := clip(createdBy, 200); err != nil {
		return err
	}
	return nil
}

func refs(ids []string) []contract.IDRef {
	out := make([]contract.IDRef, 0, len(ids))
	for _, id := range ids {
		out = append(out, contract.IDRef{ID: id})
	}
	return out
}

func ensureCharacters(ctx context.Context, q querier, ids []string) error {
	for _, id := range ids {
		if err := validUUID(id); err != nil {
			return err
		}
	}
	if len(ids) == 0 {
		return nil
	}
	var n int
	err := q.QueryRow(ctx, `SELECT count(*) FROM characters WHERE id::text = ANY($1::text[])`, ids).Scan(&n)
	if err != nil {
		return err
	}
	if n != len(unique(ids)) {
		return fmt.Errorf("%w: unknown character", ErrInvalid)
	}
	return nil
}

func unique(ids []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}

func replaceAssignments(ctx context.Context, q querier, questID string, ids []string) error {
	if _, err := q.Exec(ctx, `DELETE FROM quest_assignments WHERE quest_id=$1`, questID); err != nil {
		return err
	}
	for _, id := range unique(ids) {
		if _, err := q.Exec(ctx, `INSERT INTO quest_assignments (quest_id, character_id) VALUES ($1,$2)`, questID, id); err != nil {
			return err
		}
	}
	return nil
}

func insertSnapshot(ctx context.Context, q querier, quest contract.Quest) error {
	ids := make([]string, 0, len(quest.AssignedTo))
	for _, a := range quest.AssignedTo {
		ids = append(ids, a.ID)
	}
	raw, err := json.Marshal(ids)
	if err != nil {
		return err
	}
	objJSON, err := objectivesJSON(quest.Objectives)
	if err != nil {
		return err
	}
	_, err = q.Exec(ctx, `
		INSERT INTO quest_versions (quest_id, version, title, description, objectives, reward, min_level, party, active, assigned_to, created_by)
		VALUES ($1,$2,$3,$4,$5::jsonb,$6,$7,$8,$9,$10::jsonb,$11)`,
		quest.ID, quest.Version, quest.Title, quest.Description, objJSON, quest.Reward,
		nullableInt(quest.MinLevel), quest.PartyQuest, quest.Active, string(raw), quest.CreatedBy)
	return err
}

func scanQuest(scan func(dest ...any) error) (contract.Quest, error) {
	var q contract.Quest
	var ml sql.NullInt32
	var obj string
	err := scan(&q.ID, &q.Title, &q.Description, &obj, &q.Reward, &ml, &q.PartyQuest, &q.Active, &q.Version, &q.CreatedBy)
	if err != nil {
		return q, err
	}
	q.MinLevel = nullInt(ml)
	q.Objectives, err = contract.DecodeObjectives(obj)
	if err != nil {
		return q, fmt.Errorf("%w: objectives", ErrInvalid)
	}
	q.AssignedTo = []contract.IDRef{}
	return q, nil
}

const questSelect = `
		SELECT q.id::text, q.title, q.description, q.objectives::text, q.reward, q.min_level, q.party, q.active, q.version,
			COALESCE(NULLIF(p.battletag, ''), q.created_by)
		FROM quests q
		LEFT JOIN players p ON p.id = q.created_by_player_id`

func loadQuest(ctx context.Context, q querier, id string) (contract.Quest, error) {
	quest, err := scanQuest(q.QueryRow(ctx, questSelect+` WHERE q.id=$1`, id).Scan)
	if errors.Is(err, pgx.ErrNoRows) {
		return contract.Quest{}, ErrNotFound
	}
	if err != nil {
		return contract.Quest{}, err
	}
	assigned, err := loadAssignments(ctx, q, []string{id})
	if err != nil {
		return contract.Quest{}, err
	}
	quest.AssignedTo = assigned[id]
	if quest.AssignedTo == nil {
		quest.AssignedTo = []contract.IDRef{}
	}
	return quest, nil
}

func loadAssignments(ctx context.Context, q querier, questIDs []string) (map[string][]contract.IDRef, error) {
	out := map[string][]contract.IDRef{}
	if len(questIDs) == 0 {
		return out, nil
	}
	rows, err := q.Query(ctx, `SELECT quest_id::text, character_id::text FROM quest_assignments WHERE quest_id::text = ANY($1::text[]) ORDER BY character_id::text`, questIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var qid, cid string
		if err := rows.Scan(&qid, &cid); err != nil {
			return nil, err
		}
		out[qid] = append(out[qid], contract.IDRef{ID: cid})
	}
	return out, rows.Err()
}

func (s *Store) GetQuest(ctx context.Context, id string) (contract.QuestDetail, error) {
	if err := validUUID(id); err != nil {
		return contract.QuestDetail{}, err
	}
	q, err := loadQuest(ctx, s.pool, id)
	if err != nil {
		return contract.QuestDetail{}, err
	}
	rows, err := s.pool.Query(ctx, `
		SELECT version, title, description, objectives::text, reward, min_level, party, active, assigned_to::text, created_by, created_at
		FROM quest_versions WHERE quest_id=$1 ORDER BY version`, id)
	if err != nil {
		return contract.QuestDetail{}, err
	}
	defer rows.Close()
	var versions []contract.QuestVersion
	for rows.Next() {
		var v contract.QuestVersion
		var ml sql.NullInt32
		var assigned, obj string
		var created time.Time
		if err := rows.Scan(&v.Version, &v.Title, &v.Description, &obj, &v.Reward, &ml, &v.PartyQuest, &v.Active, &assigned, &v.CreatedBy, &created); err != nil {
			return contract.QuestDetail{}, err
		}
		v.Objectives, err = contract.DecodeObjectives(obj)
		if err != nil {
			return contract.QuestDetail{}, err
		}
		v.MinLevel = nullInt(ml)
		v.CreatedAt = contract.FormatTime(created)
		var ids []string
		_ = json.Unmarshal([]byte(assigned), &ids)
		v.AssignedTo = refs(ids)
		versions = append(versions, v)
	}
	if err := rows.Err(); err != nil {
		return contract.QuestDetail{}, err
	}
	return contract.QuestDetail{Quest: q, Versions: versions}, nil
}

func (s *Store) ListQuests(ctx context.Context, characterID string) ([]contract.QuestListItem, error) {
	var level int
	var charKnown bool
	if characterID != "" {
		if err := validUUID(characterID); err != nil {
			return nil, err
		}
		err := s.pool.QueryRow(ctx, `SELECT level FROM characters WHERE id=$1`, characterID).Scan(&level)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		if err != nil {
			return nil, err
		}
		charKnown = true
	}
	rows, err := s.pool.Query(ctx, questSelect+` ORDER BY q.active DESC, q.title`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var quests []contract.Quest
	var ids []string
	for rows.Next() {
		q, err := scanQuest(rows.Scan)
		if err != nil {
			return nil, err
		}
		quests = append(quests, q)
		ids = append(ids, q.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	assigned, err := loadAssignments(ctx, s.pool, ids)
	if err != nil {
		return nil, err
	}
	progress := map[string]contract.Progress{}
	if charKnown {
		prows, err := s.pool.Query(ctx, `
			SELECT quest_id::text, quest_version, status, completed_at
			FROM progress WHERE character_id=$1`, characterID)
		if err != nil {
			return nil, err
		}
		defer prows.Close()
		for prows.Next() {
			var p contract.Progress
			var completed sql.NullTime
			if err := prows.Scan(&p.QuestID, &p.QuestVersion, &p.Status, &completed); err != nil {
				return nil, err
			}
			if completed.Valid {
				p.CompletedAt = contract.FormatTime(completed.Time)
			}
			progress[p.QuestID] = p
		}
		if err := prows.Err(); err != nil {
			return nil, err
		}
	}
	out := make([]contract.QuestListItem, 0, len(quests))
	for _, q := range quests {
		q.AssignedTo = assigned[q.ID]
		if q.AssignedTo == nil {
			q.AssignedTo = []contract.IDRef{}
		}
		item := contract.QuestListItem{Quest: q}
		if charKnown {
			ok := contract.Eligible(level, characterID, q)
			item.Eligible = &ok
			if p, found := progress[q.ID]; found {
				status := p.Status
				item.Status = &status
				item.CompletedAt = p.CompletedAt
				item.CompletedVersion = p.QuestVersion
			}
		}
		out = append(out, item)
	}
	return out, nil
}

func (s *Store) Sync(ctx context.Context, playerID string) (contract.SyncResponse, error) {
	var tag string
	if err := s.pool.QueryRow(ctx, `SELECT COALESCE(battletag, '') FROM players WHERE id=$1`, playerID).Scan(&tag); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return contract.SyncResponse{}, ErrNotFound
		}
		return contract.SyncResponse{}, err
	}
	chars, err := scanCharactersMust(ctx, s.pool, characterSelect+` WHERE c.player_id=$1 ORDER BY c.name, c.realm`, playerID)
	if err != nil {
		return contract.SyncResponse{}, err
	}
	if chars == nil {
		chars = []contract.Character{}
	}
	rows, err := s.pool.Query(ctx, questSelect+`
		WHERE q.active OR q.id IN (
			SELECT p.quest_id FROM progress p
			JOIN characters c ON c.id = p.character_id
			WHERE c.player_id=$1
		)
		ORDER BY q.min_level NULLS FIRST, q.title`, playerID)
	if err != nil {
		return contract.SyncResponse{}, err
	}
	defer rows.Close()
	var quests []contract.Quest
	var ids []string
	for rows.Next() {
		q, err := scanQuest(rows.Scan)
		if err != nil {
			return contract.SyncResponse{}, err
		}
		quests = append(quests, q)
		ids = append(ids, q.ID)
	}
	if err := rows.Err(); err != nil {
		return contract.SyncResponse{}, err
	}
	if quests == nil {
		quests = []contract.Quest{}
	}
	assigned, err := loadAssignments(ctx, s.pool, ids)
	if err != nil {
		return contract.SyncResponse{}, err
	}
	for i := range quests {
		quests[i].AssignedTo = assigned[quests[i].ID]
		if quests[i].AssignedTo == nil {
			quests[i].AssignedTo = []contract.IDRef{}
		}
	}
	prows, err := s.pool.Query(ctx, `
		SELECT p.quest_id::text, p.character_id::text, p.quest_version, p.status, p.completed_at, p.discord_url
		FROM progress p
		JOIN characters c ON c.id = p.character_id
		WHERE c.player_id=$1`, playerID)
	if err != nil {
		return contract.SyncResponse{}, err
	}
	defer prows.Close()
	var progress []contract.Progress
	for prows.Next() {
		var p contract.Progress
		var completed sql.NullTime
		var discordURL string
		if err := prows.Scan(&p.QuestID, &p.CharacterID, &p.QuestVersion, &p.Status, &completed, &discordURL); err != nil {
			return contract.SyncResponse{}, err
		}
		if completed.Valid {
			p.CompletedAt = contract.FormatTime(completed.Time)
		}
		if discordURL != "" {
			p.ScreenshotURL = discordURL
		}
		progress = append(progress, p)
	}
	if progress == nil {
		progress = []contract.Progress{}
	}
	if err := prows.Err(); err != nil {
		return contract.SyncResponse{}, err
	}
	completions, err := s.sagaCompletions(ctx, ids)
	if err != nil {
		return contract.SyncResponse{}, err
	}
	return contract.SyncResponse{
		BattleTag: tag, GeneratedAt: contract.FormatTime(time.Now()),
		Characters: chars, Quests: quests, Progress: progress, Completions: completions,
	}, nil
}

func (s *Store) sagaCompletions(ctx context.Context, questIDs []string) ([]contract.QuestCompletion, error) {
	want := map[string]bool{}
	for _, id := range questIDs {
		want[id] = true
	}
	if len(want) == 0 {
		return []contract.QuestCompletion{}, nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT p.quest_id::text, c.name, COALESCE(c.realm, ''), p.completed_at
		FROM progress p
		JOIN characters c ON c.id = p.character_id
		WHERE p.status = 'completed'
		ORDER BY c.name, c.realm`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []contract.QuestCompletion{}
	for rows.Next() {
		var c contract.QuestCompletion
		var completed sql.NullTime
		if err := rows.Scan(&c.QuestID, &c.Character, &c.Realm, &completed); err != nil {
			return nil, err
		}
		if !want[c.QuestID] || strings.TrimSpace(c.Character) == "" {
			continue
		}
		if completed.Valid {
			c.CompletedAt = contract.FormatTime(completed.Time)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func scanCharactersMust(ctx context.Context, q querier, sql string, args ...any) ([]contract.Character, error) {
	rows, err := q.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCharacters(rows)
}

func (s *Store) Complete(ctx context.Context, playerID string, in contract.CompletionRequest, image []byte, contentType string) (contract.CompletionResponse, error) {
	if err := validUUID(in.CharacterID); err != nil {
		return contract.CompletionResponse{}, err
	}
	if err := validUUID(in.QuestID); err != nil {
		return contract.CompletionResponse{}, err
	}
	if in.QuestVersion < 1 {
		return contract.CompletionResponse{}, fmt.Errorf("%w: quest_version", ErrInvalid)
	}
	in.BattleTag = strings.TrimSpace(in.BattleTag)
	if in.BattleTag != "" {
		if err := validateBattleTag(in.BattleTag); err != nil {
			return contract.CompletionResponse{}, err
		}
	}
	if in.Level < 0 || in.Level > 1000 {
		return contract.CompletionResponse{}, fmt.Errorf("%w: level", ErrInvalid)
	}
	completedAt := time.Now().UTC()
	if strings.TrimSpace(in.CompletedAt) != "" {
		t, err := time.Parse(time.RFC3339, in.CompletedAt)
		if err != nil {
			return contract.CompletionResponse{}, fmt.Errorf("%w: completed_at", ErrInvalid)
		}
		completedAt = t.UTC()
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return contract.CompletionResponse{}, err
	}
	defer tx.Rollback(ctx)

	var name, tag string
	err = tx.QueryRow(ctx, `
		SELECT c.name, COALESCE(p.battletag, '')
		FROM characters c JOIN players p ON p.id = c.player_id
		WHERE c.id=$1 AND c.player_id=$2`, in.CharacterID, playerID).Scan(&name, &tag)
	if errors.Is(err, pgx.ErrNoRows) {
		return contract.CompletionResponse{Allowed: false}, ErrForbidden
	}
	if err != nil {
		return contract.CompletionResponse{}, err
	}
	if tag != "" && !strings.EqualFold(tag, in.BattleTag) {
		return contract.CompletionResponse{Allowed: false}, ErrForbidden
	}
	if tag == "" && in.BattleTag != "" {
		if _, err := tx.Exec(ctx, `UPDATE players SET battletag=$2 WHERE id=$1 AND (battletag IS NULL OR battletag='')`, playerID, in.BattleTag); err != nil {
			return contract.CompletionResponse{}, mapPG(err)
		}
	}
	var snapTitle string
	err = tx.QueryRow(ctx, `SELECT title FROM quest_versions WHERE quest_id=$1 AND version=$2`, in.QuestID, in.QuestVersion).Scan(&snapTitle)
	if errors.Is(err, pgx.ErrNoRows) {
		return contract.CompletionResponse{}, fmt.Errorf("%w: unknown quest version", ErrInvalid)
	}
	if err != nil {
		return contract.CompletionResponse{}, err
	}

	var existingID string
	var existingVersion int
	err = tx.QueryRow(ctx, `SELECT id::text, quest_version FROM progress WHERE character_id=$1 AND quest_id=$2`, in.CharacterID, in.QuestID).Scan(&existingID, &existingVersion)
	if err == nil {
		var storedTitle string
		_ = tx.QueryRow(ctx, `SELECT title FROM quest_versions WHERE quest_id=$1 AND version=$2`, in.QuestID, existingVersion).Scan(&storedTitle)
		if err := tx.Commit(ctx); err != nil {
			return contract.CompletionResponse{}, err
		}
		return contract.CompletionResponse{
			Allowed: true, AlreadyCompleted: true, Completed: true,
			ProgressID: existingID, CharacterName: name, QuestTitle: storedTitle, QuestVersion: existingVersion,
		}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return contract.CompletionResponse{}, err
	}

	var screenshot any
	if len(image) > 0 {
		var sid string
		if err := tx.QueryRow(ctx, `INSERT INTO screenshots (content_type, bytes) VALUES ($1,$2) RETURNING id::text`, contentType, image).Scan(&sid); err != nil {
			return contract.CompletionResponse{}, err
		}
		screenshot = sid
	}
	var level any
	if in.Level > 0 {
		level = in.Level
	}
	var progressID string
	err = tx.QueryRow(ctx, `
		INSERT INTO progress (character_id, quest_id, quest_version, status, completed_at, level_at_completion, screenshot_id)
		VALUES ($1,$2,$3,'completed',$4,$5,$6)
		ON CONFLICT (character_id, quest_id) DO NOTHING
		RETURNING id::text`,
		in.CharacterID, in.QuestID, in.QuestVersion, completedAt, level, screenshot).Scan(&progressID)
	if errors.Is(err, pgx.ErrNoRows) {
		if screenshot != nil {
			_, _ = tx.Exec(ctx, `DELETE FROM screenshots WHERE id=$1`, screenshot)
		}
		_ = tx.QueryRow(ctx, `SELECT id::text, quest_version FROM progress WHERE character_id=$1 AND quest_id=$2`, in.CharacterID, in.QuestID).Scan(&existingID, &existingVersion)
		if err := tx.Commit(ctx); err != nil {
			return contract.CompletionResponse{}, err
		}
		return contract.CompletionResponse{
			Allowed: true, AlreadyCompleted: true, Completed: true,
			ProgressID: existingID, CharacterName: name, QuestTitle: snapTitle, QuestVersion: existingVersion,
		}, nil
	}
	if err != nil {
		return contract.CompletionResponse{}, err
	}
	body := fmt.Sprintf("%s fullførte «%s» (versjon %d).", name, snapTitle, in.QuestVersion)
	if _, err := tx.Exec(ctx, `
		INSERT INTO adventure_log (player_id, character_id, quest_id, type, title, body)
		VALUES ($1,$2,$3,'quest',$4,$5)`, playerID, in.CharacterID, in.QuestID, "Oppdrag fullført", body); err != nil {
		return contract.CompletionResponse{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return contract.CompletionResponse{}, err
	}
	return contract.CompletionResponse{
		Allowed: true, Completed: true, LogWritten: true, ProgressID: progressID,
		CharacterName: name, QuestTitle: snapTitle, QuestVersion: in.QuestVersion,
	}, nil
}

func (s *Store) MarkDiscord(ctx context.Context, progressID, discordURL string) error {
	_, err := s.pool.Exec(ctx, `UPDATE progress SET discord_posted=true, discord_url=$2 WHERE id=$1`, progressID, discordURL)
	return err
}

func (s *Store) SetLevel(ctx context.Context, playerID string, in contract.LevelRequest) (contract.LevelResponse, error) {
	if err := validUUID(in.CharacterID); err != nil {
		return contract.LevelResponse{}, err
	}
	in.BattleTag = strings.TrimSpace(in.BattleTag)
	if in.BattleTag != "" {
		if err := validateBattleTag(in.BattleTag); err != nil {
			return contract.LevelResponse{}, err
		}
	}
	if in.Level < 1 || in.Level > 1000 {
		return contract.LevelResponse{}, fmt.Errorf("%w: level", ErrInvalid)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return contract.LevelResponse{}, err
	}
	defer tx.Rollback(ctx)
	var name, tag string
	var level int
	err = tx.QueryRow(ctx, `
		SELECT c.name, c.level, COALESCE(p.battletag, '')
		FROM characters c JOIN players p ON p.id = c.player_id
		WHERE c.id=$1 AND c.player_id=$2`, in.CharacterID, playerID).Scan(&name, &level, &tag)
	if errors.Is(err, pgx.ErrNoRows) {
		return contract.LevelResponse{Allowed: false}, ErrForbidden
	}
	if err != nil {
		return contract.LevelResponse{}, err
	}
	if tag != "" && !strings.EqualFold(tag, in.BattleTag) {
		return contract.LevelResponse{Allowed: false}, ErrForbidden
	}
	if level == in.Level {
		if err := tx.Commit(ctx); err != nil {
			return contract.LevelResponse{}, err
		}
		return contract.LevelResponse{Allowed: true, Level: level, Changed: false}, nil
	}
	if _, err := tx.Exec(ctx, `UPDATE characters SET level=$2, updated_at=now() WHERE id=$1`, in.CharacterID, in.Level); err != nil {
		return contract.LevelResponse{}, err
	}
	body := fmt.Sprintf("%s nådde nivå %d.", name, in.Level)
	if _, err := tx.Exec(ctx, `
		INSERT INTO adventure_log (player_id, character_id, type, title, body)
		VALUES ($1,$2,'level',$3,$4)`, playerID, in.CharacterID, fmt.Sprintf("Nivå %d", in.Level), body); err != nil {
		return contract.LevelResponse{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return contract.LevelResponse{}, err
	}
	return contract.LevelResponse{Allowed: true, Level: in.Level, Changed: true}, nil
}

func (s *Store) AddLog(ctx context.Context, playerID, characterID, questID, typ, title, body string) (contract.LogEntry, error) {
	if typ == "" || title == "" || body == "" {
		return contract.LogEntry{}, fmt.Errorf("%w: log fields", ErrInvalid)
	}
	if err := clip(typ, 40); err != nil || clip(title, 200) != nil || clip(body, 8000) != nil {
		return contract.LogEntry{}, fmt.Errorf("%w: log text", ErrInvalid)
	}
	if playerID != "" {
		if err := validUUID(playerID); err != nil {
			return contract.LogEntry{}, err
		}
	}
	if characterID != "" {
		if err := validUUID(characterID); err != nil {
			return contract.LogEntry{}, err
		}
	}
	if questID != "" {
		if err := validUUID(questID); err != nil {
			return contract.LogEntry{}, err
		}
	}
	var id string
	var created time.Time
	err := s.pool.QueryRow(ctx, `
		INSERT INTO adventure_log (player_id, character_id, quest_id, type, title, body)
		VALUES (NULLIF($1,'')::uuid, NULLIF($2,'')::uuid, NULLIF($3,'')::uuid, $4, $5, $6)
		RETURNING id::text, created_at`, playerID, characterID, questID, typ, title, body).Scan(&id, &created)
	if err != nil {
		return contract.LogEntry{}, err
	}
	return contract.LogEntry{
		ID: id, PlayerID: playerID, CharacterID: characterID, QuestID: questID,
		Type: typ, Title: title, Body: body, CreatedAt: contract.FormatTime(created),
	}, nil
}

func (s *Store) ListLog(ctx context.Context, battletag, characterID string, limit int) ([]contract.LogEntry, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if characterID != "" {
		if err := validUUID(characterID); err != nil {
			return nil, err
		}
	}
	rows, err := s.pool.Query(ctx, `
		SELECT l.id::text, COALESCE(l.player_id::text,''), COALESCE(l.character_id::text,''), COALESCE(l.quest_id::text,''),
		       l.type, l.title, l.body, l.created_at
		FROM adventure_log l
		LEFT JOIN players p ON p.id = l.player_id
		WHERE ($1 = '' OR p.battletag = $1)
		  AND ($2 = '' OR l.character_id::text = $2)
		ORDER BY l.created_at DESC
		LIMIT $3`, battletag, characterID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []contract.LogEntry
	for rows.Next() {
		var e contract.LogEntry
		var created time.Time
		if err := rows.Scan(&e.ID, &e.PlayerID, &e.CharacterID, &e.QuestID, &e.Type, &e.Title, &e.Body, &created); err != nil {
			return nil, err
		}
		e.CreatedAt = contract.FormatTime(created)
		out = append(out, e)
	}
	if out == nil {
		out = []contract.LogEntry{}
	}
	return out, rows.Err()
}

const sessionTTL = 30 * 24 * time.Hour

// IssueSession finds or creates the player for a verified Smia subject and
// returns a new GutSaga session token. The Smia access token is not stored.
func (s *Store) IssueSession(ctx context.Context, subject, email, displayName string) (contract.SessionResponse, error) {
	subject = strings.TrimSpace(subject)
	email = strings.TrimSpace(email)
	displayName = strings.TrimSpace(displayName)
	if subject == "" || len(subject) > 200 {
		return contract.SessionResponse{}, fmt.Errorf("%w: smia subject", ErrInvalid)
	}
	if displayName == "" {
		displayName = email
		if i := strings.Index(displayName, "@"); i > 0 {
			displayName = displayName[:i]
		}
	}
	if displayName == "" {
		displayName = "Spiller"
	}
	if len([]rune(displayName)) > 80 {
		displayName = string([]rune(displayName)[:80])
	}
	if len(email) > 200 {
		email = email[:200]
	}
	var p contract.Player
	var created time.Time
	err := s.pool.QueryRow(ctx, `
		INSERT INTO players (display_name, email, smia_subject)
		VALUES ($1, $2, $3)
		ON CONFLICT (smia_subject) WHERE smia_subject IS NOT NULL AND smia_subject <> ''
		DO UPDATE SET
			email = CASE WHEN EXCLUDED.email <> '' THEN EXCLUDED.email ELSE players.email END
		RETURNING id::text, COALESCE(battletag, ''), display_name, COALESCE(email, ''), COALESCE(smia_subject, ''), created_at`,
		displayName, email, subject).Scan(&p.ID, &p.BattleTag, &p.DisplayName, &p.Email, &p.SmiaSubject, &created)
	if err != nil {
		return contract.SessionResponse{}, mapPG(err)
	}
	p.CreatedAt = contract.FormatTime(created)
	token, err := newSessionToken()
	if err != nil {
		return contract.SessionResponse{}, err
	}
	expires := time.Now().UTC().Add(sessionTTL)
	if _, err := s.pool.Exec(ctx, `
		INSERT INTO sessions (player_id, token_hash, expires_at) VALUES ($1, $2, $3)`,
		p.ID, hashKey(token), expires); err != nil {
		return contract.SessionResponse{}, err
	}
	return contract.SessionResponse{
		TokenType: "Bearer", SessionToken: token, ExpiresAt: contract.FormatTime(expires), Player: p,
	}, nil
}

// UpsertCharacter stores a character reported by the addon for the logged-in player.
func (s *Store) UpsertCharacter(ctx context.Context, playerID string, in contract.CharacterUpsert) (contract.Character, error) {
	if err := validUUID(playerID); err != nil {
		return contract.Character{}, err
	}
	in.Name = strings.TrimSpace(in.Name)
	in.Realm = strings.TrimSpace(in.Realm)
	in.Race = strings.TrimSpace(in.Race)
	in.Class = strings.TrimSpace(in.Class)
	in.BattleTag = strings.TrimSpace(in.BattleTag)
	if err := validateName(in.Name); err != nil {
		return contract.Character{}, err
	}
	if err := validateRealm(in.Realm); err != nil {
		return contract.Character{}, err
	}
	if err := clip(in.Race, 40); err != nil || clip(in.Class, 40) != nil {
		return contract.Character{}, fmt.Errorf("%w: character text", ErrInvalid)
	}
	if in.Level == 0 {
		in.Level = 1
	}
	if in.Level < 1 || in.Level > 1000 {
		return contract.Character{}, fmt.Errorf("%w: level", ErrInvalid)
	}
	if in.BattleTag != "" {
		if err := validateBattleTag(in.BattleTag); err != nil {
			return contract.Character{}, err
		}
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return contract.Character{}, err
	}
	defer tx.Rollback(ctx)
	var tag string
	err = tx.QueryRow(ctx, `SELECT COALESCE(battletag, '') FROM players WHERE id=$1`, playerID).Scan(&tag)
	if errors.Is(err, pgx.ErrNoRows) {
		return contract.Character{}, ErrNotFound
	}
	if err != nil {
		return contract.Character{}, err
	}
	if in.BattleTag != "" {
		if tag == "" {
			if _, err := tx.Exec(ctx, `UPDATE players SET battletag=$2 WHERE id=$1`, playerID, in.BattleTag); err != nil {
				return contract.Character{}, mapPG(err)
			}
		} else if !strings.EqualFold(tag, in.BattleTag) {
			return contract.Character{}, fmt.Errorf("%w: battletag", ErrConflict)
		}
	}
	var id string
	var oldLevel int
	err = tx.QueryRow(ctx, `
		SELECT id::text, level FROM characters
		WHERE player_id=$1 AND lower(name)=lower($2) AND lower(realm)=lower($3)`,
		playerID, in.Name, in.Realm).Scan(&id, &oldLevel)
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `
			INSERT INTO characters (player_id, name, realm, race, class, level)
			VALUES ($1,$2,$3,$4,$5,$6) RETURNING id::text`,
			playerID, in.Name, in.Realm, in.Race, in.Class, in.Level).Scan(&id)
		if err != nil {
			return contract.Character{}, mapPG(err)
		}
	} else if err != nil {
		return contract.Character{}, err
	} else {
		if _, err := tx.Exec(ctx, `
			UPDATE characters SET race=$2, class=$3, level=$4, updated_at=now() WHERE id=$1`,
			id, in.Race, in.Class, in.Level); err != nil {
			return contract.Character{}, err
		}
		if oldLevel != in.Level {
			body := fmt.Sprintf("%s nådde nivå %d.", in.Name, in.Level)
			if _, err := tx.Exec(ctx, `
				INSERT INTO adventure_log (player_id, character_id, type, title, body)
				VALUES ($1,$2,'level',$3,$4)`, playerID, id, fmt.Sprintf("Nivå %d", in.Level), body); err != nil {
				return contract.Character{}, err
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return contract.Character{}, err
	}
	return s.GetCharacter(ctx, id)
}

func (s *Store) QuestIDByTitle(ctx context.Context, title string) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx, `SELECT id::text FROM quests WHERE title=$1 ORDER BY created_at LIMIT 1`, title).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	return id, err
}

func (s *Store) CachedIdentity(ctx context.Context, tokenHash string) (subject, email, name, hub, server string, member, ok bool, err error) {
	err = s.pool.QueryRow(ctx, `
		SELECT subject, email, display_name, member, hub, server
		FROM smia_token_cache WHERE token_hash=$1 AND expires_at > now()`, tokenHash).Scan(&subject, &email, &name, &member, &hub, &server)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", "", "", "", false, false, nil
	}
	if err != nil {
		return "", "", "", "", "", false, false, err
	}
	return subject, email, name, hub, server, member, true, nil
}

func (s *Store) PutCachedIdentity(ctx context.Context, tokenHash, subject, email, name, hub, server string, member bool, until time.Time) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO smia_token_cache (token_hash, subject, email, display_name, member, hub, server, expires_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (token_hash) DO UPDATE SET
			subject=EXCLUDED.subject, email=EXCLUDED.email, display_name=EXCLUDED.display_name,
			member=EXCLUDED.member, hub=EXCLUDED.hub, server=EXCLUDED.server, expires_at=EXCLUDED.expires_at`,
		tokenHash, subject, email, name, member, hub, server, until)
	return err
}
