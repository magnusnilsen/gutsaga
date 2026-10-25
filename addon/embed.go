// Package addon embeds the SagaBoard addon so the companion can install it.
package addon

import "embed"

//go:embed SagaBoard
var Files embed.FS
