//go:build darwin || windows

package tray

import (
	"context"

	"github.com/getlantern/systray"
)

type Item struct {
	Title   string
	Tooltip string
	OnClick func()
}

func Supported() bool { return true }

// Run shows the tray menu and blocks until ctx is cancelled or the user quits.
func Run(ctx context.Context, tooltip string, items []Item) {
	systray.Run(func() {
		systray.SetIcon(Icon())
		systray.SetTooltip(tooltip)
		for _, it := range items {
			menu := systray.AddMenuItem(it.Title, it.Tooltip)
			it := it
			go func() {
				for range menu.ClickedCh {
					if it.OnClick != nil {
						it.OnClick()
					}
				}
			}()
		}
		go func() {
			<-ctx.Done()
			systray.Quit()
		}()
	}, func() {})
}
