package chatlog

import "testing"

func TestParseCompleteRealmWithSpaces(t *testing.T) {
	line := `10/22 14:44:00.123  [SagaBoard] COMPLETE 11111111-1111-1111-1111-111111111111 3 Magni Argent Dawn 12 1761144240`
	ev, ok := Parse(line)
	if !ok {
		t.Fatal("not parsed")
	}
	if ev.Kind != KindComplete || ev.QuestID != "11111111-1111-1111-1111-111111111111" || ev.QuestVersion != 3 {
		t.Fatalf("%+v", ev)
	}
	if ev.Character != "Magni" || ev.Realm != "Argent Dawn" || ev.Level != 12 || ev.At.Unix() != 1761144240 {
		t.Fatalf("%+v", ev)
	}
}

func TestParseCompleteFromChannelEcho(t *testing.T) {
	// Channel named GutSaga prefixes the line; payload must still parse.
	line := `10/22 22:30:00.000  [SagaBoard] Magní: [SagaBoard] COMPLETE 11111111-1111-1111-1111-111111111111 3 Magní Celebras 2 1761144240`
	ev, ok := Parse(line)
	if !ok || ev.Kind != KindComplete || ev.Character != "Magní" || ev.Realm != "Celebras" {
		t.Fatalf("%+v ok=%v", ev, ok)
	}
}

func TestParseCharacterPipes(t *testing.T) {
	line := `[SagaBoard] CHARACTER Magni|Argent Dawn|12|Dwarf|Warrior|Magnus#2231|1761144240`
	ev, ok := Parse(line)
	if !ok || ev.Kind != KindCharacter || ev.Character != "Magni" || ev.Realm != "Argent Dawn" {
		t.Fatalf("%+v ok=%v", ev, ok)
	}
	if ev.Level != 12 || ev.Race != "Dwarf" || ev.Class != "Warrior" || ev.BattleTag != "Magnus#2231" {
		t.Fatalf("%+v", ev)
	}
	if _, ok := Parse(`[SagaBoard] CHARACTER Magni|Realm|12|Dwarf|Warrior|notag|1761144240`); ok {
		t.Fatal("battletag")
	}
}

func TestParseLevelAndJunk(t *testing.T) {
	ev, ok := Parse(`[SagaBoard] LEVELUP Magni Kel'Thuzad 13 1761144300`)
	if !ok || ev.Kind != KindLevel || ev.Realm != "Kel'Thuzad" || ev.Level != 13 {
		t.Fatalf("%+v ok=%v", ev, ok)
	}
	if _, ok := Parse(`[SagaBoard] COMPLETE bare-id`); ok {
		t.Fatal("short line should fail")
	}
	if _, ok := Parse("GutSaga: Fullført – noe"); ok {
		t.Fatal("friendly line must not parse")
	}
	if _, ok := Parse(`[SagaBoard] COMPLETE id 1 Magni - 0 1761144240`); ok {
		t.Fatal("level 0")
	}
	ev, ok = Parse(`[SagaBoard] COMPLETE abc 0 Magni - 4 1761144240`)
	if !ok || ev.Realm != "" || ev.QuestVersion != 0 || ev.Level != 4 {
		t.Fatalf("%+v", ev)
	}
}
