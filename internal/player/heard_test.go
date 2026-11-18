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

// A seek reported with a pause must not add the skipped audio to playEnded.
func TestRemotePauseAfterSeekCountsOnlyPlayback(t *testing.T) {
	for _, pos := range []int64{100000, 199000, 5000} {
		r := newHeardRig(t, 1)
		r.src.set(playingState("a", 10000), nil)
		r.poll(10 * time.Second)
		r.src.set(&music.RemoteState{TrackID: "a", Position: pos, Duration: 200000}, nil)
		r.poll(time.Second)
		if r.p.Playing() || r.p.Position() != time.Duration(pos)*time.Millisecond {
			t.Fatalf("position %s, playing %t after pause at %d", r.p.Position(), r.p.Playing(), pos)
		}
		r.p.Quit()
		r.ended("a", 11*time.Second, music.EndQuit)
	}
}

// Near the end, a device can pause at a rewound position or reset to zero on completion.
func TestRemoteNearEndPauseDistinguishesRewindFromCompletion(t *testing.T) {
	for _, pos := range []int64{30000, 0} {
		r := newHeardRig(t, 2)
		r.src.set(playingState("a", 198000), nil)
		r.poll(198 * time.Second)
		r.drain()
		r.src.set(&music.RemoteState{TrackID: "a", Position: pos, Duration: 200000}, nil)
		r.poll(2 * time.Second)
		if pos == 0 {
			r.ended("a", 200*time.Second, music.EndFinished)
			r.await("play")
			r.settle()
			if r.p.Current().ID != "b" {
				t.Fatalf("next track %+v", r.p.Current())
			}
			continue
		}
		if r.p.Playing() || r.p.Current().ID != "a" || r.p.Position() != 30*time.Second {
			t.Fatalf("rewound near the end: track %+v, playing %t, position %s", r.p.Current(), r.p.Playing(), r.p.Position())
		}
		r.p.Quit()
		r.ended("a", 200*time.Second, music.EndQuit)
	}
}

type heardRecorder struct {
	remoteFake
	starts chan string
}

func (s *heardRecorder) Played(track music.Track) { s.starts <- track.ID }

// Each local repeat starts and ends its own play for the source.
func TestLocalRepeatOneReportsEachPlay(t *testing.T) {
	_, p := posted()
	defer p.out.Close()
	src := &heardRecorder{remoteFake: remoteFake{ends: make(chan ending, 2)}, starts: make(chan string, 3)}
	p.Source = func(string) music.Source { return src }
	p.repeat = RepeatOne
	p.record()
	p.listen()
	for i := 0; i < 3; i++ {
		select {
		case id := <-src.starts:
			if id != "1" {
				t.Fatalf("started %q", id)
			}
		case <-time.After(time.Second):
			t.Fatalf("play %d started without a played callback", i+1)
		}
		if i == 2 {
			break
		}
		p.ended()
		select {
		case got := <-src.ends:
			if got.id != "1" || got.why != music.EndFinished {
				t.Fatalf("ended %+v", got)
			}
		case <-time.After(time.Second):
			t.Fatalf("play %d did not finish", i+1)
		}
	}
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
