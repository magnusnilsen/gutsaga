package syncdata

import (
	"sort"
	"strings"

	"github.com/magnusnilsen/gutsaga/internal/contract"
)

// Entry is one quest as a character should see it in the addon.
type Entry struct {
	Character    string               `json:"character"`
	CharacterID  string               `json:"character_id"`
	Realm        string               `json:"realm"`
	QuestID      string               `json:"quest_id"`
	QuestVersion int                  `json:"quest_version"`
	Title        string               `json:"title"`
	Description  string               `json:"description"`
	Objectives   []contract.Objective `json:"objectives"`
	Reward       string               `json:"reward"`
	MinLevel     *int                 `json:"min_level"`
	Status       string               `json:"status"`
	CompletedAt  string               `json:"completed_at"`
	PartyQuest   bool                 `json:"party_quest"`
	CreatedBy    string               `json:"created_by"`
	CompletedBy  []Completer          `json:"completed_by"`
}

// Completer is a character who has finished this saga.
type Completer struct {
	Character   string `json:"character"`
	Realm       string `json:"realm"`
	BattleTag   string `json:"battletag"`
	CompletedAt string `json:"completed_at"`
}

// Merge applies the companion rule: a character sees an active quest when
// min_level is null or <= their level, and the quest is a party quest or
// assigned to them. A completed quest stays visible even if it later becomes
// inactive or the character would no longer be eligible.
func Merge(sync contract.SyncResponse) []Entry {
	progress := map[string]contract.Progress{}
	for _, p := range sync.Progress {
		progress[p.CharacterID+"\n"+p.QuestID] = p
	}
	completers := completersByQuest(sync)
	var out []Entry
	for _, ch := range sync.Characters {
		for _, q := range sync.Quests {
			p, has := progress[ch.ID+"\n"+q.ID]
			completed := has && p.Status == "completed"
			if !contract.Eligible(ch.Level, ch.ID, q) && !completed {
				continue
			}
			e := Entry{
				Character: ch.Name, CharacterID: ch.ID, Realm: ch.Realm,
				QuestID: q.ID, QuestVersion: q.Version, Title: q.Title,
				Description: q.Description, Objectives: q.Objectives, Reward: q.Reward,
				MinLevel: q.MinLevel, PartyQuest: q.PartyQuest,
				CreatedBy: q.CreatedBy, CompletedBy: completers[q.ID],
			}
			if has {
				e.Status = p.Status
				e.CompletedAt = p.CompletedAt
				if completed && p.QuestVersion > 0 {
					e.QuestVersion = p.QuestVersion
				}
			}
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Character != out[j].Character {
			return out[i].Character < out[j].Character
		}
		ic := out[i].Status == "completed"
		jc := out[j].Status == "completed"
		if ic != jc {
			return !ic
		}
		li, lj := 0, 0
		if out[i].MinLevel != nil {
			li = *out[i].MinLevel
		}
		if out[j].MinLevel != nil {
			lj = *out[j].MinLevel
		}
		if li != lj {
			return li < lj
		}
		return out[i].Title < out[j].Title
	})
	return out
}

// FindCharacter matches name and, when both sides have a realm, the realm.
func completersByQuest(sync contract.SyncResponse) map[string][]Completer {
	type key struct{ quest, name, realm string }
	seen := map[key]bool{}
	out := map[string][]Completer{}
	add := func(questID, name, realm, tag, at string) {
		name = strings.TrimSpace(name)
		realm = strings.TrimSpace(realm)
		tag = strings.TrimSpace(tag)
		if questID == "" || name == "" {
			return
		}
		k := key{questID, strings.ToLower(name), strings.ToLower(realm)}
		if seen[k] {
			list := out[questID]
			for i := range list {
				if !strings.EqualFold(list[i].Character, name) || !strings.EqualFold(list[i].Realm, realm) {
					continue
				}
				if list[i].BattleTag == "" && tag != "" {
					list[i].BattleTag = tag
				}
				if list[i].CompletedAt == "" && at != "" {
					list[i].CompletedAt = at
				}
				break
			}
			return
		}
		seen[k] = true
		out[questID] = append(out[questID], Completer{
			Character: name, Realm: realm, BattleTag: tag, CompletedAt: at,
		})
	}
	known := map[string]contract.Character{}
	for _, ch := range sync.Characters {
		known[ch.ID] = ch
	}
	for _, p := range sync.Progress {
		if p.Status != "completed" {
			continue
		}
		ch := known[p.CharacterID]
		add(p.QuestID, ch.Name, ch.Realm, ch.BattleTag, p.CompletedAt)
	}
	for _, c := range sync.Completions {
		add(c.QuestID, c.Character, c.Realm, c.BattleTag, c.CompletedAt)
	}
	for id := range out {
		sort.SliceStable(out[id], func(i, j int) bool {
			return out[id][i].CompletedAt < out[id][j].CompletedAt
		})
	}
	return out
}

func FindCharacter(sync contract.SyncResponse, name, realm string) (contract.Character, bool) {
	var matches []contract.Character
	for _, c := range sync.Characters {
		if !strings.EqualFold(c.Name, name) {
			continue
		}
		if realm != "" && c.Realm != "" && !strings.EqualFold(c.Realm, realm) {
			continue
		}
		matches = append(matches, c)
	}
	if len(matches) == 1 {
		return matches[0], true
	}
	return contract.Character{}, false
}

func QuestByID(sync contract.SyncResponse, id string) (contract.Quest, bool) {
	for _, q := range sync.Quests {
		if q.ID == id {
			return q, true
		}
	}
	return contract.Quest{}, false
}
