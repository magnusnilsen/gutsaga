package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/magnusnilsen/gutsaga/internal/contract"
)

// personalCreator is the BattleTag stamped on auto-cloned starter sagas.
const personalCreator = "Sagalore#0000"

// EnsurePersonalGroup creates one account-scoped Personal group with starter
// sagas if the player does not already own one. Safe to call on every Sync.
func (s *Store) EnsurePersonalGroup(ctx context.Context, playerID string) error {
	if err := validUUID(playerID); err != nil {
		return err
	}
	var existing string
	err := s.pool.QueryRow(ctx, `
		SELECT id::text FROM groups
		WHERE personal AND owner_player_id=$1`, playerID).Scan(&existing)
	if err == nil {
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}

	slug := personalSlug(playerID)
	if err := validateSlug(slug); err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var groupID string
	err = tx.QueryRow(ctx, `
		INSERT INTO groups (slug, name, locale, owner_player_id, personal)
		VALUES ($1,'Personal','en',$2,true)
		RETURNING id::text`, slug, playerID).Scan(&groupID)
	if err != nil {
		// Concurrent Sync may have won the unique index race.
		if mapped := mapPG(err); errors.Is(mapped, ErrConflict) {
			return nil
		}
		return mapPG(err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO group_members (group_id, player_id, role) VALUES ($1,$2,'owner')`, groupID, playerID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE characters SET participating = true WHERE player_id=$1`, playerID); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}

	if _, err := s.EnsurePlayer(ctx, personalCreator, "Sagalore"); err != nil {
		return fmt.Errorf("personal creator: %w", err)
	}
	for _, q := range personalStarterQuests() {
		q.GroupID = groupID
		q.CreatedBy = personalCreator
		if _, err := s.CreateQuest(ctx, q); err != nil {
			return fmt.Errorf("personal starter %q: %w", q.Title, err)
		}
	}
	return nil
}

func personalSlug(playerID string) string {
	hex := strings.ReplaceAll(playerID, "-", "")
	if len(hex) > 8 {
		hex = hex[:8]
	}
	return "p-" + strings.ToLower(hex)
}

func (s *Store) groupIsPersonal(ctx context.Context, groupID string) (bool, error) {
	var personal bool
	err := s.pool.QueryRow(ctx, `SELECT personal FROM groups WHERE id=$1`, groupID).Scan(&personal)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrNotFound
	}
	return personal, err
}

// personalStarterQuests are zone- and faction-agnostic. Emotes work anywhere
// in the open world, so every new account can finish them on any character.
func personalStarterQuests() []QuestInput {
	return []QuestInput{
		{
			Title:       "Say hello",
			Description: "A saga can watch what you do in the world.\n\nWave at nobody in particular — /wave — to mark this one done.",
			PartyQuest:  true,
			Objectives: []contract.Objective{
				{ID: "wave", Type: contract.ObjectiveEmote, Text: "Wave (/wave)", Emote: "WAVE"},
			},
		},
		{
			Title:       "Take a seat",
			Description: "Rest for a moment.\n\nSit down with /sit. Stand up whenever you like afterward.",
			PartyQuest:  true,
			Objectives: []contract.Objective{
				{ID: "sit", Type: contract.ObjectiveEmote, Text: "Sit (/sit)", Emote: "SIT"},
			},
		},
		{
			Title:       "Celebrate",
			Description: "Something went right — or will.\n\nCheer with /cheer to finish this starter.",
			PartyQuest:  true,
			Objectives: []contract.Objective{
				{ID: "cheer", Type: contract.ObjectiveEmote, Text: "Cheer (/cheer)", Emote: "CHEER"},
			},
		},
	}
}
