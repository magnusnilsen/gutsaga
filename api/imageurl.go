package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/magnusnilsen/gutsaga/internal/contract"
)

// Minimal 1×1 transparent PNG for Smia get_quest when a saga has no cover.
var emptyPNG = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d,
	0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4, 0x89, 0x00, 0x00, 0x00,
	0x0a, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
	0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00, 0x00, 0x00, 0x00, 0x49,
	0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
}

func serveEmptyPNG(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		_, _ = w.Write(emptyPNG)
	}
}

func (s *Server) imageTTL() time.Duration {
	if s.imageURLTTL <= 0 {
		return time.Hour
	}
	return s.imageURLTTL
}

func (s *Server) withSignedCoverURL(q contract.Quest) contract.Quest {
	if !q.HasImage {
		return q
	}
	if u := s.signedSagaImageURL(q.ID, time.Now()); u != "" {
		q.ImageSignedURL = u
	}
	return q
}

// signedSagaImageURL is empty unless both the public origin and the HMAC
// secret are set. The path is the only public cover endpoint.
func (s *Server) signedSagaImageURL(id string, now time.Time) string {
	base := strings.TrimRight(strings.TrimSpace(s.publicBaseURL), "/")
	secret := s.imageURLSecret
	if base == "" || secret == "" || id == "" {
		return ""
	}
	exp := strconv.FormatInt(now.Add(s.imageTTL()).Unix(), 10)
	sig := signImage(secret, id, exp)
	return base + "/v1/public/sagas/" + id + "/image?exp=" + exp + "&sig=" + sig
}

func signImage(secret, id, exp string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(id + "\n" + exp))
	return hex.EncodeToString(mac.Sum(nil))
}

func validImageSig(secret, id, exp, sig string) bool {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(id + "\n" + exp))
	want := mac.Sum(nil)
	got, err := hex.DecodeString(sig)
	if err != nil || len(got) != len(want) {
		return false
	}
	return hmac.Equal(want, got)
}

// getPublicSagaImage serves cover bytes to clients that cannot send a
// bearer token (markdown images). A bad signature is rejected before any
// database lookup, so a guess does not reveal whether the saga exists.
func (s *Server) getPublicSagaImage(w http.ResponseWriter, r *http.Request) {
	if s.imageURLSecret == "" {
		writeErr(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	id := r.PathValue("id")
	expStr := r.URL.Query().Get("exp")
	sig := r.URL.Query().Get("sig")
	if !validImageSig(s.imageURLSecret, id, expStr, sig) {
		writeErr(w, http.StatusForbidden, "forbidden", "bad signature")
		return
	}
	exp, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil || time.Now().Unix() >= exp {
		writeErr(w, http.StatusForbidden, "forbidden", "expired")
		return
	}
	ct, image, err := s.store.QuestImage(r.Context(), id)
	if err != nil {
		if errors.Is(err, ErrNotFound) || errors.Is(err, ErrInvalid) {
			writeErr(w, http.StatusNotFound, "not_found", "not found")
			return
		}
		s.fail(w, err)
		return
	}
	if ct == "" {
		ct = "application/octet-stream"
	}
	remain := exp - time.Now().Unix()
	if remain < 0 {
		remain = 0
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "public, max-age="+strconv.FormatInt(remain, 10))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(image)
}
