//go:build !darwin && !windows

package tray

import "context"

type Item struct {
	Title   string
	Tooltip string
	OnClick func()
}

func Supported() bool { return false }

// Run waits until ctx is cancelled. Linux builds have no tray icon.
func Run(ctx context.Context, _ string, _ []Item) {
	<-ctx.Done()
}
