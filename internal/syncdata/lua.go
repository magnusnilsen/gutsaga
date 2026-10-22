package syncdata

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/magnusnilsen/gutsaga/internal/contract"
)

// WriteLua renders GutSagaData.lua. The addon reads this file at load /reload.
func WriteLua(battletag, generatedAt string, entries []Entry) string {
	var b strings.Builder
	b.WriteString("-- Generert av GutSaga-programmet. Ikke rediger for hånd.\n")
	b.WriteString("-- Last inn på nytt med /reload for å se nye oppdrag.\n")
	b.WriteString("GutSagaData = {\n")
	b.WriteString("  version = 1,\n")
	fmt.Fprintf(&b, "  battletag = %s,\n", luaString(battletag))
	fmt.Fprintf(&b, "  generated_at = %s,\n", luaString(generatedAt))
	b.WriteString("  entries = {\n")
	for _, e := range entries {
		b.WriteString("    {\n")
		fmt.Fprintf(&b, "      character = %s,\n", luaString(e.Character))
		fmt.Fprintf(&b, "      character_id = %s,\n", luaString(e.CharacterID))
		fmt.Fprintf(&b, "      realm = %s,\n", luaString(e.Realm))
		fmt.Fprintf(&b, "      quest_id = %s,\n", luaString(e.QuestID))
		fmt.Fprintf(&b, "      quest_version = %d,\n", e.QuestVersion)
		fmt.Fprintf(&b, "      title = %s,\n", luaString(e.Title))
		fmt.Fprintf(&b, "      description = %s,\n", luaString(e.Description))
		fmt.Fprintf(&b, "      objectives = %s,\n", luaObjectives(e.Objectives))
		fmt.Fprintf(&b, "      reward = %s,\n", luaString(e.Reward))
		if e.MinLevel == nil {
			b.WriteString("      min_level = nil,\n")
		} else {
			fmt.Fprintf(&b, "      min_level = %d,\n", *e.MinLevel)
		}
		fmt.Fprintf(&b, "      status = %s,\n", luaString(e.Status))
		fmt.Fprintf(&b, "      completed_at = %s,\n", luaString(e.CompletedAt))
		if e.PartyQuest {
			b.WriteString("      party_quest = true,\n")
		} else {
			b.WriteString("      party_quest = false,\n")
		}
		fmt.Fprintf(&b, "      created_by = %s,\n", luaString(e.CreatedBy))
		fmt.Fprintf(&b, "      completed_by = %s,\n", luaCompleters(e.CompletedBy))
		b.WriteString("    },\n")
	}
	b.WriteString("  },\n}\n")
	return b.String()
}

func luaObjectives(list []contract.Objective) string {
	if len(list) == 0 {
		return "{}"
	}
	var b strings.Builder
	b.WriteString("{\n")
	for _, o := range list {
		b.WriteString("        {\n")
		fmt.Fprintf(&b, "          id = %s,\n", luaString(o.ID))
		fmt.Fprintf(&b, "          type = %s,\n", luaString(o.Type))
		fmt.Fprintf(&b, "          text = %s,\n", luaString(o.Text))
		writeLuaString(&b, "zone", o.Zone)
		writeLuaString(&b, "subzone", o.Subzone)
		writeLuaString(&b, "npc", o.NPC)
		writeLuaString(&b, "object", o.Object)
		writeLuaString(&b, "item_name", o.ItemName)
		if o.MapID != nil {
			fmt.Fprintf(&b, "          map_id = %d,\n", *o.MapID)
		}
		writeLuaFloat(&b, "x", o.X)
		writeLuaFloat(&b, "y", o.Y)
		writeLuaFloat(&b, "radius", o.Radius)
		if o.ItemID != nil {
			fmt.Fprintf(&b, "          item_id = %d,\n", *o.ItemID)
		}
		if o.Count > 0 {
			fmt.Fprintf(&b, "          count = %d,\n", o.Count)
		}
		b.WriteString("        },\n")
	}
	b.WriteString("      }")
	return b.String()
}

func luaCompleters(list []Completer) string {
	if len(list) == 0 {
		return "{}"
	}
	var b strings.Builder
	b.WriteString("{\n")
	for _, c := range list {
		b.WriteString("        { ")
		fmt.Fprintf(&b, "character = %s, realm = %s, completed_at = %s", luaString(c.Character), luaString(c.Realm), luaString(c.CompletedAt))
		b.WriteString(" },\n")
	}
	b.WriteString("      }")
	return b.String()
}

func writeLuaString(b *strings.Builder, key, value string) {
	if value == "" {
		return
	}
	fmt.Fprintf(b, "          %s = %s,\n", key, luaString(value))
}

func writeLuaFloat(b *strings.Builder, key string, value *float64) {
	if value == nil {
		return
	}
	fmt.Fprintf(b, "          %s = %s,\n", key, strconv.FormatFloat(*value, 'f', -1, 64))
}

func luaString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 32 {
				fmt.Fprintf(&b, "\\%d", r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}
