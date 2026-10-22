// Package version is set with -ldflags at release time.
package version

// Version is the companion build. Release builds replace it.
var Version = "0.1.0-dev"

// DefaultAPIBaseURL is baked into the companion. Override with --api-url or
// GUTSAGA_API_URL. -ldflags can replace this when the Railway hostname exists.
var DefaultAPIBaseURL = "https://gutsaga.up.railway.app"
