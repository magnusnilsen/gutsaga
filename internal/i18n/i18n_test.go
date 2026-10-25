package i18n

import "testing"

func TestNorwegianKeysExistInEnglish(t *testing.T) {
	en := ENKeys()
	for key := range NBKeys() {
		if _, ok := en[key]; !ok {
			t.Fatalf("nb key %q is missing from en", key)
		}
	}
}

func TestMissingKeyFallsBackToEnglish(t *testing.T) {
	if got := T("nb", "discord.created_anon"); got != "Ny saga" {
		t.Fatalf("nb %q", got)
	}
	// A key present only in English is used for nb too.
	en["test.only_en"] = "English fallback"
	t.Cleanup(func() { delete(en, "test.only_en") })
	if got := T("nb", "test.only_en"); got != "English fallback" {
		t.Fatalf("fallback %q", got)
	}
	if got := T("de", "discord.no_goals"); got != "No goals" {
		t.Fatalf("unknown lang %q", got)
	}
}
