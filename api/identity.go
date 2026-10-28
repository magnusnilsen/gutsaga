package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/magnusnilsen/gutsaga/internal/contract"
)

func normalizeIdentity(provider, subject, email string) (string, string, string, error) {
	provider = strings.TrimSpace(provider)
	subject = strings.TrimSpace(subject)
	email = strings.TrimSpace(email)
	if provider == "" || len(provider) > 32 || subject == "" || len(subject) > 200 {
		return "", "", "", fmt.Errorf("%w: identity", ErrInvalid)
	}
	if email != "" && (len(email) > 320 || !strings.Contains(email, "@")) {
		return "", "", "", fmt.Errorf("%w: email", ErrInvalid)
	}
	return provider, subject, email, nil
}

// BindIdentity maps an external subject to the Battle.net player from the
// OAuth callback. Linking again moves the subject onto that player.
func (s *Store) BindIdentity(ctx context.Context, provider, subject, email, playerID string) error {
	if err := validUUID(playerID); err != nil {
		return err
	}
	var err error
	provider, subject, email, err = normalizeIdentity(provider, subject, email)
	if err != nil {
		return err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO identity_links (provider, subject, email, player_id)
		VALUES ($1,$2,$3,$4)
		ON CONFLICT (provider, subject) DO UPDATE
		SET email = EXCLUDED.email,
		    player_id = EXCLUDED.player_id,
		    updated_at = now()`, provider, subject, email, playerID)
	return mapPG(err)
}

// PlayerByIdentity returns the player linked to provider+subject.
// A newer email is stored. Email is not a lookup key.
func (s *Store) PlayerByIdentity(ctx context.Context, provider, subject, email string) (contract.Player, error) {
	var err error
	provider, subject, email, err = normalizeIdentity(provider, subject, email)
	if err != nil {
		return contract.Player{}, err
	}
	var p contract.Player
	var created time.Time
	var storedEmail string
	err = s.pool.QueryRow(ctx, `
		SELECT p.id::text, COALESCE(p.battletag, ''), p.display_name, COALESCE(p.bnet_account_id, ''), p.created_at, l.email
		FROM identity_links l
		JOIN players p ON p.id = l.player_id
		WHERE l.provider=$1 AND l.subject=$2`, provider, subject).Scan(
		&p.ID, &p.BattleTag, &p.DisplayName, &p.BnetAccountID, &created, &storedEmail)
	if errors.Is(err, pgx.ErrNoRows) {
		return contract.Player{}, ErrNotLinked
	}
	if err != nil {
		return contract.Player{}, err
	}
	if email != "" && email != storedEmail {
		if _, err := s.pool.Exec(ctx, `
			UPDATE identity_links SET email=$3, updated_at=now()
			WHERE provider=$1 AND subject=$2`, provider, subject, email); err != nil {
			return contract.Player{}, err
		}
	}
	scanPlayer(&p, created)
	return p, nil
}
