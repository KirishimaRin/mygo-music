package app

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"mygo-music/internal/i18n"
	"mygo-music/internal/listening"
)

// The calendar selects a day's plays, and a play opens the track's other days.
func TestListeningHistoryNavigation(t *testing.T) {
	a := newTestApp()
	ch := posts(a)
	h := &a.listening
	h.store = listening.Open(filepath.Join(t.TempDir(), "history.db"))
	defer h.store.Close()
	first := time.Date(2026, 11, 19, 0, 0, 0, 0, time.UTC)
	track := listening.Track{Source: "local", ID: "song"}
	h.store.Append(listening.Entry{ID: "first", Track: track, Title: "A test song", Started: first, Heard: time.Minute})
	h.store.Append(listening.Entry{ID: "second", Track: track, Title: "A test song", Started: first.AddDate(0, 0, 1), Heard: time.Minute})
	tt := ui.NewTester(a.view, 880, 580)
	click := func(label string) {
		t.Helper()
		if err := tt.Click(label); err != nil {
			t.Fatal(err)
		}
		until(t, ch, func() bool { return !h.loading })
		tt.Frame()
	}
	click(T("Listening History"))
	if a.route.page != pageListening {
		t.Fatalf("sidebar opened %+v", a.route)
	}
	a.showListeningDay(first.AddDate(0, 0, 1))
	until(t, ch, func() bool { return !h.loading })
	tt.Frame()

	click(i18n.Date(first))
	if h.day != first || len(h.view.Entries) != 1 || h.view.Entries[0].ID != "first" {
		t.Fatalf("day %v, entries %+v", h.day, h.view.Entries)
	}
	click("A test song")
	if h.track == nil || h.track.Track != track || len(h.view.Entries) != 2 || h.view.Entries[0].ID != "second" {
		t.Fatalf("track %+v, entries %+v", h.track, h.view.Entries)
	}
	click(T("Back to day"))
	if h.track != nil || h.day != first || len(h.view.Entries) != 1 {
		t.Fatalf("day %v, track %+v, entries %+v", h.day, h.track, h.view.Entries)
	}
	click(T("Previous month"))
	if h.day.Month() != time.October || len(h.view.Entries) != 0 {
		t.Fatalf("day %v, entries %+v", h.day, h.view.Entries)
	}
}
