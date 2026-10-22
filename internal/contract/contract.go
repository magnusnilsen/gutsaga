// Package contract is the JSON API shared by the GutSaga API and the companion.
package contract

import "time"

// SyncResponse is GET /v1/sync. The companion merges quests onto characters;
// the server does not pre-filter eligibility (except that it omits inactive
// quests that this player has no progress on).
type SyncResponse struct {
	BattleTag   string            `json:"battletag"`
	GeneratedAt string            `json:"generated_at"`
	Characters  []Character       `json:"characters"`
	Quests      []Quest           `json:"quests"`
	Progress    []Progress        `json:"progress"`
	Completions []QuestCompletion `json:"completions,omitempty"`
}

// QuestCompletion is one character who has finished a saga. Sync includes
// every character in the database, not only the signed-in player.
type QuestCompletion struct {
	QuestID     string `json:"quest_id"`
	Character   string `json:"character"`
	Realm       string `json:"realm"`
	BattleTag   string `json:"battletag,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
	Level       int    `json:"level,omitempty"`
	Race        string `json:"race,omitempty"`
	Class       string `json:"class,omitempty"`
	CompletedAt string `json:"completed_at,omitempty"`
}

type Character struct {
	ID        string `json:"id"`
	PlayerID  string `json:"player_id,omitempty"`
	Name      string `json:"name"`
	Realm     string `json:"realm"`
	Level     int    `json:"level"`
	Race      string `json:"race"`
	Class     string `json:"class"`
	BattleTag string `json:"battletag"`
	Backstory string `json:"backstory,omitempty"`
}

type IDRef struct {
	ID string `json:"id"`
}

type Quest struct {
	ID                 string      `json:"id"`
	Title              string      `json:"title"`
	Description        string      `json:"description"`
	Objectives         []Objective `json:"objectives"`
	ObjectivesOrdered  bool        `json:"objectives_ordered"`
	MinLevel           *int        `json:"min_level"`
	Reward             string      `json:"reward"`
	PartyQuest         bool        `json:"party_quest"`
	AssignedTo         []IDRef     `json:"assigned_to"`
	CreatedBy          string      `json:"created_by"`
	Active             bool        `json:"active"`
	Version            int         `json:"version"`
	HasImage           bool        `json:"has_image"`
	ImageURL           string      `json:"image_url,omitempty"`
}

type Progress struct {
	QuestID       string `json:"quest_id"`
	CharacterID   string `json:"character_id"`
	QuestVersion  int    `json:"quest_version"`
	Status        string `json:"status"`
	CompletedAt   string `json:"completed_at,omitempty"`
	ScreenshotURL string `json:"screenshot_url,omitempty"`
}

// CompletionRequest is the JSON form of POST /v1/complete.
// screenshot_base64 is optional. Multipart uploads use the same fields plus a file part named "screenshot".
type CompletionRequest struct {
	CharacterID      string `json:"character_id"`
	QuestID          string `json:"quest_id"`
	QuestVersion     int    `json:"quest_version"`
	BattleTag        string `json:"battletag"`
	Level            int    `json:"level,omitempty"`
	CompletedAt      string `json:"completed_at,omitempty"`
	ScreenshotBase64 string `json:"screenshot_base64,omitempty"`
}

type CompletionResponse struct {
	Allowed          bool   `json:"allowed"`
	AlreadyCompleted bool   `json:"already_completed"`
	Completed        bool   `json:"completed"`
	ProgressID       string `json:"progress_id,omitempty"`
	LogWritten       bool   `json:"log_written"`
	DiscordPosted    bool   `json:"discord_posted"`
	CharacterName    string `json:"character_name,omitempty"`
	QuestTitle       string `json:"quest_title,omitempty"`
	QuestVersion     int    `json:"quest_version,omitempty"`
}

type LevelRequest struct {
	CharacterID string `json:"character_id"`
	BattleTag   string `json:"battletag"`
	Level       int    `json:"level"`
}

type LevelResponse struct {
	Allowed bool `json:"allowed"`
	Level   int  `json:"level"`
	Changed bool `json:"changed"`
}

type Player struct {
	ID          string `json:"id"`
	BattleTag   string `json:"battletag"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email,omitempty"`
	SmiaSubject string `json:"smia_subject,omitempty"`
	CreatedAt   string `json:"created_at"`
}

// SessionResponse is what the API returns after it has verified a Smia token.
// The session token is a GutSaga credential. It is not the Smia access token.
type SessionResponse struct {
	TokenType    string `json:"token_type"`
	SessionToken string `json:"session_token"`
	ExpiresAt    string `json:"expires_at"`
	Player       Player `json:"player"`
}

// CharacterUpsert is PUT /v1/characters. The companion sends what the addon read.
type CharacterUpsert struct {
	Name      string `json:"name"`
	Realm     string `json:"realm"`
	Level     int    `json:"level"`
	Race      string `json:"race"`
	Class     string `json:"class"`
	BattleTag string `json:"battletag"`
}

type CreatePlayerResponse struct {
	Player Player `json:"player"`
	APIKey string `json:"api_key"`
}

type QuestVersion struct {
	Version     int         `json:"version"`
	Title       string      `json:"title"`
	Description string      `json:"description"`
	Objectives  []Objective `json:"objectives"`
	Reward      string      `json:"reward"`
	MinLevel    *int        `json:"min_level"`
	PartyQuest  bool        `json:"party_quest"`
	Active      bool        `json:"active"`
	AssignedTo  []IDRef     `json:"assigned_to"`
	CreatedBy   string      `json:"created_by"`
	CreatedAt   string      `json:"created_at"`
}

type QuestDetail struct {
	Quest
	Versions []QuestVersion `json:"versions"`
}

// QuestListItem is an admin quest row. Status is null when character_id was
// requested and that character has not started the quest. Eligible is set only
// when character_id was requested.
type QuestListItem struct {
	Quest
	Status           *string `json:"status"`
	CompletedAt      string  `json:"completed_at,omitempty"`
	CompletedVersion int     `json:"completed_version,omitempty"`
	Eligible         *bool   `json:"eligible,omitempty"`
}

type LogEntry struct {
	ID          string `json:"id"`
	PlayerID    string `json:"player_id,omitempty"`
	CharacterID string `json:"character_id,omitempty"`
	QuestID     string `json:"quest_id,omitempty"`
	Type        string `json:"type"`
	Title       string `json:"title"`
	Body        string `json:"body"`
	CreatedAt   string `json:"created_at"`
}

type ErrorBody struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// Eligible reports whether a character should see a quest.
// A character sees a quest when it is active, min_level is null or <= the
// character's level, and either it is a party quest or assigned_to contains
// the character id.
func Eligible(level int, characterID string, quest Quest) bool {
	if !quest.Active {
		return false
	}
	if quest.MinLevel != nil && level < *quest.MinLevel {
		return false
	}
	if quest.PartyQuest {
		return true
	}
	for _, a := range quest.AssignedTo {
		if a.ID == characterID {
			return true
		}
	}
	return false
}

// FormatTime renders a UTC timestamp the way the API writes it.
func FormatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
