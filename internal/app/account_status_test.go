package app

import (
	"testing"

	"github.com/magnusnilsen/gutsaga/internal/config"
	"github.com/magnusnilsen/gutsaga/internal/contract"
)

func TestAccountStatusLoggedOut(t *testing.T) {
	user := accountStatus(false, contract.Player{}, config.Config{}, contract.SyncResponse{})
	if user != "Ikke innlogget" {
		t.Fatalf("got %q", user)
	}
}

func TestAccountStatusPrefersSyncEmail(t *testing.T) {
	user := accountStatus(true, contract.Player{
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
}
