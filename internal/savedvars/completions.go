// Package savedvars reads Sagalore completions from WoW SavedVariables.
// Classic Era does not write DEFAULT_CHAT_FRAME:AddMessage into WoWChatLog.txt,
// so the companion also polls these files for COMPLETE events.
package savedvars

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/magnusnilsen/gutsaga/internal/contract"
)

// Completion is one local finish stored by the addon under SagaloreDB.completions.
type Completion struct {
	QuestID      string
	QuestVersion int
	Character    string
	Realm        string
	Race         string
	Class        string
	Level        int
	At           time.Time
	Path         string
	Instance     string
	GroupMembers []contract.GroupMember
}

var (
	completionBlock = regexp.MustCompile(`(?s)\["([0-9a-fA-F-]{36})"\]\s*=\s*\{(.*?)\},`)
	fieldString     = regexp.MustCompile(`\["(saga_id|character|realm|race|class|group_members|instance)"\]\s*=\s*"([^"]*)"`)
	fieldNumber     = regexp.MustCompile(`\["(version|level|completed_at)"\]\s*=\s*(-?\d+)`)
)

// ListSagaloreFiles returns SavedVariables files for this flavor.
// Sagalore.lua is preferred. OurSagas.lua is included only when that character
// does not yet have a Sagalore.lua, so a copy that has not run is still read
// and a migrated character is not imported twice.
func ListSagaloreFiles(wowDir string) ([]string, error) {
	root := filepath.Join(wowDir, "WTF", "Account")
	var current, legacy []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() || !inSavedVariables(path) {
			return nil
		}
		switch {
		case strings.EqualFold(d.Name(), "Sagalore.lua"):
			current = append(current, path)
		case strings.EqualFold(d.Name(), "OurSagas.lua"):
			legacy = append(legacy, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	have := map[string]bool{}
	for _, path := range current {
		have[filepath.Dir(path)] = true
	}
	out := append([]string{}, current...)
	for _, path := range legacy {
		if !have[filepath.Dir(path)] {
			out = append(out, path)
		}
	}
	return out, nil
}

func inSavedVariables(path string) bool {
	sep := string(filepath.Separator) + "SavedVariables" + string(filepath.Separator)
	return strings.Contains(path, sep)
}

// ParseCompletions extracts SagaloreDB.completions entries from one SavedVariables file.
func ParseCompletions(path string, body string) []Completion {
	start := strings.Index(body, `["completions"]`)
	if start < 0 {
		start = strings.Index(body, "completions")
	}
	if start < 0 {
		return nil
	}
	body = body[start:]
	end := strings.Index(body, `["objectives"]`)
	if end < 0 {
		end = strings.Index(body, `["sections"]`)
	}
	if end > 0 {
		body = body[:end]
	}
	var out []Completion
	for _, m := range completionBlock.FindAllStringSubmatch(body, -1) {
		block := m[2]
		c := Completion{QuestID: m[1], Path: path, Level: 1}
		for _, f := range fieldString.FindAllStringSubmatch(block, -1) {
			switch f[1] {
			case "saga_id":
				if f[2] != "" {
					c.QuestID = f[2]
				}
			case "character":
				c.Character = f[2]
			case "realm":
				c.Realm = f[2]
			case "race":
				c.Race = f[2]
			case "class":
				c.Class = f[2]
			case "instance":
				c.Instance = f[2]
			case "group_members":
				c.GroupMembers = parseMemberBlob(f[2])
			}
		}
		for _, f := range fieldNumber.FindAllStringSubmatch(block, -1) {
			n, err := strconv.ParseInt(f[2], 10, 64)
			if err != nil {
				continue
			}
			switch f[1] {
			case "version":
				c.QuestVersion = int(n)
			case "level":
				c.Level = int(n)
			case "completed_at":
				if n > 0 {
					c.At = time.Unix(n, 0).UTC()
				}
			}
		}
		if c.QuestID == "" || c.Character == "" {
			continue
		}
		if c.At.IsZero() {
			c.At = time.Now().UTC()
		}
		if c.Level < 1 {
			c.Level = 1
		}
		out = append(out, c)
	}
	return out
}

func parseMemberBlob(s string) []contract.GroupMember {
	if s == "" {
		return nil
	}
	var out []contract.GroupMember
	for _, part := range strings.Split(s, ";") {
		bits := strings.Split(part, "/")
		if len(bits) != 3 || strings.TrimSpace(bits[0]) == "" {
			continue
		}
		level, err := strconv.Atoi(bits[2])
		if err != nil || level < 0 {
			level = 0
		}
		out = append(out, contract.GroupMember{Name: bits[0], Realm: bits[1], Level: level})
	}
	return out
}

// CopyObjectiveChecks copies in-progress objective checks from an older
// SavedVariables file (GutSaga or SagaBoard) into Sagalore once, when the new file has none.
func CopyObjectiveChecks(wowDir string) error {
	root := filepath.Join(wowDir, "WTF", "Account")
	if _, err := os.Stat(root); err != nil {
		return nil
	}
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return nil
		}
		name := d.Name()
		if !strings.EqualFold(name, "GutSaga.lua") && !strings.EqualFold(name, "SagaBoard.lua") {
			return nil
		}
		if !strings.Contains(path, string(filepath.Separator)+"SavedVariables"+string(filepath.Separator)) {
			return nil
		}
		oldBody, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		start := strings.Index(string(oldBody), `["objectives"]`)
		if start < 0 {
			return nil
		}
		next := filepath.Join(filepath.Dir(path), "Sagalore.lua")
		cur, _ := os.ReadFile(next)
		if strings.Contains(string(cur), "objectives_copied") || (strings.Contains(string(cur), `["objectives"]`) && !strings.Contains(string(cur), `["objectives"] = {}`)) {
			return nil
		}
		snippet := string(oldBody)[start:]
		marker := "\nSagaloreDB = SagaloreDB or {}\nSagaloreDB.objectives_copied = true\n"
		if err := os.WriteFile(next, append(cur, []byte(marker+snippet)...), 0o644); err != nil {
			return err
		}
		return nil
	})
}

// MigrateOurSagas copies SavedVariables/OurSagas.lua onto Sagalore.lua and
// renames the OurSagasDB global. WoW names the file after the addon folder, so
// the old file is not loaded once the addon is Sagalore. A Sagalore.lua that
// already holds progress is left alone.
func MigrateOurSagas(wowDir string) error {
	root := filepath.Join(wowDir, "WTF", "Account")
	if _, err := os.Stat(root); err != nil {
		return nil
	}
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() {
			return nil
		}
		if !strings.EqualFold(d.Name(), "OurSagas.lua") || !inSavedVariables(path) {
			return nil
		}
		srcBody, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		dest := filepath.Join(filepath.Dir(path), "Sagalore.lua")
		destBody, destErr := os.ReadFile(dest)
		if destErr != nil && !os.IsNotExist(destErr) {
			return destErr
		}
		if !shouldCopySavedVars(string(srcBody), string(destBody), destErr == nil) {
			return nil
		}
		rewritten := strings.ReplaceAll(string(srcBody), "OurSagasDB", "SagaloreDB")
		return os.WriteFile(dest, []byte(rewritten), 0o644)
	})
}

func shouldCopySavedVars(src, dst string, dstExists bool) bool {
	if strings.TrimSpace(src) == "" {
		return false
	}
	if !dstExists {
		return true
	}
	if hasProgress(dst) {
		return false
	}
	return hasProgress(src)
}

func hasProgress(body string) bool {
	if len(ParseCompletions("migrate", body)) > 0 || len(ParseResets("migrate", body)) > 0 || len(ParsePendingInvites("migrate", body)) > 0 {
		return true
	}
	if strings.Contains(body, `["seen_notice"]`) {
		return true
	}
	return objectivesHaveData(body)
}

func objectivesHaveData(body string) bool {
	i := strings.Index(body, `["objectives"]`)
	if i < 0 {
		return false
	}
	rest := body[i:]
	return strings.Contains(rest, `["steps"]`) || strings.Contains(rest, `["done"]`)
}

// ReadCompletions loads completions from every Sagalore SavedVariables file under wowDir.
func ReadCompletions(wowDir string) ([]Completion, error) {
	files, err := ListSagaloreFiles(wowDir)
	if err != nil {
		return nil, err
	}
	var out []Completion
	for _, path := range files {
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		out = append(out, ParseCompletions(path, string(b))...)
	}
	return out, nil
}
