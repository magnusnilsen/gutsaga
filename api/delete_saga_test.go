package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/magnusnilsen/gutsaga/internal/contract"
)

func TestAdminDeleteSaga(t *testing.T) {
	resetDB(t)
	srv := httptest.NewServer((&Server{store: NewStore(testPool), adminToken: "admin-test"}).Handler())
	t.Cleanup(srv.Close)
	admin := apiClient{base: srv.URL, token: "admin-test", http: srv.Client()}

	var quest contract.Quest
	resp, err := admin.do(http.MethodPost, "/v1/admin/sagas", map[string]any{
		"title": "Slapp Dermot", "description": "Test",
		"objectives": []map[string]any{{"id": "x", "type": "emote", "text": "Slap", "emote": "SLAP", "npc": "Dermot"}},
		"created_by": "Magnus#2231", "party": true,
	}, &quest)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("create %d %v", statusOf(resp), err)
	}

	resp, err = admin.do(http.MethodDelete, "/v1/admin/sagas/"+quest.ID, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete want 204, got %d", resp.StatusCode)
	}

	resp, err = admin.do(http.MethodGet, "/v1/admin/sagas/"+quest.ID, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("get after delete want 404, got %d", resp.StatusCode)
	}

	resp, err = admin.do(http.MethodDelete, "/v1/admin/sagas/"+quest.ID, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("second delete want 404, got %d", resp.StatusCode)
	}
}

func TestDeleteSagaRequiresAdminToken(t *testing.T) {
	resetDB(t)
	srv := httptest.NewServer((&Server{store: NewStore(testPool), adminToken: "admin-test"}).Handler())
	t.Cleanup(srv.Close)
	admin := apiClient{base: srv.URL, token: "admin-test", http: srv.Client()}

	var quest contract.Quest
	resp, err := admin.do(http.MethodPost, "/v1/admin/sagas", map[string]any{
		"title": "Keep me", "description": "Auth check",
		"objectives": []map[string]any{{"id": "x", "type": "talk", "text": "Snakk", "npc": "Farley"}},
		"created_by": "Magnus#2231", "party": true,
	}, &quest)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("create %d %v", statusOf(resp), err)
	}

	noAuth := apiClient{base: srv.URL, http: srv.Client()}
	resp, err = noAuth.do(http.MethodDelete, "/v1/admin/sagas/"+quest.ID, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no token want 401, got %d", resp.StatusCode)
	}

	wrong := apiClient{base: srv.URL, token: "wrong", http: srv.Client()}
	resp, err = wrong.do(http.MethodDelete, "/v1/admin/sagas/"+quest.ID, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong token want 401, got %d", resp.StatusCode)
	}

	// Player API has no saga delete route.
	var created contract.CreatePlayerResponse
	resp, err = admin.do(http.MethodPost, "/v1/admin/players", map[string]string{
		"battletag": "Player#1", "display_name": "Player",
	}, &created)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("player %d %v", statusOf(resp), err)
	}
	player := apiClient{base: srv.URL, token: created.APIKey, http: srv.Client()}
	resp, err = player.do(http.MethodDelete, "/v1/sagas/"+quest.ID, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusMethodNotAllowed && resp.StatusCode != http.StatusNotFound {
		t.Fatalf("player delete want 404/405, got %d", resp.StatusCode)
	}

	resp, err = admin.do(http.MethodGet, "/v1/admin/sagas/"+quest.ID, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("saga should still exist after failed deletes, got %d", resp.StatusCode)
	}
}
