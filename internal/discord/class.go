package discord

import "strings"

// English display names for UnitClass file tokens (and Title Case inputs).
var classDisplay = map[string]string{
	"WARRIOR":     "Warrior",
	"PALADIN":     "Paladin",
	"HUNTER":      "Hunter",
	"ROGUE":       "Rogue",
	"PRIEST":      "Priest",
	"DEATHKNIGHT": "Death Knight",
	"SHAMAN":      "Shaman",
	"MAGE":        "Mage",
	"WARLOCK":     "Warlock",
	"MONK":        "Monk",
	"DRUID":       "Druid",
	"DEMONHUNTER": "Demon Hunter",
	"EVOKER":      "Evoker",
}

func classKey(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range strings.ToUpper(s) {
		if r >= 'A' && r <= 'Z' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// DisplayClass turns HUNTER / hunter into Hunter. Unknown values are returned trimmed.
func DisplayClass(class string) string {
	class = strings.TrimSpace(class)
	if class == "" {
		return ""
	}
	if name, ok := classDisplay[classKey(class)]; ok {
		return name
	}
	return class
}
