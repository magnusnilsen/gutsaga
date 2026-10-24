package contract

import "testing"

func TestNormalizeEmoteAndComposites(t *testing.T) {
	x, y := 0.5, 0.5
	item := 159
	in := []Objective{
		{
			ID: "wave", Type: "emote", Text: "Hils", Emote: "/salute",
			NPC: "Stormwind Guard", MapID: intPtr(84), X: &x, Y: &y,
			When: &TimeWindow{Start: "22:00", End: "02:00"},
		},
		{
			ID: "both", Type: "all_of", Text: "Stå med vann",
			Children: []Objective{
				{ID: "inn", Type: "location", Text: "Goldshire", Zone: "Elwynn Forest"},
				{ID: "water", Type: "collect", Text: "Vann", ItemID: &item},
			},
		},
		{
			ID: "either", Type: "any_of", Text: "Ett av dem",
			Children: []Objective{
				{ID: "talk", Type: "interact", Text: "Snakk", NPC: "Farley"},
				{ID: "bow", Type: "emote", Text: "Bukk", Emote: "bow"},
			},
		},
	}
	out, err := NormalizeObjectives(in, false)
	if err != nil {
		t.Fatal(err)
	}
	if out[0].Emote != "SALUTE" || out[0].When.Clock != ClockLocal || out[0].Radius == nil || *out[0].Radius != DefaultRadius {
		t.Fatalf("emote %+v", out[0])
	}
	if out[1].Children[1].Count != 1 || out[2].Children[1].Emote != "BOW" {
		t.Fatalf("%+v", out)
	}
	if _, err := NormalizeObjectives([]Objective{{
		ID: "bad", Type: "emote", Text: "Nei",
	}}, false); err == nil {
		t.Fatal("emote token required")
	}
	if _, err := NormalizeObjectives([]Objective{{
		ID: "one", Type: "all_of", Text: "For få",
		Children: []Objective{{ID: "a", Type: "location", Text: "Her", Zone: "Elwynn"}},
	}}, false); err == nil {
		t.Fatal("all_of needs two children")
	}
	if _, err := NormalizeObjectives([]Objective{{
		ID: "dup", Type: "any_of", Text: "Dup",
		Children: []Objective{
			{ID: "dup", Type: "location", Text: "A", Zone: "Elwynn"},
			{ID: "b", Type: "location", Text: "B", Zone: "Elwynn"},
		},
	}}, false); err == nil {
		t.Fatal("duplicate id")
	}
}

func TestNormalizeTimeAndGroup(t *testing.T) {
	w, err := NormalizeTimeWindow(&TimeWindow{Clock: "game", Start: "22:00", End: "02:00"})
	if err != nil || w.Clock != ClockGame {
		t.Fatalf("%+v %v", w, err)
	}
	if _, err := NormalizeTimeWindow(&TimeWindow{Clock: "game", StartsAt: "2026-10-24T20:00:00Z"}); err == nil {
		t.Fatal("game clock has no date")
	}
	if _, err := NormalizeTimeWindow(&TimeWindow{Start: "22:00"}); err == nil {
		t.Fatal("start without end")
	}
	w, err = NormalizeTimeWindow(&TimeWindow{
		StartsAt: "2026-10-24T20:00:00Z", EndsAt: "2026-10-25T06:00:00+02:00",
	})
	if err != nil || w.Clock != ClockLocal || w.EndsAt != "2026-10-25T04:00:00Z" {
		t.Fatalf("%+v %v", w, err)
	}
	level := 17
	g, err := NormalizeGroup(&GroupRule{Size: 4, MinLevel: &level, Instance: " The Deadmines "})
	if err != nil || g.Size != 4 || g.Instance != "The Deadmines" || g.MinLevel == nil || *g.MinLevel != 17 {
		t.Fatalf("%+v %v", g, err)
	}
	if _, err := NormalizeGroup(&GroupRule{Size: 0}); err == nil {
		t.Fatal("size")
	}
	members := EnsureMember(nil, "Magni", "Argent Dawn", 18)
	members = append(members, GroupMember{Name: "Mira", Realm: "Argent Dawn", Level: 17})
	members = append(members, GroupMember{Name: "Pip", Realm: "Argent Dawn", Level: 10})
	if err := GroupSatisfied(g, members, "Deadmines"); err == nil {
		t.Fatal("only two of three meet level 17, size is 4")
	}
	g.Size = 2
	if err := GroupSatisfied(g, members, "The Deadmines"); err != nil {
		t.Fatal(err)
	}
	if err := GroupSatisfied(g, members, "Elwynn Forest"); err == nil {
		t.Fatal("instance")
	}
	if !NameListed("Goldshire|Lion's Pride Inn", "Lion's Pride Inn") {
		t.Fatal("listed")
	}
}

func intPtr(n int) *int { return &n }
