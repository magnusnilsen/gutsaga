package discord

import "strings"

// Official Blizzard RAID_CLASS_COLORS as Discord embed hex values.
var classColors = map[string]int{
	"WARRIOR":     0xC79C6E,
	"PALADIN":     0xF58CBA,
	"HUNTER":      0xABD473,
	"ROGUE":       0xFFF569,
	"PRIEST":      0xFFFFFF,
	"DEATHKNIGHT": 0xC41E3A,
	"SHAMAN":      0x0070DD,
	"MAGE":        0x3FC7EB,
	"WARLOCK":     0x8788EE,
	"MONK":        0x00FF98,
	"DRUID":       0xFF7D0A,
	"DEMONHUNTER": 0xA330C9,
	"EVOKER":      0x33937F,
}

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

// ClassColor is the Blizzard class color for a class name or file token.
// ok is false when the class is unknown.
func ClassColor(class string) (color int, ok bool) {
	color, ok = classColors[classKey(class)]
	return color, ok
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
