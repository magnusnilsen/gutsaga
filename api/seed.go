package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/magnusnilsen/gutsaga/internal/contract"
)

// Map positions are fractions of UiMap 37 (Elwynn Forest) on retail, Classic Era,
// and Mists Classic. Lion's Pride Inn / Goldshire is about 43.8, 65.8. The north
// road where Falkhaan Isenstrider stands is about 45.6, 47.7. Innkeeper Farley,
// Refreshing Spring Water (159), and Ice Cold Milk (1179) exist in all three.
// NPC names are enUS because UnitName follows the client language.
func seedQuests() []QuestInput {
	goldX, goldY := 0.438, 0.658
	goldR := 0.05
	roadX, roadY := 0.456, 0.477
	roadR := 0.04
	water := 159
	milk := 1179
	one := 1
	return []QuestInput{
		{
			Title: "En kveld i Goldshire",
			Description: "Gå til Lion's Pride Inn i hjertet av Goldshire.\n\n" +
				"Snakk med verten Farley om dagens rykter, og sørg for at du har kildevann i sekken før du setter deg ned ved peisen.",
			PartyQuest: true,
			CreatedBy:  "GutSaga#0000",
			Objectives: []contract.Objective{
				{ID: "goldshire", Type: contract.ObjectiveLocation, Text: "Stå i eller rett utenfor Lion's Pride Inn i Goldshire", Subzone: "Goldshire|Lion's Pride Inn", MapID: intPtr(37), X: &goldX, Y: &goldY, Radius: &goldR},
				{ID: "farley", Type: contract.ObjectiveInteract, Text: "Snakk med Innkeeper Farley", NPC: "Innkeeper Farley"},
				{ID: "water", Type: contract.ObjectiveCollect, Text: "Ha 1 Refreshing Spring Water (kjøpes av Farley)", ItemID: &water, ItemName: "Refreshing Spring Water", Count: one},
			},
		},
		{
			Title: "Veien fra Northshire",
			Description: "Nord for Goldshire ligger veien ut av Northshire Abbey.\n\n" +
				"Stå der Falkhaan Isenstrider pleier å stå, der skogen åpner seg mot Elwynn.",
			PartyQuest: true,
			CreatedBy:  "GutSaga#0000",
			Objectives: []contract.Objective{
				{ID: "road", Type: contract.ObjectiveLocation, Text: "Stå på veien sør for Northshire", MapID: intPtr(37), X: &roadX, Y: &roadY, Radius: &roadR},
			},
		},
		{
			Title: "Melk til vaktlaget",
			Description: "Vaktene i Goldshire trenger noe kaldt å drikke.\n\n" +
				"Ice Cold Milk selges av Innkeeper Farley — kjøp én og møt opp på plassen foran innstedet.",
			PartyQuest: true,
			CreatedBy:  "GutSaga#0000",
			Objectives: []contract.Objective{
				{ID: "square", Type: contract.ObjectiveLocation, Text: "Stå i Goldshire", Subzone: "Goldshire|Lion's Pride Inn", MapID: intPtr(37), X: &goldX, Y: &goldY, Radius: &goldR},
				{ID: "milk", Type: contract.ObjectiveCollect, Text: "Ha 1 Ice Cold Milk", ItemID: &milk, ItemName: "Ice Cold Milk", Count: one},
			},
		},
	}
}

func intPtr(n int) *int { return &n }

func seed(ctx context.Context, store *Store) (created, updated int, err error) {
	if _, err = store.EnsurePlayer(ctx, "GutSaga#0000", "GutSaga"); err != nil {
		return 0, 0, fmt.Errorf("system creator: %w", err)
	}
	for _, q := range seedQuests() {
		id, findErr := store.QuestIDByTitle(ctx, q.Title)
		if errors.Is(findErr, ErrNotFound) {
			if _, err = store.CreateQuest(ctx, q); err != nil {
				return created, updated, fmt.Errorf("%s: %w", q.Title, err)
			}
			created++
			continue
		}
		if findErr != nil {
			return created, updated, findErr
		}
		objectives := q.Objectives
		active := true
		if _, err = store.UpdateQuest(ctx, id, QuestPatch{
			Description: &q.Description,
			Objectives:  &objectives,
			PartyQuest:  &q.PartyQuest,
			Active:      &active,
			CreatedBy:   &q.CreatedBy,
			Changes:     []string{"Oppdatert fra seed"},
		}); err != nil {
			return created, updated, fmt.Errorf("%s: %w", q.Title, err)
		}
		updated++
	}
	return created, updated, nil
}
