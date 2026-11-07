package app

import (
	"errors"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"mygo-music/internal/music"
)

// sourced is tracks of the source called fake, as the player goes by.
func sourced(tracks ...music.Track) []music.Track {
	for i := range tracks {
		tracks[i].Source = "fake"
	}
	return tracks
}

// radioApp shows the stations of its source, one of them called Night Drive,
// signed in and with what the app posts in hand.
func radioApp() (*app, chan func()) {
	a := newTestApp()
	ch := posts(a)
	without(a, "isLiked")
	a.account = &music.Account{ID: "1"}
	a.stations, a.stationsOnce = []music.Station{{ID: "st", Name: "Night Drive"}}, true
	a.route = route{page: pageStations, src: "fake"}
	return a, ch
}

// Starting a station plays what its source gives, and it goes on asking for
// more as the songs near their end, without the songs it has twice.
func TestStationPlaysAndGoesOn(t *testing.T) {
	asked := make(chan string, 4)
	a, ch := radioApp()
	batches := [][]music.Track{sourced(song("1", "一"), song("2", "二")), sourced(song("2", "二"), song("3", "三"))}
	fake(a).stationNext = func(id string) ([]music.Track, error) {
		asked <- id
		b := batches[0]
		if len(batches) > 1 {
			batches = batches[1:]
		}
		return b, nil
	}
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("Start Listening"); err != nil {
		t.Fatal(err)
	}
	if id := wait(t, asked, "the first songs"); id != "st" {
		t.Fatalf("asked station %q", id)
	}
	until(t, ch, func() bool { return a.pl.Radio() != "" })
	if a.pl.Radio() != radioName("fake", &a.stations[0]) || a.pl.Current().ID != "1" || a.radioStarting != "" {
		t.Fatalf("radio %q, playing %+v, starting %q", a.pl.Radio(), a.pl.Current(), a.radioStarting)
	}
	wait(t, asked, "more songs")
	until(t, ch, func() bool { return len(a.pl.Queue()) > 2 })
	if q := a.pl.Queue(); len(q) != 3 || q[2].ID != "3" {
		t.Errorf("the queue is %+v", q)
	}
}

// Playing the station playing goes on with it, not starting it again.
func TestStationPlayingGoesOn(t *testing.T) {
	a, _ := radioApp()
	fake(a).stationNext = func(string) ([]music.Track, error) { t.Error("started it again"); return nil, nil }
	a.pl.Cue(sourced(song("1", "一"), song("2", "二"), song("3", "三")))
	a.pl.AttachRadio(radioName("fake", &a.stations[0]), func(func([]music.Track)) {})
	a.playStation(a.library, &a.stations[0])
	if a.radioStarting != "" {
		t.Errorf("starting %q", a.radioStarting)
	}
}

// A station that only gives what can't be played is asked again a few times,
// then says it can't play, and one that fails says so too.
func TestStationWithNothingToPlay(t *testing.T) {
	asked := 0
	a, ch := radioApp()
	unavailable := song("1", "一")
	unavailable.Unavailable = true
	fake(a).stationNext = func(string) ([]music.Track, error) { asked++; return []music.Track{unavailable}, nil }
	a.playStation(a.library, &a.stations[0])
	runPost(t, ch)
	if asked != 3 || a.pl.Radio() != "" || a.pl.Current() != nil || a.toast == "" || a.radioStarting != "" {
		t.Fatalf("asked %d times, radio %q, toast %q, starting %q", asked, a.pl.Radio(), a.toast, a.radioStarting)
	}

	a.toast = ""
	fake(a).stationNext = func(string) ([]music.Track, error) { return nil, errors.New("offline") }
	a.playStation(a.library, &a.stations[0])
	runPost(t, ch)
	if a.pl.Radio() != "" || a.toast == "" {
		t.Errorf("radio %q, toast %q", a.pl.Radio(), a.toast)
	}
}

// A station asked for while signed out asks the listener to sign in, not it.
func TestStationSignedOut(t *testing.T) {
	a, _ := radioApp()
	a.account = nil
	fake(a).stationNext = func(string) ([]music.Track, error) { t.Error("asked the station"); return nil, nil }
	a.playStation(a.library, &a.stations[0])
	if !a.signIn.open {
		t.Error("didn't ask to sign in")
	}
}

// Now Playing's chip leaves the radio: the song plays on, and those the radio
// queued after it go.
func TestLeaveRadio(t *testing.T) {
	a, _ := radioApp()
	a.pl.Cue(sourced(song("1", "晚风"), song("2", "推荐的歌"), song("3", "下一首")))
	a.pl.SetLyrics(nil)
	a.pl.AttachRadio(radioName("fake", &a.stations[0]), func(func([]music.Track)) {})
	a.nowPlaying = true
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	tt.Advance(time.Second)
	tt.Frame()
	if err := tt.Click("Leave Night Drive"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.pl.Radio() != "" || a.pl.Current().ID != "1" || len(a.pl.Queue()) != 1 {
		t.Fatalf("radio %q, current %+v, queue %d", a.pl.Radio(), a.pl.Current(), len(a.pl.Queue()))
	}
	if _, ok := tt.Find("Leave Night Drive"); ok {
		t.Error("the chip stayed")
	}
}

// A radio's name gives back the source, the station and what it's called, and
// something else names none.
func TestRadioName(t *testing.T) {
	src, id, label := radioParts(radioName("fake", &music.Station{ID: "st", Name: "Night Drive"}))
	if src != "fake" || id != "st" || label != "Night Drive" {
		t.Errorf("source %q, station %q, called %q", src, id, label)
	}
	if src, _, _ := radioParts("heart"); src != "" {
		t.Errorf("source %q of something else", src)
	}
}

// A few stations are a switch: the one chosen is what Start Listening plays, and
// with one playing, choosing another changes to it at once.
func TestStationSwitch(t *testing.T) {
	asked := make(chan string, 4)
	a, ch := radioApp()
	a.stations = []music.Station{{ID: "a", Name: "First"}, {ID: "b", Name: "Second"}, {ID: "c", Name: "Third"}}
	fake(a).stationNext = func(id string) ([]music.Track, error) {
		asked <- id
		return sourced(song(id+"1", "一"), song(id+"2", "二"), song(id+"3", "三")), nil
	}
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("Second"); err != nil {
		t.Fatal(err)
	}
	settled(tt)
	if a.pl.Radio() != "" {
		t.Fatal("choosing one started it")
	}
	if err := tt.Click("Start Listening"); err != nil {
		t.Fatal(err)
	}
	if id := wait(t, asked, "the first songs"); id != "b" {
		t.Fatalf("started %q", id)
	}
	until(t, ch, func() bool { return a.pl.Radio() != "" })
	settled(tt)
	if err := tt.Click("Third"); err != nil {
		t.Fatal(err)
	}
	for {
		if id := wait(t, asked, "the other station"); id == "c" {
			break
		}
	}
	until(t, ch, func() bool { return a.pl.Radio() == radioName("fake", &a.stations[2]) })
}

// Stations a source calls standalone are banners of their own, and the rest go
// by the rule as before: a few a switch, too many a grid.
func TestStationGroups(t *testing.T) {
	ids := func(sts []*music.Station) (out string) {
		for _, st := range sts {
			out += st.ID
		}
		return out
	}
	for _, tc := range []struct {
		stations                []music.Station
		switched, card, banners string
	}{
		{[]music.Station{{ID: "a"}, {ID: "b"}, {ID: "c"}}, "abc", "", ""},
		{[]music.Station{{ID: "a"}, {ID: "b"}, {ID: "c"}, {ID: "d"}, {ID: "e"}}, "", "abcde", ""},
		{[]music.Station{{ID: "a"}, {ID: "h", Standalone: true}, {ID: "b"}, {ID: "c"}}, "abc", "", "h"},
		{[]music.Station{{ID: "a"}, {ID: "b"}, {ID: "c"}, {ID: "d"}, {ID: "e"}, {ID: "h", Standalone: true}}, "", "abcde", "h"},
	} {
		switched, cards, banners := stationGroups(tc.stations)
		if ids(switched) != tc.switched || ids(cards) != tc.card || ids(banners) != tc.banners {
			t.Errorf("%+v: switched %q, cards %q, banners %q", tc.stations, ids(switched), ids(cards), ids(banners))
		}
	}
}

// heartApp shows a switch of two stations and Heart Mode, standalone, with
// what the app posts in hand.
func heartApp() (*app, chan func()) {
	a, ch := radioApp()
	a.stations = []music.Station{
		{ID: "a", Name: "First"}, {ID: "b", Name: "Second"},
		{ID: "heart", Name: "Heart Mode", Headline: "Songs you'll love", Icon: "heart", Standalone: true},
	}
	return a, ch
}

// A standalone station's banner starts it by its own ID, and the switch
// beside it chooses among the others only.
func TestStandaloneStationBanner(t *testing.T) {
	asked := make(chan string, 4)
	a, ch := heartApp()
	fake(a).stationNext = func(id string) ([]music.Track, error) {
		asked <- id
		return sourced(song(id+"1", "一"), song(id+"2", "二"), song(id+"3", "三")), nil
	}
	tt := ui.NewTester(a.view, 1200, 900)
	tt.Frame()
	if _, ok := tt.AccessByRole(ui.RoleRadio, "Heart Mode"); ok {
		t.Fatal("the switch offers the standalone station")
	}
	if err := tt.Click("Start Playing"); err != nil {
		t.Fatal(err)
	}
	if id := wait(t, asked, "the first songs"); id != "heart" {
		t.Fatalf("started %q", id)
	}
	until(t, ch, func() bool { return a.pl.Radio() != "" })
	if a.pl.Radio() != radioName("fake", &a.stations[2]) {
		t.Fatalf("radio %q", a.pl.Radio())
	}
}

// Leaving a standalone station from its banner stops the radio, and the song
// it played plays on.
func TestLeaveStandaloneStation(t *testing.T) {
	a, _ := heartApp()
	fake(a).stationNext = func(string) ([]music.Track, error) { return nil, nil }
	a.pl.Cue(sourced(song("1", "晚风"), song("2", "推荐的歌")))
	a.pl.AttachRadio(radioName("fake", &a.stations[2]), func(func([]music.Track)) {})
	tt := ui.NewTester(a.view, 1200, 900)
	settled(tt)
	// Paused on the station, the banner offered "Playing", as if it were.
	if _, ok := tt.Find("Playing"); ok {
		t.Error("the paused banner says it's playing")
	}
	if _, ok := tt.Find("Resume"); !ok {
		t.Error("the paused banner doesn't offer to resume")
	}
	if err := tt.Click("Leave Heart Mode"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.pl.Radio() != "" || a.pl.Current() == nil || a.pl.Current().ID != "1" {
		t.Fatalf("radio %q, current %+v", a.pl.Radio(), a.pl.Current())
	}
	if _, ok := tt.Find("Leave Heart Mode"); ok {
		t.Error("the banner still offers to leave")
	}
	if _, ok := tt.Find("Start Playing"); !ok {
		t.Error("the banner doesn't offer to start again")
	}
}
