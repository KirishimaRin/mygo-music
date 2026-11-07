package player

import (
	"testing"
	"time"

	"mygo-music/internal/music"
)

// newHeardRig is a remote rig whose source hears how each play ends.
func newHeardRig(t *testing.T, n int) *remoteRig {
	return newRemoteRigOf(t, n, &remoteFake{cmd: make(chan string, 64), ends: make(chan ending, 8)})
}

// ended waits for the source to hear a play ended, and checks how.
func (r *remoteRig) ended(id string, heard time.Duration, why music.EndReason) {
	r.t.Helper()
	select {
	case got := <-r.src.ends:
		if want := (ending{id, heard, why}); got != want {
			r.t.Fatalf("heard %+v, want %+v", got, want)
		}
	case <-time.After(2 * time.Second):
		r.t.Fatalf("not told how %s ended", id)
	}
}

// nothingMore checks the source heard of no other play ending.
func (r *remoteRig) nothingMore() {
	r.t.Helper()
	select {
	case got := <-r.src.ends:
		r.t.Fatalf("also heard %+v", got)
	case <-time.After(20 * time.Millisecond):
	}
}

// The time a song is paused doesn't count as heard.
func TestPausedIsNotHeard(t *testing.T) {
	r := newHeardRig(t, 2)
	r.now = r.now.Add(10 * time.Second)
	r.p.TogglePlay()
	r.await("pause")
	r.now = r.now.Add(time.Minute)
	r.p.TogglePlay()
	r.await("resume")
	r.now = r.now.Add(5 * time.Second)
	r.p.Next()
	r.ended("a", 15*time.Second, music.EndSkipped)
	r.await("play")
}

// What a seek jumps over isn't heard, and what a seek back plays again is.
func TestSeekCountsWhatPlays(t *testing.T) {
	r := newHeardRig(t, 2)
	r.now = r.now.Add(10 * time.Second)
	r.p.Seek(100 * time.Second)
	r.await("seek")
	r.now = r.now.Add(5 * time.Second)
	r.p.Seek(20 * time.Second)
	r.await("seek")
	r.now = r.now.Add(30 * time.Second)
	r.p.Next()
	r.ended("a", 45*time.Second, music.EndSkipped)
	r.await("play")
}

// A song played to its end is told so once, and played again it's another
// play, counted from nothing.
func TestEachPlayEndsOnce(t *testing.T) {
	r := newHeardRig(t, 2)
	r.src.set(playingState("a", 198000), nil)
	r.poll(198 * time.Second)
	r.drain()
	r.src.set(&music.RemoteState{TrackID: "a", Position: 200000, Duration: 200000}, nil)
	r.poll(2 * time.Second)
	r.await("play")
	r.drain()
	r.settle()
	r.ended("a", 200*time.Second, music.EndFinished)
	r.nothingMore()

	r.now = r.now.Add(3 * time.Second)
	r.p.Jump(1)
	r.await("play")
	r.settle()
	r.ended("b", 3*time.Second, music.EndSkipped)
	r.now = r.now.Add(7 * time.Second)
	r.p.Next()
	r.ended("b", 7*time.Second, music.EndSkipped)
	r.await("play")
	r.settle()
	r.nothingMore()
}

// Quitting tells the source how the song playing went before it returns,
// for the app to close the source only then.
func TestQuitEndsThePlay(t *testing.T) {
	r := newHeardRig(t, 1)
	r.now = r.now.Add(12 * time.Second)
	r.p.Quit()
	select {
	case got := <-r.src.ends:
		if got != (ending{"a", 12 * time.Second, music.EndQuit}) {
			t.Errorf("heard %+v", got)
		}
	default:
		t.Fatal("quit before the source heard")
	}
	r.p.Quit()
	r.nothingMore()
}

// A source that doesn't want to know isn't kept count for.
func TestNoCountWithoutPlayEnded(t *testing.T) {
	r := newRemoteRig(t, 2)
	if r.p.listening {
		t.Error("counting for a source without playEnded")
	}
}
