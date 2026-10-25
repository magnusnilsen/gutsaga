// Package addon embeds the OurSagas addon so the companion can install it.
package addon

import "embed"

//go:embed OurSagas
var Files embed.FS
