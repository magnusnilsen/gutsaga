//go:build darwin || windows

package tray

import (
	"context"
	"fmt"
	"sync"

	"github.com/getlantern/systray"
)

const charSlots = 6

type Item struct {
	Title     string
	Tooltip   string
	Disabled  bool
	Separator bool
	OnClick   func()
}

// Status is the disabled account block at the top of the tray menu.
type Status struct {
	User       string
	BattleTag  string
	Characters []string
}

// Menu updates the disabled account lines and the login/logout action.
type Menu struct {
	mu         sync.Mutex
	user       *systray.MenuItem
	tag        *systray.MenuItem
	emptyChars *systray.MenuItem
	chars      []*systray.MenuItem
	auth       *systray.MenuItem
	authClick  func()
}

func Supported() bool { return true }

// Run shows the tray menu and blocks until ctx is cancelled or the user quits.
// onReady runs on the systray goroutine once the menu exists; use it to push
// the first Status and wire later updates via SetStatus / SetAuthAction.
// items are the actions below login/logout (sync, quit, …).
func Run(ctx context.Context, tooltip string, items []Item, onReady func(*Menu)) {
	systray.Run(func() {
		systray.SetIcon(Icon())
		systray.SetTooltip(tooltip)

		m := &Menu{
			user:       addDisabled("Ikke innlogget", "Smia-konto"),
			tag:        addDisabled("BattleTag: —", "BattleTag knyttet til spilleren"),
			emptyChars: addDisabled("Karakterer: ingen", "Karakterer synket fra API-et"),
		}
		for i := 0; i < charSlots; i++ {
			it := addDisabled("—", "Karakter")
			it.Hide()
			m.chars = append(m.chars, it)
		}
		systray.AddSeparator()

		m.auth = systray.AddMenuItem("Logg inn med Smia", "Åpne nettleseren og logg inn")
		go func() {
			for range m.auth.ClickedCh {
				m.mu.Lock()
				fn := m.authClick
				m.mu.Unlock()
				if fn != nil {
					fn()
				}
			}
		}()

		for _, it := range items {
			if it.Separator {
				systray.AddSeparator()
				continue
			}
			menu := systray.AddMenuItem(it.Title, it.Tooltip)
			if it.Disabled {
				menu.Disable()
			}
			it := it
			if it.OnClick == nil {
				continue
			}
			go func() {
				for range menu.ClickedCh {
					it.OnClick()
				}
			}()
		}

		if onReady != nil {
			onReady(m)
		}

		go func() {
			<-ctx.Done()
			systray.Quit()
		}()
	}, func() {})
}

func addDisabled(title, tip string) *systray.MenuItem {
	it := systray.AddMenuItem(title, tip)
	it.Disable()
	return it
}

// SetAuthAction sets the login/logout item title and click handler.
func (m *Menu) SetAuthAction(loggedIn bool, onClick func()) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.authClick = onClick
	if m.auth == nil {
		return
	}
	if loggedIn {
		m.auth.SetTitle("Logg ut")
		m.auth.SetTooltip("Logg ut av Smia på denne maskinen")
		return
	}
	m.auth.SetTitle("Logg inn med Smia")
	m.auth.SetTooltip("Åpne nettleseren og logg inn")
}

// SetStatus refreshes the grayed-out account block.
func (m *Menu) SetStatus(s Status) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	user := stringsOr(s.User, "Ikke innlogget")
	m.user.SetTitle(user)
	m.user.SetTooltip(user)

	tag := stringsOr(s.BattleTag, "BattleTag: —")
	m.tag.SetTitle(tag)
	m.tag.SetTooltip(tag)

	if len(s.Characters) == 0 {
		m.emptyChars.SetTitle("Karakterer: ingen")
		m.emptyChars.Show()
		for _, c := range m.chars {
			c.Hide()
		}
		return
	}

	m.emptyChars.Hide()
	shown := s.Characters
	if len(shown) > charSlots {
		extra := len(shown) - (charSlots - 1)
		shown = append(append([]string{}, s.Characters[:charSlots-1]...), fmt.Sprintf("…og %d til", extra))
	}
	for i, slot := range m.chars {
		if i < len(shown) {
			slot.SetTitle(shown[i])
			slot.SetTooltip(shown[i])
			slot.Show()
		} else {
			slot.Hide()
		}
	}
}

func stringsOr(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
