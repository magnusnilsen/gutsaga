// Package envcompat reads SAGALORE_* settings and the OURSAGAS_* names they replaced.
package envcompat

import (
	"os"
	"strings"
)

// Get returns the trimmed value of name. When name starts with SAGALORE_ and
// that variable is empty, the matching OURSAGAS_ variable is used so existing
// hosts and shells keep working after the rename.
func Get(name string) string {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		return v
	}
	const prefix = "SAGALORE_"
	if strings.HasPrefix(name, prefix) {
		return strings.TrimSpace(os.Getenv("OURSAGAS_" + strings.TrimPrefix(name, prefix)))
	}
	return ""
}
