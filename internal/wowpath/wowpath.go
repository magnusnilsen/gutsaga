// Package wowpath finds a World of Warcraft flavor directory.
package wowpath

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// DefaultCandidates lists install locations for a GOOS. env is os.Getenv when nil.
func DefaultCandidates(goos string, env func(string) string) []string {
	if env == nil {
		env = os.Getenv
	}
	home := env("HOME")
	if home == "" {
		home = env("USERPROFILE")
	}
	switch goos {
	case "windows":
		pf86 := env("ProgramFiles(x86)")
		pf := env("ProgramFiles")
		local := env("LOCALAPPDATA")
		var roots []string
		for _, r := range []string{pf86, pf, local} {
			if r == "" {
				continue
			}
			roots = append(roots,
				filepath.Join(r, "World of Warcraft Forever"),
				filepath.Join(r, "World of Warcraft"),
			)
		}
		return roots
	default:
		var roots []string
		if home != "" {
			roots = append(roots,
				filepath.Join(home, "Applications", "World of Warcraft Forever"),
				filepath.Join(home, "Applications", "World of Warcraft"),
			)
		}
		roots = append(roots,
			"/Applications/World of Warcraft Forever",
			"/Applications/World of Warcraft",
		)
		return roots
	}
}

func Candidates() []string {
	return DefaultCandidates(runtime.GOOS, os.Getenv)
}

func isFlavor(path string) bool {
	info, err := os.Stat(filepath.Join(path, "Interface", "AddOns"))
	return err == nil && info.IsDir()
}

func score(path, client string) int {
	// Only the install folder and its parent count. A home directory named
	// "forever" must not beat an actual Forever client folder.
	base := strings.ToLower(filepath.Base(path))
	parent := strings.ToLower(filepath.Base(filepath.Dir(path)))
	blob := parent + "/" + base
	forever := strings.Contains(blob, "forever")
	retail := strings.Contains(blob, "_retail_") || strings.HasSuffix(base, "retail")
	if client == "retail" {
		if retail {
			return 100
		}
		if forever {
			return 10
		}
		return 1
	}
	if forever {
		return 100
	}
	if retail {
		return 10
	}
	return 1
}

func flavorOrder(client string) []string {
	if client == "retail" {
		return []string{"_retail_", "_forever_", "forever", "_classic_era_", "_classic_"}
	}
	return []string{"_forever_", "forever", "_retail_", "_classic_era_", "_classic_"}
}

// Resolve returns the flavor directory (the one that contains Interface/AddOns).
// A parent install folder is accepted; Forever is preferred over retail.
func Resolve(path string) (string, bool) {
	return resolve(path, "forever")
}

// ResolveClient is Resolve with an explicit client, "forever" or "retail".
func ResolveClient(path, client string) (string, bool) {
	return resolve(path, client)
}

func resolve(path, client string) (string, bool) {
	path = filepath.Clean(path)
	if isFlavor(path) {
		return path, true
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return "", false
	}
	for _, name := range flavorOrder(client) {
		candidate := filepath.Join(path, name)
		if isFlavor(candidate) {
			return candidate, true
		}
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return "", false
	}
	var found []string
	for _, e := range entries {
		candidate := filepath.Join(path, e.Name())
		if isFlavor(candidate) {
			found = append(found, candidate)
		}
	}
	if len(found) == 0 {
		return "", false
	}
	sort.Slice(found, func(i, j int) bool { return score(found[i], client) > score(found[j], client) })
	return found[0], true
}

// Detect uses configured when it points at a WoW directory, otherwise the first
// existing candidate, preferring a path that contains "forever".
func Detect(configured string, candidates []string) (string, error) {
	return DetectClient(configured, candidates, "forever")
}

// DetectClient prefers _retail_ when client is "retail", otherwise Forever.
func DetectClient(configured string, candidates []string, client string) (string, error) {
	if client != "retail" {
		client = "forever"
	}
	if strings.TrimSpace(configured) != "" {
		if p, ok := ResolveClient(configured, client); ok {
			return p, nil
		}
		return "", errors.New("wow_path peker ikke på en WoW-mappe med Interface/AddOns")
	}
	var found []string
	seen := map[string]bool{}
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if p, ok := ResolveClient(c, client); ok && !seen[p] {
			seen[p] = true
			found = append(found, p)
		}
	}
	if len(found) == 0 {
		return "", errors.New("fant ikke World of Warcraft. Sett --wow-path eller wow_path i config.json")
	}
	sort.SliceStable(found, func(i, j int) bool { return score(found[i], client) > score(found[j], client) })
	return found[0], nil
}

func AddonDir(wow string) string {
	return filepath.Join(wow, "Interface", "AddOns", "GutSaga")
}

func ChatLog(wow string) string { return filepath.Join(wow, "Logs", "WoWChatLog.txt") }

func Screenshots(wow string) string { return filepath.Join(wow, "Screenshots") }
