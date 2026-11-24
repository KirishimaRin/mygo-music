package player

import (
	"context"
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

// listen starts a play of the current track, counting what is heard when
// its source or an observer wants to know.
func (p *Player) listen() {
	if p.listening || p.track == nil || !p.playing {
		return
	}
	if !p.playBegan {
		p.playBegan = true
		if p.ObservePlay != nil {
			p.observeEnd = p.ObservePlay(*p.track)
		}
	}
	src := p.sourceOf(p.track)
	if _, ok := src.(music.PlayEnder); p.observeEnd == nil && (!ok || !src.Can("playEnded")) {
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

// endPlay settles the play under way and queues its observation. The returned
// channel, if any, closes once the source has heard about its own play.
func (p *Player) endPlay(why music.EndReason, pos time.Duration) (done chan struct{}) {
	p.playBegan = false
	if !p.listening {
		return nil
	}
	p.listening = false
	p.heardTo(pos)
	if p.observeEnd != nil {
		p.observeEnd(p.heard, why)
		p.observeEnd = nil
	}
	// Its source as it is now: one taken out meanwhile isn't told.
	source := p.sourceOf(p.track)
	src, ok := source.(music.PlayEnder)
	if !ok || !source.Can("playEnded") {
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

// Quit ends the current play and gives its source and observers one budget
// to finish before the app closes the plugins.
func (p *Player) Quit() {
	done := p.endPlay(music.EndQuit, p.Position())
	if done == nil && p.DrainObservers == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), quitWait)
	defer cancel()
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
		}
	}
	if p.DrainObservers != nil {
		p.DrainObservers(ctx)
	}
}
