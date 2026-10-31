package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/magnusnilsen/gutsaga/internal/contract"
)

func TestEnsurePersonalGroupOnSync(t *testing.T) {
	resetDB(t)
	srv := httptest.NewServer((&Server{store: NewStore(testPool)}).Handler())
	t.Cleanup(srv.Close)
	player, token := devSession(t, "bnet-new", "Newbie#1", "Newbie")
	store := NewStore(testPool)
	if _, err := store.UpsertCharacter(context.Background(), player.ID, contract.CharacterUpsert{
		Name: "Magni", Realm: "Everlook", Level: 5, BattleTag: "Newbie#1",
	}); err != nil {
		t.Fatal(err)
	}
	api := apiClient{base: srv.URL, token: token, http: srv.Client()}

	var sync contract.SyncResponse
	resp, err := api.do(http.MethodGet, "/v1/sync", nil, &sync)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("sync %d %v", statusOf(resp), err)
	}
	if len(sync.Groups) != 1 || !sync.Groups[0].Personal || sync.Groups[0].Name != "Personal" {
		t.Fatalf("groups %+v", sync.Groups)
	}
	if len(sync.Quests) != 3 {
		t.Fatalf("want 3 starters, got %d", len(sync.Quests))
	}
	var participating bool
	if err := testPool.QueryRow(context.Background(), `
		SELECT participating FROM characters WHERE player_id=$1 AND name='Magni'`, player.ID).Scan(&participating); err != nil || !participating {
		t.Fatalf("participating=%v err=%v", participating, err)
	}

	resp, err = api.do(http.MethodGet, "/v1/sync", nil, &sync)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("second sync %d %v", statusOf(resp), err)
	}
	if len(sync.Groups) != 1 || len(sync.Quests) != 3 {
		t.Fatalf("not idempotent groups=%d quests=%d", len(sync.Groups), len(sync.Quests))
	}

	resp, err = api.do(http.MethodPost, "/v1/groups/"+sync.Groups[0].ID+"/invites", map[string]string{
		"name": "Other", "realm": "Everlook",
	}, nil)
	if err != nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("invite personal %d %v", statusOf(resp), err)
	}
	resp, err = api.do(http.MethodPost, "/v1/groups/"+sync.Groups[0].ID+"/leave", nil, nil)
	if err != nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("leave personal %d %v", statusOf(resp), err)
	}
}

func TestSyncPreloadsPendingJoinSagas(t *testing.T) {
	resetDB(t)
	srv := httptest.NewServer((&Server{store: NewStore(testPool)}).Handler())
	t.Cleanup(srv.Close)
	owner, ownerToken := devSession(t, "bnet-owner", "Owner#1", "Owner")
	_, guestToken := devSession(t, "bnet-guest", "Guest#9", "Guest")
	group := devGroup(t, owner.ID, "gutar", "Gutar")
	ownerAPI := apiClient{base: srv.URL, token: ownerToken, http: srv.Client()}
	guestAPI := apiClient{base: srv.URL, token: guestToken, http: srv.Client()}

	var saga contract.Quest
	resp, err := ownerAPI.do(http.MethodPost, "/v1/sagas", map[string]any{
		"title": "Friend saga", "party": true, "group": "gutar",
		"objectives": []map[string]any{{"id": "x", "type": "talk", "text": "Talk", "npc": "Farley"}},
	}, &saga)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("create saga %d %v", statusOf(resp), err)
	}

	var invite contract.Invite
	resp, err = ownerAPI.do(http.MethodPost, "/v1/groups/"+group.ID+"/invites", map[string]string{
		"name": "Magni", "realm": "Everlook", "battletag": "Guest#9",
	}, &invite)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("invite %d %v", statusOf(resp), err)
	}

	var sync contract.SyncResponse
	resp, err = guestAPI.do(http.MethodGet, "/v1/sync", nil, &sync)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("guest sync %d %v", statusOf(resp), err)
	}
	personal := 0
	pending := 0
	for _, q := range sync.Quests {
		if q.PendingInviteID == invite.ID {
			pending++
			if q.ID != saga.ID {
				t.Fatalf("pending quest %+v", q)
			}
		}
		if q.PendingInviteID == "" {
			personal++
		}
	}
	if personal != 3 || pending != 1 {
		t.Fatalf("personal=%d pending=%d quests=%d", personal, pending, len(sync.Quests))
	}
	if len(sync.Invites) != 1 || sync.Invites[0].ID != invite.ID {
		t.Fatalf("invites %+v", sync.Invites)
	}

	var accepted contract.Invite
	resp, err = guestAPI.do(http.MethodPost, "/v1/invites/"+invite.ID+"/accept", map[string]string{}, &accepted)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("accept %d %v", statusOf(resp), err)
	}
	var memberCount int
	if err := testPool.QueryRow(context.Background(), `
		SELECT count(*) FROM group_members WHERE group_id=$1`, group.ID).Scan(&memberCount); err != nil {
		t.Fatal(err)
	}
	if memberCount < 2 {
		t.Fatalf("after accept members=%d accepted=%+v", memberCount, accepted)
	}
	sync = contract.SyncResponse{}
	resp, err = guestAPI.do(http.MethodGet, "/v1/sync", nil, &sync)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("after accept %d %v", statusOf(resp), err)
	}
	found := false
	for _, q := range sync.Quests {
		if q.ID == saga.ID {
			found = true
			if q.PendingInviteID != "" {
				t.Fatalf("still pending quest=%+v invites=%+v groups=%+v", q, sync.Invites, sync.Groups)
			}
		}
	}
	if !found {
		t.Fatal("friend saga missing after accept")
	}
	if len(sync.Groups) < 2 {
		t.Fatalf("want personal+gutar, got %+v", sync.Groups)
	}
}