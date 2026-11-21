package app

import (
	"log"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mygo-music/internal/listening"
	"mygo-music/internal/player"
)

const listeningPageSize = 100

type listeningState struct {
	store   *listening.Store
	day     time.Time
	track   *listening.Entry
	view    listening.View
	limit   int
	request uint64
	loading bool
	failed  bool
}

func (a *app) openListeningStore() {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		log.Printf("finding listening history directory: %v", err)
		return
	}
	a.listening.store = listening.Open(filepath.Join(dir, "listening.sqlite3"))
	a.pl.Listened = a.storeListen
}

func (a *app) closeListeningStore() {
	if s := a.listening.store; s != nil {
		s.Close()
	}
}

func (a *app) storeListen(l player.Listen) {
	e := listening.Entry{
		ID: l.ID, Track: listening.Track{Source: l.Track.Source, ID: l.Track.ID},
		Title: l.Track.Name, Artists: l.Track.ArtistNames(),
		Started: l.Started, Ended: l.Ended, Heard: l.Heard, Reason: string(l.Reason),
	}
	h := &a.listening
	h.store.Append(e)
	if a.route.page == pageListening {
		a.readListening()
	}
}

func (a *app) showListeningDay(day time.Time) {
	h := &a.listening
	if h.day.Year() != day.Year() || h.day.Month() != day.Month() {
		h.view.Days = [32]bool{}
	}
	h.day = day
	a.showListeningTrack(nil)
}

func (a *app) showListeningTrack(track *listening.Entry) {
	h := &a.listening
	h.track, h.limit = track, listeningPageSize
	h.view.Entries, h.view.More = nil, false
	a.pageScroll = ui.ScrollState{}
	a.readListening()
}

func (a *app) readListening() {
	h := &a.listening
	if h.store == nil {
		h.failed = true
		return
	}
	h.request++
	id := h.request
	h.loading, h.failed = true, false
	var track *listening.Track
	if h.track != nil {
		track = &h.track.Track
	}
	h.store.Read(h.day, track, h.limit, func(v listening.View, err error) {
		a.post(func() {
			if h.request != id {
				return
			}
			h.loading, h.failed = false, err != nil
			if err != nil {
				log.Printf("reading listening history: %v", err)
				return
			}
			if track != nil {
				v.Days = h.view.Days
			}
			h.view = v
		})
	})
}

// stepListeningMonth keeps the selected day number where the new month has it.
func (a *app) stepListeningMonth(delta int) {
	day := a.listening.day
	first := time.Date(day.Year(), day.Month()+time.Month(delta), 1, 0, 0, 0, 0, day.Location())
	last := first.AddDate(0, 1, -1).Day()
	a.showListeningDay(first.AddDate(0, 0, min(day.Day(), last)-1))
}
