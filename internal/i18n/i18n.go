// Package i18n is the companion and Discord catalog.
// English is the fallback. API error codes stay English in the API package.
package i18n

import "strings"

const (
	EN = "en"
	NB = "nb"
)

// Normalize accepts en, nb, and a few aliases. Anything else is English.
func Normalize(lang string) string {
	switch strings.ToLower(strings.TrimSpace(lang)) {
	case "nb", "no", "nb-no", "nb_no", "norwegian", "norsk":
		return NB
	default:
		return EN
	}
}

// T returns the catalog string. A missing key in nb uses English.
// A key missing from English is returned unchanged.
func T(lang, key string) string {
	lang = Normalize(lang)
	if lang == NB {
		if v, ok := nb[key]; ok {
			return v
		}
	}
	if v, ok := en[key]; ok {
		return v
	}
	return key
}

// Catalog returns a copy of one language, with English filled in for gaps.
func Catalog(lang string) map[string]string {
	lang = Normalize(lang)
	out := make(map[string]string, len(en))
	for k, v := range en {
		out[k] = v
	}
	if lang == NB {
		for k, v := range nb {
			out[k] = v
		}
	}
	return out
}

// NBKeys returns the Norwegian keys. Tests require each one to exist in English.
func NBKeys() map[string]string {
	out := make(map[string]string, len(nb))
	for k, v := range nb {
		out[k] = v
	}
	return out
}

func ENKeys() map[string]string {
	out := make(map[string]string, len(en))
	for k, v := range en {
		out[k] = v
	}
	return out
}

var en = map[string]string{
	"discord.someone":      "Someone",
	"discord.completed":    "completed a saga!",
	"discord.created":      "%s created a new saga",
	"discord.created_anon": "New saga",
	"discord.updated":      "%s updated a saga",
	"discord.updated_anon": "A saga was updated",
	"discord.retired":      "%s retired a saga",
	"discord.retired_anon": "A saga was retired",
	"discord.retired_body": "This saga is no longer active.",
	"discord.version":      "New version %d.",
	"discord.description":  "Description",
	"discord.ready":        "A new saga is ready.",
	"discord.level":        "reached a new level.",
	"discord.no_goals":     "No goals",
	"discord.goal":         "Goal",
	"discord.more_goals":   "… and %d more goals",
	"discord.more_checked": "… and %d more goals",

	"app.logged_out":     "Not logged in",
	"app.account":        "Account: %s",
	"app.login":          "Log in with Battle.net",
	"app.logout":         "Log out",
	"app.login_tip":      "Open the browser and log in",
	"app.logout_tip":     "Log out of SagaBoard on this computer",
	"app.sync":           "Sync now",
	"app.sync_tip":       "Fetch sagas from the API",
	"app.quit":           "Quit",
	"app.quit_tip":       "Quit SagaBoard",
	"app.language":       "Language: %s",
	"app.language_en":    "English",
	"app.language_nb":    "Norwegian",
	"app.mint_token":     "Create personal token",
	"app.mint_token_tip": "Show a gst_ token once for external tools",
	"app.revoke_token":   "Revoke personal token",
	"app.version":        "Version",
	"app.update_ready":   "New version %s — restart",
	"app.auto_update":    "Automatic updates",
	"app.running":        "SagaBoard is running. Log in with Battle.net if asked. Quit with Ctrl+C.",
	"app.synced":         "Synced. Type /reload in the game if you are logged in.",
	"app.api_required":   "--api-url or SAGABOARD_API_URL is required",

	"news.created":         "New saga: %s — type /reload in WoW",
	"news.created_addon":   "New saga: %s is loaded.",
	"news.updated":         "Saga updated: %s — type /reload in WoW",
	"news.updated_addon":   "Saga updated: %s is loaded.",
	"news.retired":         "Saga retired: %s — type /reload in WoW",
	"news.retired_addon":   "Saga retired: %s is loaded.",
	"news.completed":       "Saga completed: %s — type /reload in WoW",
	"news.completed_addon": "Saga completed: %s is loaded.",
	"news.extra":           " and %d more",

	"setup.wow":      "WoW folder (empty = search, _retail_ when the client is retail)",
	"setup.client":   "Client: forever, retail, classic_era, or classic",
	"setup.repo":     "GitHub repo for updates (owner/name)",
	"setup.token":    "GitHub token (empty if the repo is public)",
	"setup.auto":     "Automatic updates (yes/no)",
	"setup.language": "Language (en or nb)",
}

var nb = map[string]string{
	"discord.someone":      "En karakter",
	"discord.completed":    "fullførte en saga!",
	"discord.created":      "%s opprettet en ny saga",
	"discord.created_anon": "Ny saga",
	"discord.updated":      "%s oppdaterte en saga",
	"discord.updated_anon": "En saga er oppdatert",
	"discord.retired":      "%s la bort en saga",
	"discord.retired_anon": "En saga er lagt bort",
	"discord.retired_body": "Sagaen er ikke lenger aktiv.",
	"discord.version":      "Ny versjon %d.",
	"discord.description":  "Beskrivelse",
	"discord.ready":        "En ny saga er klar.",
	"discord.level":        "nådde et nytt nivå.",
	"discord.no_goals":     "Ingen mål",
	"discord.goal":         "Mål",
	"discord.more_goals":   "… og %d mål til",
	"discord.more_checked": "… og %d mål til",

	"app.logged_out":     "Ikke innlogget",
	"app.account":        "Konto: %s",
	"app.login":          "Logg inn med Battle.net",
	"app.logout":         "Logg ut",
	"app.login_tip":      "Åpne nettleseren og logg inn",
	"app.logout_tip":     "Logg ut av SagaBoard på denne maskinen",
	"app.sync":           "Synkroniser nå",
	"app.sync_tip":       "Hent sagaer fra API-et",
	"app.quit":           "Avslutt",
	"app.quit_tip":       "Avslutt SagaBoard",
	"app.language":       "Språk: %s",
	"app.language_en":    "Engelsk",
	"app.language_nb":    "Norsk",
	"app.mint_token":     "Lag personlig token",
	"app.mint_token_tip": "Vis et gst_-token én gang for eksterne verktøy",
	"app.revoke_token":   "Trekk tilbake personlig token",
	"app.version":        "Versjon",
	"app.auto_update":    "Automatiske oppdateringer",
	"app.running":        "SagaBoard kjører. Logg inn med Battle.net om du blir bedt om det. Avslutt med Ctrl+C.",
	"app.synced":         "Synkronisert. Skriv /reload i spillet hvis du er innlogget.",
	"app.api_required":   "--api-url eller SAGABOARD_API_URL må være satt",

	"news.created":         "Ny saga: %s — skriv /reload i WoW",
	"news.created_addon":   "Ny saga: %s er lastet inn.",
	"news.updated":         "Saga oppdatert: %s — skriv /reload i WoW",
	"news.updated_addon":   "Saga oppdatert: %s er lastet inn.",
	"news.retired":         "Saga avsluttet: %s — skriv /reload i WoW",
	"news.retired_addon":   "Saga avsluttet: %s er lastet inn.",
	"news.completed":       "Saga fullført: %s — skriv /reload i WoW",
	"news.completed_addon": "Saga fullført: %s er lastet inn.",
	"news.extra":           " og %d til",

	"setup.wow":      "Mappe til WoW (tom = let automatisk, _retail_ når klient er retail)",
	"setup.client":   "Klient: forever, retail, classic_era eller classic",
	"setup.repo":     "GitHub-repo for oppdateringer (eier/navn)",
	"setup.token":    "GitHub-token (tom hvis repoet er offentlig)",
	"setup.auto":     "Automatiske oppdateringer (ja/nei)",
	"setup.language": "Språk (en eller nb)",
}
