package contract

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Objective types the addon can verify, in order. A later objective is not
// checked until every earlier one is done. kill is experimental: the combat
// log may return secret values, and those hits are ignored.
const (
	ObjectiveLocation = "location"
	ObjectiveInteract = "interact"
	ObjectiveCollect  = "collect"
	ObjectiveKill     = "kill"
	objectiveLegacy   = "legacy"
)

// DefaultRadius is the map-fraction distance used when a location objective
// sets x and y but omits radius. Map coordinates are 0–1.
const DefaultRadius = 0.02

var objectiveID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,40}$`)

// Objective is one ordered step of a quest. LLMs author these through the
// admin API. The same array is snapshotted on every quest version.
type Objective struct {
	ID       string   `json:"id"`
	Type     string   `json:"type"`
	Text     string   `json:"text"`
	Zone     string   `json:"zone,omitempty"`
	Subzone  string   `json:"subzone,omitempty"`
	MapID    *int     `json:"map_id,omitempty"`
	X        *float64 `json:"x,omitempty"`
	Y        *float64 `json:"y,omitempty"`
	Radius   *float64 `json:"radius,omitempty"`
	NPC      string   `json:"npc,omitempty"`
	Object   string   `json:"object,omitempty"`
	ItemID   *int     `json:"item_id,omitempty"`
	ItemName string   `json:"item_name,omitempty"`
	Count    int      `json:"count,omitempty"`
}

// DecodeObjectives reads a jsonb value. Null and empty become an empty slice.
func DecodeObjectives(raw string) ([]Objective, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return []Objective{}, nil
	}
	var out []Objective
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = []Objective{}
	}
	return out, nil
}

// NormalizeObjectives checks an authored objective list and fills defaults.
// legacy is accepted only so a migrated text objective can be patched forward.
func NormalizeObjectives(in []Objective, allowLegacy bool) ([]Objective, error) {
	if len(in) == 0 {
		return nil, fmt.Errorf("objectives are required")
	}
	if len(in) > 20 {
		return nil, fmt.Errorf("at most 20 objectives")
	}
	seen := map[string]bool{}
	out := make([]Objective, len(in))
	for i, o := range in {
		o.ID = strings.TrimSpace(o.ID)
		o.Type = strings.TrimSpace(o.Type)
		o.Text = strings.TrimSpace(o.Text)
		o.Zone = strings.TrimSpace(o.Zone)
		o.Subzone = strings.TrimSpace(o.Subzone)
		o.NPC = strings.TrimSpace(o.NPC)
		o.Object = strings.TrimSpace(o.Object)
		o.ItemName = strings.TrimSpace(o.ItemName)
		if !objectiveID.MatchString(o.ID) {
			return nil, fmt.Errorf("objective %d: id must match [A-Za-z0-9_-]{1,40}", i+1)
		}
		if seen[o.ID] {
			return nil, fmt.Errorf("objective id %s is duplicated", o.ID)
		}
		seen[o.ID] = true
		if o.Text == "" || utf8.RuneCountInString(o.Text) > 300 {
			return nil, fmt.Errorf("objective %s: text is required (max 300)", o.ID)
		}
		if err := clipField(o.Zone, o.Subzone, o.NPC, o.Object, o.ItemName); err != nil {
			return nil, fmt.Errorf("objective %s: %w", o.ID, err)
		}
		switch o.Type {
		case ObjectiveLocation:
			if err := normalizeLocation(&o); err != nil {
				return nil, err
			}
		case ObjectiveInteract:
			if o.NPC == "" && o.Object == "" {
				return nil, fmt.Errorf("objective %s: interact needs npc and/or object", o.ID)
			}
		case ObjectiveCollect:
			if o.ItemID == nil || *o.ItemID <= 0 {
				return nil, fmt.Errorf("objective %s: collect needs item_id", o.ID)
			}
			if o.Count == 0 {
				o.Count = 1
			}
			if o.Count < 1 || o.Count > 10000 {
				return nil, fmt.Errorf("objective %s: count must be 1..10000", o.ID)
			}
		case ObjectiveKill:
			if o.NPC == "" {
				return nil, fmt.Errorf("objective %s: kill needs npc (the creature name, not a GUID)", o.ID)
			}
			if o.Count == 0 {
				o.Count = 1
			}
			if o.Count < 1 || o.Count > 1000 {
				return nil, fmt.Errorf("objective %s: count must be 1..1000", o.ID)
			}
		case objectiveLegacy:
			if !allowLegacy {
				return nil, fmt.Errorf("objective %s: type legacy is not authorable", o.ID)
			}
		default:
			return nil, fmt.Errorf("objective %s: type must be location, interact, collect, or kill", o.ID)
		}
		out[i] = o
	}
	return out, nil
}

func normalizeLocation(o *Objective) error {
	hasZone := o.Zone != "" || o.Subzone != "" || (o.MapID != nil && *o.MapID > 0)
	hasPoint := o.X != nil || o.Y != nil
	if o.MapID != nil && *o.MapID <= 0 {
		return fmt.Errorf("objective %s: map_id must be positive", o.ID)
	}
	if hasPoint && (o.X == nil || o.Y == nil) {
		return fmt.Errorf("objective %s: x and y are set together", o.ID)
	}
	if hasPoint {
		if *o.X < 0 || *o.X > 1 || *o.Y < 0 || *o.Y > 1 {
			return fmt.Errorf("objective %s: x and y are map fractions from 0 to 1", o.ID)
		}
		if o.Radius == nil {
			r := DefaultRadius
			o.Radius = &r
		}
		if *o.Radius <= 0 || *o.Radius > 1 {
			return fmt.Errorf("objective %s: radius must be within (0, 1]", o.ID)
		}
	}
	if !hasZone && !hasPoint {
		return fmt.Errorf("objective %s: location needs zone, subzone, map_id, or x/y", o.ID)
	}
	return nil
}

func clipField(values ...string) error {
	for _, v := range values {
		if utf8.RuneCountInString(v) > 80 {
			return fmt.Errorf("field longer than 80 characters")
		}
	}
	return nil
}
