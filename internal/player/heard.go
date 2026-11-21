package player

import (
	"time"

	"mygo-music/internal/music"
)

// A play of a track lasts from when it starts to play until something else
// plays, it plays to its end, or the app quits. For a source that wants to
// know, the player keeps how much of it is heard: how far its position moved
// on, from where it started and from each seek, so what's paused, waited for
// or jumped over doesn't count, and what a seek back plays again does. It's
// added up only as the position jumps and as the play ends, never as it plays.

// quitWait is how long quitting waits for a source to hear how its last play
// ended.
const quitWait = 2 * time.Second

// listen starts a play of the current track as it starts to play, unless one
// is under way or its source doesn't want to know.
func (p *Player) listen() {
	if p.listening || p.track == nil || !p.playing {
		return
	}
	src := p.sourceOf(p.track)
	if _, ok := src.(music.PlayEnder); !ok || !src.Can("playEnded") {
		return
	}
	p.listening, p.heard, p.mark = true, 0, p.Position()
}

// jumped notes that the position jumps to t, the play going on from there.
func (p *Player) jumped(t time.Duration) {
	if p.listening {
		p.heardTo(p.Position())
		p.mark = max(t, 0)
	}
}

// heardTo counts what was heard from the mark to pos, where the next begins.
func (p *Player) heardTo(pos time.Duration) {
	if pos > p.mark {
		p.heard += pos - p.mark
	}
	p.mark = pos
}

// endPlay ends the play under way at pos, and tells its source on a goroutine
// how much of it was heard and why it ended: done closes once it's been told.
func (p *Player) endPlay(why music.EndReason, pos time.Duration) (done chan struct{}) {
	if !p.listening {
		return nil
	}
	p.listening = false
	p.heardTo(pos)
	// Its source as it is now: one taken out meanwhile isn't told.
	src, ok := p.sourceOf(p.track).(music.PlayEnder)
	if !ok {
		return nil
	}
	t, heard := *p.track, p.heard
	done = make(chan struct{})
	go func() {
		defer close(done)
		src.PlayEnded(t, heard, why)
	}()
	return done
}

// Quit ends the play under way as the app quits, waiting a little for its
// source to hear of it, before the app closes the source.
func (p *Player) Quit() {
	if done := p.endPlay(music.EndQuit, p.Position()); done != nil {
		select {
		case <-done:
		case <-time.After(quitWait):
		}
	}
}
