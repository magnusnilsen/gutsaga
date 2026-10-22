package syncdata

import (
	"strings"
	"testing"

	"github.com/magnusnilsen/gutsaga/internal/contract"
)

func intptr(n int) *int { return &n }

func TestMergeRules(t *testing.T) {
	min10 := 10
	min40 := 40
	sync := contract.SyncResponse{
		Characters: []contract.Character{
			{ID: "c1", Name: "Magni", Realm: "Argent Dawn", Level: 12},
			{ID: "c2", Name: "Mira", Realm: "Argent Dawn", Level: 8},
		},
		Quests: []contract.Quest{
			{ID: "q-party", Title: "Lagoppdrag", Active: true, PartyQuest: true, Version: 2, MinLevel: &min10, AssignedTo: []contract.IDRef{}},
			{ID: "q-mine", Title: "Bare Magni", Active: true, Version: 4, MinLevel: intptr(1), AssignedTo: []contract.IDRef{{ID: "c1"}}},
			{ID: "q-high", Title: "For høyt", Active: true, PartyQuest: true, Version: 1, MinLevel: &min40},
			{ID: "q-off", Title: "Av", Active: false, PartyQuest: true, Version: 3},
			{ID: "q-done", Title: "Gammelt", Active: false, Version: 5, AssignedTo: []contract.IDRef{{ID: "c1"}}},
		},
		Progress: []contract.Progress{
			{QuestID: "q-done", CharacterID: "c1", Status: "completed", QuestVersion: 5, CompletedAt: "2026-10-01T00:00:00Z"},
		},
	}
	entries := Merge(sync)
	got := map[string]Entry{}
	for _, e := range entries {
		got[e.Character+"/"+e.QuestID] = e
	}
	if _, ok := got["Magni/q-party"]; !ok {
		t.Fatal("party quest should show for Magni")
	}
	if _, ok := got["Mira/q-party"]; ok {
		t.Fatal("Mira is under min level")
	}
	if e := got["Magni/q-mine"]; e.QuestVersion != 4 {
		t.Fatalf("version %d", e.QuestVersion)
	}
	if _, ok := got["Mira/q-mine"]; ok {
		t.Fatal("assigned only to Magni")
	}
	if _, ok := got["Magni/q-high"]; ok {
		t.Fatal("min level")
	}
	if _, ok := got["Magni/q-off"]; ok {
		t.Fatal("inactive")
	}
	if e := got["Magni/q-done"]; e.Status != "completed" {
		t.Fatal("completed inactive quest should remain")
	}
	if !strings.Contains(WriteLua("Magnus#2231", "2026-10-22T00:00:00Z", entries), `quest_version = 4`) {
		t.Fatal(WriteLua("Magnus#2231", "2026-10-22T00:00:00Z", entries))
	}
}

func TestLuaEscapes(t *testing.T) {
	lua := WriteLua(`Magnus#2231`, "t", []Entry{{
		Character: "Magni", Title: `Si "hei"`, Description: "linje\nto", QuestVersion: 1, QuestID: "q",
	}})
	if !strings.Contains(lua, `Si \"hei\"`) || !strings.Contains(lua, `linje\nto`) {
		t.Fatal(lua)
	}
}

func TestFindCharacter(t *testing.T) {
	sync := contract.SyncResponse{Characters: []contract.Character{
		{ID: "1", Name: "Magni", Realm: "Argent Dawn"},
		{ID: "2", Name: "Magni", Realm: "Silvermoon"},
	}}
	if _, ok := FindCharacter(sync, "magni", ""); ok {
		t.Fatal("ambiguous without realm")
	}
	c, ok := FindCharacter(sync, "Magni", "argent dawn")
	if !ok || c.ID != "1" {
		t.Fatalf("%+v %v", c, ok)
	}
}
