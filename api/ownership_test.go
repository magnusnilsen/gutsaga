package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/magnusnilsen/gutsaga/internal/contract"
)

func TestAdminPatchRequiresMatchingUpdatedBy(t *testing.T) {
	resetDB(t)
	srv := httptest.NewServer((&Server{store: NewStore(testPool), adminToken: "admin-test"}).Handler())
	t.Cleanup(srv.Close)
	admin := apiClient{base: srv.URL, token: "admin-test", http: srv.Client()}

	var quest contract.Quest
	resp, err := admin.do(http.MethodPost, "/v1/admin/sagas", map[string]any{
		"title": "Eies av Magnus", "description": "Kun forfatteren.",
		"objectives": []map[string]any{{"id": "x", "type": "talk", "text": "Snakk", "npc": "Farley"}},
		"created_by": "Magnus#2231", "party": true,
	}, &quest)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("create %d %v", statusOf(resp), err)
	}

	resp, err = admin.do(http.MethodPatch, "/v1/admin/sagas/"+quest.ID, map[string]any{
		"title": "Hijack", "updated_by": "Other#9999",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("want 403 for wrong author, got %d", resp.StatusCode)
	}

	var ok contract.Quest
	resp, err = admin.do(http.MethodPatch, "/v1/admin/sagas/"+quest.ID, map[string]any{
		"title": "Oppdatert", "updated_by": "magnus#2231",
	}, &ok)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("author patch %d %v", statusOf(resp), err)
	}
	if ok.Title != "Oppdatert" || ok.Version != 2 {
		t.Fatalf("patched %+v", ok)
	}

	// Omitting updated_by keeps full admin power (ops / scripts).
	resp, err = admin.do(http.MethodPatch, "/v1/admin/sagas/"+quest.ID, map[string]any{
		"title": "Admin override",
	}, &ok)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("admin without updated_by %d %v", statusOf(resp), err)
	}
}

func TestAdminPatchRequiresMatchingEditorEmail(t *testing.T) {
	resetDB(t)
	srv := httptest.NewServer((&Server{store: NewStore(testPool), adminToken: "admin-test"}).Handler())
	t.Cleanup(srv.Close)
	admin := apiClient{base: srv.URL, token: "admin-test", http: srv.Client()}

	var quest contract.Quest
	resp, err := admin.do(http.MethodPost, "/v1/admin/sagas", map[string]any{
		"title": "Eies via e-post", "description": "Smia-bruker.",
		"objectives": []map[string]any{{"id": "x", "type": "talk", "text": "Snakk", "npc": "Farley"}},
		"created_by": "Magnus#2231", "party": true,
	}, &quest)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("create %d %v", statusOf(resp), err)
	}
	_, err = testPool.Exec(context.Background(), `UPDATE players SET email='magnus@sparkel.io' WHERE battletag='Magnus#2231'`)
	if err != nil {
		t.Fatal(err)
	}

	resp, err = admin.do(http.MethodPatch, "/v1/admin/sagas/"+quest.ID, map[string]any{
		"title": "Hijack", "editor_email": "other@example.com",
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("want 403 for wrong email, got %d", resp.StatusCode)
	}

	var ok contract.Quest
	resp, err = admin.do(http.MethodPatch, "/v1/admin/sagas/"+quest.ID, map[string]any{
		"title": "Oppdatert", "editor_email": "Magnus@Sparkel.io",
	}, &ok)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("author email patch %d %v", statusOf(resp), err)
	}
	if ok.Title != "Oppdatert" {
		t.Fatalf("patched %+v", ok)
	}
}

func TestPublicPlaceholderPNG(t *testing.T) {
	srv := httptest.NewServer((&Server{store: NewStore(testPool), adminToken: "admin-test"}).Handler())
	t.Cleanup(srv.Close)
	for _, path := range []string{"/v1/public/placeholder.png", "/v1/public/empty.png"} {
		resp, err := srv.Client().Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "image/png" {
			resp.Body.Close()
			t.Fatalf("%s status=%d ct=%s", path, resp.StatusCode, resp.Header.Get("Content-Type"))
		}
		n, _ := io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if n < 1000 {
			t.Fatalf("%s too small (%d bytes); media cards need a real-size placeholder", path, n)
		}
	}
}
