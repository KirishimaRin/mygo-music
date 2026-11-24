package app

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"mygo-music/internal/music"
	"mygo-music/internal/plugin"
)

func loadPlaybackObserver(t *testing.T, source string) *plugin.Plugin {
	t.Helper()
	dir := t.TempDir()
	for name, body := range map[string]string{
		"plugin.json": `{"id":"observer","observePlayback":true}`,
		"index.js":    source,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	p, err := plugin.Load(dir, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	return p
}

func checkPlaybackEvents(t *testing.T, p *plugin.Plugin, want ...string) {
	t.Helper()
	tracks, err := p.Recent()
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(tracks))
	for _, track := range tracks {
		got = append(got, track.ID)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("events %q, want %q", got, want)
	}
}

// A slow callback keeps the next events behind it, and quit waits even
// when the player has already switched away from the observed tracks.
func TestPlaybackObserverOrdersCallbacksBeforeQuit(t *testing.T) {
	p := loadPlaybackObserver(t, `
const events = [];
let release;
const gate = new Promise(resolve => { release = resolve; });
export async function observePlayed({id, source, name}) {
  if (id === "a") await gate;
  events.push([id, source, name].join(" "));
}
export function observeEnded({id, source, name, seconds, reason}) {
  events.push([id, source, name, seconds, reason].join(" "));
}
export function like() { release(); }
export function recent() { return events.map(id => ({id})); }
`)
	a := newTestApp()
	addFake(a, p)
	a.enabledChanged()

	a.pl.ObservePlay(music.Track{ID: "a", Source: "fake", Name: "First"})(1250*time.Millisecond, music.EndSkipped)
	a.pl.ObservePlay(music.Track{ID: "b", Source: "fake", Name: "Second"})(8250*time.Millisecond, music.EndFinished)
	checkPlaybackEvents(t, p)

	done := make(chan struct{})
	go func() {
		a.pl.Quit()
		close(done)
	}()
	if err := p.Like("release", true); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("quit did not return")
	}
	checkPlaybackEvents(t, p, "a fake First", "a fake First 1.3 skipped", "b fake Second", "b fake Second 8.3 ended")
}

// Disabling a plugin drops its queued events and stops plays already under
// way from reaching it after it's enabled again.
func TestPlaybackObserverDisableDropsOldPlay(t *testing.T) {
	p := loadPlaybackObserver(t, `
const events = [];
let markStarted;
const started = new Promise(resolve => { markStarted = resolve; });
export async function observePlayed({id}) {
  events.push("start " + id);
  if (id === "old") {
    markStarted();
    await new Promise(() => {});
  }
}
export function observeEnded({id}) { events.push("end " + id); }
export async function recent() {
  await started;
  return events.map(id => ({id}));
}
`)
	a := newTestApp()
	lib := addFake(a, p)
	a.enabledChanged()
	endOld := a.pl.ObservePlay(music.Track{ID: "old", Source: "fake"})
	a.pl.ObservePlay(music.Track{ID: "queued", Source: "fake"})
	checkPlaybackEvents(t, p, "start old")
	old := a.playbackObservers[p]
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	a.settings.DisabledPlugins = []string{lib.id}
	a.enabledChanged()
	if a.pl.ObservePlay != nil {
		t.Fatal("a disabled plugin still subscribes to new plays")
	}
	a.settings.DisabledPlugins = nil
	a.enabledChanged()
	endOld(time.Second, music.EndSkipped)
	a.pl.ObservePlay(music.Track{ID: "new", Source: "fake"})(time.Second, music.EndFinished)
	old.wait(ctx)
	if ctx.Err() != nil {
		t.Fatal("the old observer didn't stop")
	}
	a.pl.Quit()

	checkPlaybackEvents(t, p, "start old", "start new", "end new")
}
