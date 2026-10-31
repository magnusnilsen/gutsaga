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

func personalStarterQuests() []QuestInput {
	goldX, goldY := 0.438, 0.658
	goldR := 0.05
	roadX, roadY := 0.456, 0.477
	roadR := 0.04
	water := 159
	milk := 1179
	one := 1
	return []QuestInput{
		{
			Title: "An evening in Goldshire",
			Description: "Go to Lion's Pride Inn in the heart of Goldshire.\n\n" +
				"Talk to Innkeeper Farley about the day's rumors, and make sure you have spring water in your bag before you sit by the fire.",
			PartyQuest: true,
			Objectives: []contract.Objective{
				{ID: "goldshire", Type: contract.ObjectiveLocation, Text: "Stand in or just outside Lion's Pride Inn in Goldshire", Subzone: "Goldshire|Lion's Pride Inn", MapID: intPtr(37), X: &goldX, Y: &goldY, Radius: &goldR},
				{ID: "farley", Type: contract.ObjectiveInteract, Text: "Talk to Innkeeper Farley", NPC: "Innkeeper Farley"},
				{ID: "water", Type: contract.ObjectiveCollect, Text: "Have 1 Refreshing Spring Water (sold by Farley)", ItemID: &water, ItemName: "Refreshing Spring Water", Count: one},
			},
		},
		{
			Title: "The road from Northshire",
			Description: "North of Goldshire is the road out of Northshire Abbey.\n\n" +
				"Stand where Falkhaan Isenstrider usually stands, where the forest opens toward Elwynn.",
			PartyQuest: true,
			Objectives: []contract.Objective{
				{ID: "road", Type: contract.ObjectiveLocation, Text: "Stand on the road south of Northshire", MapID: intPtr(37), X: &roadX, Y: &roadY, Radius: &roadR},
			},
		},
		{
			Title: "Milk for the watch",
			Description: "The guards in Goldshire need something cold to drink.\n\n" +
				"Ice Cold Milk is sold by Innkeeper Farley — buy one and meet on the square in front of the inn.",
			PartyQuest: true,
			Objectives: []contract.Objective{
				{ID: "square", Type: contract.ObjectiveLocation, Text: "Stand in Goldshire", Subzone: "Goldshire|Lion's Pride Inn", MapID: intPtr(37), X: &goldX, Y: &goldY, Radius: &goldR},
				{ID: "milk", Type: contract.ObjectiveCollect, Text: "Have 1 Ice Cold Milk", ItemID: &milk, ItemName: "Ice Cold Milk", Count: one},
			},
		},
	}
}
