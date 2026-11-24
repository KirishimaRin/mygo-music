package app

import (
	"context"
	"sync"
	"time"

	"mygo-music/internal/music"
	"mygo-music/internal/plugin"
)

type playbackEvent struct {
	track music.Track
	heard time.Duration
	why   music.EndReason // empty when the play starts
}

// A playbackObserver belongs to one period in which a plugin is enabled.
// It runs that plugin's callbacks in order, off the main thread.
type playbackObserver struct {
	plugin *plugin.Plugin
	ctx    context.Context
	cancel context.CancelFunc

	mu     sync.Mutex
	events []playbackEvent
	busy   chan struct{} // closed when the queue drains
}

func (o *playbackObserver) add(event playbackEvent) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.ctx.Err() != nil {
		return
	}
	o.events = append(o.events, event)
	if o.busy == nil {
		o.busy = make(chan struct{})
		go o.run()
	}
}

func (o *playbackObserver) run() {
	for {
		o.mu.Lock()
		if o.ctx.Err() != nil || len(o.events) == 0 {
			o.events = nil
			close(o.busy)
			o.busy = nil
			o.mu.Unlock()
			return
		}
		event := o.events[0]
		o.events[0] = playbackEvent{}
		o.events = o.events[1:]
		o.mu.Unlock()

		if event.why == "" {
			o.plugin.ObservePlayed(o.ctx, event.track)
		} else {
			o.plugin.ObserveEnded(o.ctx, event.track, event.heard, event.why)
		}
	}
}

func (o *playbackObserver) wait(ctx context.Context) {
	o.mu.Lock()
	busy := o.busy
	o.mu.Unlock()
	if busy != nil {
		select {
		case <-busy:
		case <-ctx.Done():
		}
	}
}

// syncPlaybackObservers replaces the enabled set. A play keeps the set it
// started with, and a removed member stops receiving its events.
func (a *app) syncPlaybackObservers() {
	if a.pl == nil {
		return
	}
	var next map[*plugin.Plugin]*playbackObserver
	for _, lib := range a.libs {
		p := pluginOf(lib)
		if p == nil || !p.ObservesPlayback() || !(p.Can("observePlayed") || p.Can("observeEnded")) {
			continue
		}
		o := a.playbackObservers[p]
		if o == nil {
			ctx, cancel := context.WithCancel(context.Background())
			o = &playbackObserver{plugin: p, ctx: ctx, cancel: cancel}
		}
		if next == nil {
			next = make(map[*plugin.Plugin]*playbackObserver)
		}
		next[p] = o
	}
	for p, o := range a.playbackObservers {
		if next[p] == nil {
			o.cancel()
		}
	}
	a.playbackObservers = next
	a.pl.ObservePlay, a.pl.DrainObservers = nil, nil
	if len(next) > 0 {
		a.pl.ObservePlay, a.pl.DrainObservers = a.observePlay, a.drainObservers
	}
}

func (a *app) observePlay(track music.Track) func(time.Duration, music.EndReason) {
	observers := a.playbackObservers
	for _, o := range observers {
		o.add(playbackEvent{track: track})
	}
	return func(heard time.Duration, why music.EndReason) {
		for _, o := range observers {
			o.add(playbackEvent{track: track, heard: heard, why: why})
		}
	}
}

func (a *app) drainObservers(ctx context.Context) {
	for _, o := range a.playbackObservers {
		o.wait(ctx)
	}
	for _, o := range a.playbackObservers {
		o.cancel()
	}
	a.playbackObservers = nil
	a.pl.ObservePlay, a.pl.DrainObservers = nil, nil
}
