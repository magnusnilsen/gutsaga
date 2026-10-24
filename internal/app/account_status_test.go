package app

import (
	"testing"

	"github.com/magnusnilsen/gutsaga/internal/config"
	"github.com/magnusnilsen/gutsaga/internal/contract"
)

func TestAccountStatusLoggedOut(t *testing.T) {
	user, tag, chars := accountStatus(false, contract.Player{}, config.Config{}, contract.SyncResponse{})
	if user != "Ikke innlogget" || tag != "BattleTag: —" || chars != nil {
		t.Fatalf("got %q %q %#v", user, tag, chars)
	}
}

func TestAccountStatusPrefersSyncEmailAndListsCharacters(t *testing.T) {
	user, tag, chars := accountStatus(true, contract.Player{
		DisplayName: "Magnus", BattleTag: "old#1",
	}, config.Config{BattleTag: "cfg#2"}, contract.SyncResponse{
		Email: "magnus@sparkel.io", BattleTag: "Magnus#2231",
		Characters: []contract.Character{
			{Name: "Magní", Realm: "Celebras", Class: "Hunter", Level: 2},
		},
	})
	if user != "Konto: magnus@sparkel.io" {
		t.Fatalf("user=%q", user)
	}
	if tag != "BattleTag: Magnus#2231" {
		t.Fatalf("tag=%q", tag)
	}
	if len(chars) != 1 || chars[0] != "Magní · Celebras · nivå 2 · Hunter" {
		t.Fatalf("chars=%#v", chars)
	}
}

func TestFormatCharacterLine(t *testing.T) {
	got := formatCharacterLine(contract.Character{Name: "Mira", Realm: "Celebras", Level: 8})
	if got != "Mira · Celebras · nivå 8" {
		t.Fatalf("got %q", got)
	}
}
