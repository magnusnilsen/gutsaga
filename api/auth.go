package main

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/magnusnilsen/gutsaga/internal/contract"
	"github.com/magnusnilsen/gutsaga/internal/smia"
)

type storeSmiaCache struct{ store *Store }

func (c storeSmiaCache) Get(ctx context.Context, tokenHash string) (smia.Identity, bool, error) {
	subject, email, name, hub, server, member, ok, err := c.store.CachedIdentity(ctx, tokenHash)
	if err != nil || !ok {
		return smia.Identity{}, ok, err
	}
	return smia.Identity{
		Subject: subject, Email: email, DisplayName: name, Member: member, Hub: hub, Server: server,
	}, true, nil
}

func (c storeSmiaCache) Put(ctx context.Context, tokenHash string, id smia.Identity, until int64) error {
	return c.store.PutCachedIdentity(ctx, tokenHash, id.Subject, id.Email, id.DisplayName, id.Hub, id.Server, id.Member, time.Unix(until, 0).UTC())
}

func (s *Server) session(w http.ResponseWriter, r *http.Request) {
	if s.smia == nil {
		writeErr(w, http.StatusServiceUnavailable, "smia_not_configured", "Smia-innlogging er ikke satt opp")
		return
	}
	token := bearer(r)
	if token == "" {
		writeErr(w, http.StatusUnauthorized, "unauthorized", "mangler Smia-token")
		return
	}
	// Identity is taken only from the verifier. A JSON body is ignored on purpose.
	id, err := s.smia.Verify(r.Context(), token)
	if err != nil {
		s.fail(w, err)
		return
	}
	if !id.Member || !strings.EqualFold(id.Hub, smia.Hub) || !strings.EqualFold(id.Server, smia.Server) {
		writeErr(w, http.StatusForbidden, "not_a_member", "Du må være medlem av Smia-hubben wow, server mcp")
		return
	}
	if strings.TrimSpace(id.Subject) == "" {
		writeErr(w, http.StatusBadGateway, "smia_unavailable", "Smia oppga ingen bruker")
		return
	}
	out, err := s.store.IssueSession(r.Context(), id.Subject, id.Email, id.DisplayName)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) upsertCharacter(w http.ResponseWriter, r *http.Request) {
	var in contract.CharacterUpsert
	if err := readJSON(r, &in, 1<<20); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid", "bad json")
		return
	}
	out, err := s.store.UpsertCharacter(r.Context(), playerFrom(r).ID, in)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}
