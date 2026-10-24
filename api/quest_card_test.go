package main

import (
	"strings"
	"testing"

	"github.com/magnusnilsen/gutsaga/internal/contract"
)

func TestBuildQuestCard(t *testing.T) {
	q := contract.Quest{
		Title: "Test",
		Objectives: []contract.Objective{
			{ID: "a", Type: "talk", Text: "Snakk med Farley"},
			{ID: "b", Type: "go", Text: "Stå i Goldshire"},
		},
	}
	card := buildQuestCard(q, nil)
	if card == nil || len(card.Columns) != 1 {
		t.Fatalf("columns %+v", card)
	}
	if len(card.Rows) != 3 { // 2 goals + empty completers
		t.Fatalf("rows %d %+v", len(card.Rows), card.Rows)
	}
	if card.Rows[0].Group != "Mål" || card.Rows[0].Cells["detail"] != "1. Snakk med Farley" {
		t.Fatalf("goal0 %+v", card.Rows[0])
	}
	if card.Rows[2].Group != "Fullført" {
		t.Fatalf("done %+v", card.Rows[2])
	}

	card = buildQuestCard(q, []contract.QuestCompleter{{
		Character: "Magni", Class: "Paladin", Level: 12, Realm: "Celebras", When: "22 oktober 2026 19:00",
	}})
	if len(card.Rows) != 3 {
		t.Fatalf("with completer rows %d", len(card.Rows))
	}
	got, _ := card.Rows[2].Cells["detail"].(string)
	if got == "" || got[:5] != "Magni" {
		t.Fatalf("completer line %q", got)
	}

	sub := buildQuestCardSubtitle(card)
	if !strings.Contains(sub, "Mål") || !strings.Contains(sub, "Fullført") || !strings.Contains(sub, "Magni") {
		t.Fatalf("subtitle %q", sub)
	}
}
