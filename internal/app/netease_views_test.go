package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"mygo-music/internal/music"
)

type wikiTestSource struct {
	*fakeSource
	asked chan string
}

func (s *wikiTestSource) SongWiki(id string) (*music.SongWiki, error) {
	s.asked <- id
	return &music.SongWiki{Sections: []music.WikiSection{{
		Title:   "Credits",
		Entries: []music.WikiEntry{{Title: "Composer " + id, ArtistID: "19"}},
	}}}, nil
}

// Opening the wiki loads only the current song and its artist link uses that source.
func TestWikiOpensFromNowPlaying(t *testing.T) {
	a := newTestApp()
	ch := posts(a)
	without(a, "comments", "like", "isLiked")
	src := &wikiTestSource{fake(a), make(chan string, 4)}
	a.library.src = src
	a.library.caps["songWiki"] = true
	a.pl.Cue([]music.Track{song("1", "One"), song("2", "Two")})
	a.pl.SetLyrics(nil)
	a.nowPlaying = true
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Advance(time.Second)
	tt.Frame()
	if len(a.library.wikis) != 0 {
		t.Fatal("the wiki loaded before its tab was opened")
	}
	if err := tt.Click("Encyclopedia"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if id := wait(t, src.asked, "wiki request"); id != "1" {
		t.Fatalf("loaded wiki for %s", id)
	}
	a.pl.Next()
	a.pl.SetLyrics(nil)
	tt.Frame()
	if id := wait(t, src.asked, "next wiki request"); id != "2" {
		t.Fatalf("loaded wiki for %s after a track change", id)
	}
	runPost(t, ch)
	runPost(t, ch)
	tt.Frame()
	if err := tt.Click("Composer 2"); err != nil {
		t.Fatal(err)
	}
	if a.nowPlaying || a.route != (route{page: pageArtist, src: a.library.id, id: "19"}) {
		t.Fatalf("the composer link opened %+v, now playing %v", a.route, a.nowPlaying)
	}
}

// The saved-albums disclosure expands onto album links without moving the active page.
func TestSidebarSavedAlbumOpensAlbum(t *testing.T) {
	a := editTestApp()
	fake(a).info.Navigation = []music.NavItem{{Builtin: "savedAlbums", SidebarList: true}}
	a.library.savedAlbums = []music.Album{{ID: "78", Name: "A saved record"}}
	a.library.savedAlbumsPaged.loaded = true
	before := a.route
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("Saved Albums"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.route != before {
		t.Fatal("the disclosure navigated away")
	}
	if _, visible := tt.Find("A saved record"); visible {
		t.Fatal("the collapsed section still builds its album row")
	}
	if err := tt.Click("Saved Albums"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("A saved record"); err != nil {
		t.Fatal(err)
	}
	if a.route != (route{page: pageAlbum, src: a.library.id, id: "78"}) {
		t.Fatalf("the album row opened %+v", a.route)
	}
}

type cloudTestSource struct {
	*fakeSource
	queries chan string
}

func (s *cloudTestSource) CloudTracks(offset int, query string) (*music.CloudPage, error) {
	s.queries <- query
	if query == "" {
		return &music.CloudPage{}, nil
	}
	if offset == 0 {
		return &music.CloudPage{Next: 30}, nil
	}
	return &music.CloudPage{Tracks: []music.Track{song("42", "Uploaded song")}}, nil
}

// A cloud search goes beyond a page with no matches and can play the result.
func TestCloudSearchContinuesPastEmptyPage(t *testing.T) {
	a := newTestApp()
	a.account = &music.Account{ID: "1"}
	ch := posts(a)
	src := &cloudTestSource{fake(a), make(chan string, 4)}
	src.info.Navigation = []music.NavItem{{Page: "cloud", View: "cloudTracks", Title: "My Music Cloud"}}
	a.library.src = src
	a.library.caps["cloudTracks"] = true
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("My Music Cloud"); err != nil {
		t.Fatal(err)
	}
	if query := wait(t, src.queries, "cloud browse"); query != "" {
		t.Fatalf("searched for %q on entering the cloud page", query)
	}
	runPost(t, ch)
	tt.Frame()
	if err := tt.Click("Search Music Cloud"); err != nil {
		t.Fatal(err)
	}
	tt.Type("uploaded")
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	if query := wait(t, src.queries, "cloud search"); query != "uploaded" {
		t.Fatalf("searched for %q", query)
	}
	runPost(t, ch)
	tt.Frame()
	tt.Frame()
	if query := wait(t, src.queries, "the next search page"); query != "uploaded" {
		t.Fatalf("next page searched for %q", query)
	}
	runPost(t, ch)
	tt.Frame()
	if err := tt.Click("Uploaded song"); err != nil {
		t.Fatal(err)
	}
	if track := a.pl.Current(); track == nil || track.ID != "42" {
		t.Fatalf("the match did not play: %v", track)
	}
}
