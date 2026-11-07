package app

import (
	"errors"
	"testing"

	"github.com/egoist/mygo/ui"

	"mygo-music/internal/music"
)

// featureApp is Home led by a feature card of three songs.
func featureApp(more string) *app {
	a := newTestApp()
	a.home = &music.Browse{Sections: []music.Section{{
		Title:    "For You",
		Tracks:   sourced(song("1", "一", "甲"), song("2", "二", "乙"), song("3", "三", "丙")),
		MorePage: more,
	}}}
	return a
}

// The feature card opens all of its songs: their section's page, back from
// which is Home, or the page its source says has the rest.
func TestFeatureCardOpens(t *testing.T) {
	a := featureApp("")
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("For You"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.route != (route{page: pageSection, src: "fake", query: "0"}) {
		t.Fatalf("opened %+v", a.route)
	}
	if _, ok := tt.Find("三"); !ok {
		t.Error("its page doesn't list the songs")
	}
	tt.Key(0, ui.KeyBack)
	tt.Frame()
	if a.route.page != pageHome {
		t.Errorf("back went to %+v", a.route)
	}

	a = featureApp("more")
	tt = ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("For You"); err != nil {
		t.Fatal(err)
	}
	if a.route != (route{page: pageBrowse, src: "fake", id: "more"}) {
		t.Errorf("opened %+v", a.route)
	}
}

// The feature card's button plays its songs without opening them, and pauses
// them once they play.
func TestFeatureCardPlays(t *testing.T) {
	a := featureApp("")
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("Play"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if cur := a.pl.Current(); cur == nil || cur.ID != "1" || len(a.pl.Queue()) != 3 || a.route.page != pageHome {
		t.Fatalf("playing %+v of %d on %+v", cur, len(a.pl.Queue()), a.route)
	}
	// Under the pointer, it pauses what plays.
	if err := tt.Click("Pause"); err != nil {
		t.Fatal(err)
	}
	if a.pl.Playing() || a.pl.Current().ID != "1" {
		t.Errorf("playing %v, %+v", a.pl.Playing(), a.pl.Current())
	}
}

// On a wide page, a pick on the feature card plays its songs from it.
func TestFeatureCardPicks(t *testing.T) {
	a := featureApp("")
	tt := ui.NewTester(a.view, 1500, 800)
	tt.Frame()
	if err := tt.Click("二"); err != nil {
		t.Fatal(err)
	}
	if cur := a.pl.Current(); cur == nil || cur.ID != "2" || len(a.pl.Queue()) != 3 {
		t.Fatalf("playing %+v of %d", cur, len(a.pl.Queue()))
	}
}

// Playlists in the feature card's own section keep off it as rows of cards keep
// apart: they came right under it, as its section's title, which would have
// kept them off, is the card's.
func TestFeatureCardKeepsItsSectionOff(t *testing.T) {
	a := featureApp("")
	a.home.Sections[0].Playlists = []music.Playlist{{ID: "p1", Name: "Coding BGM"}}
	tt := ui.NewTester(a.view, 1200, 900)
	settled(tt)
	card, ok := tt.Find("For You")
	list, ok2 := tt.Find("Coding BGM")
	if !ok || !ok2 {
		t.Fatalf("shows %q", tt.Texts())
	}
	if gap := list.Y - (card.Y + card.H); gap < cardRowGap-0.5 {
		t.Errorf("the playlists are %.1f under the card", gap)
	}
}

// The page the feature card opens is headed by the section's pageTitle, if
// it has one, and its subtitle follows the count of songs.
func TestSectionPageHeading(t *testing.T) {
	a := featureApp("")
	a.home.Sections[0].PageTitle = "Daily Song Picks"
	a.home.Sections[0].Subtitle = "New every day"
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("For You"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if _, ok := tt.Find("Daily Song Picks"); !ok {
		t.Error("the page isn't headed by its pageTitle")
	}
	if _, ok := tt.Find("3 songs · New every day"); !ok {
		t.Error("the subtitle doesn't follow the count")
	}
}

// sectionNavApp's sidebar has an entry for the songs of its Home's section
// "daily", which isn't first, and Home answers with what home does, counting
// how often it was asked.
func sectionNavApp(home func() (*music.Browse, error)) (*app, chan func(), *int) {
	a := newTestApp()
	ch := posts(a)
	without(a, "isLiked")
	asked := new(int)
	f := fake(a)
	f.info.Navigation = []music.NavItem{{Builtin: "home"}, {Title: "Daily Songs", Section: "daily", Icon: "calendar"}}
	f.home = func() (*music.Browse, error) {
		*asked++
		return home()
	}
	return a, ch, asked
}

// dailyHome is a Home whose section "daily" comes after another.
func dailyHome() (*music.Browse, error) {
	return &music.Browse{Sections: []music.Section{
		{Title: "Albums", Albums: []music.Album{{ID: "8", Name: "迷跡波"}}},
		{ID: "daily", Title: "For You", PageTitle: "Daily Song Picks", Tracks: sourced(song("1", "一", "甲"), song("2", "二", "乙"), song("3", "三", "丙"))},
	}}, nil
}

// The sidebar opens the page of a Home section's songs by its ID before Home
// has loaded, loading it, and the page plays them all once it comes.
func TestSidebarOpensSectionBeforeHome(t *testing.T) {
	a, ch, asked := sectionNavApp(dailyHome)
	a.route = route{page: pageLiked, src: "fake"}
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("Daily Songs"); err != nil {
		t.Fatal(err)
	}
	if a.route != (route{page: pageSection, src: "fake", section: "daily"}) {
		t.Fatalf("opened %+v", a.route)
	}
	tt.Frame()
	if tt.HasText("This page is gone") {
		t.Error("the page is gone while Home loads")
	}
	until(t, ch, func() bool { return !a.homeLoading })
	tt.Frame()
	if *asked != 1 {
		t.Errorf("Home was asked %d times", *asked)
	}
	if _, ok := tt.Find("Daily Song Picks"); !ok {
		t.Fatalf("shows %q", tt.Texts())
	}
	if err := tt.Click("Play"); err != nil {
		t.Fatal(err)
	}
	if cur := a.pl.Current(); cur == nil || cur.ID != "1" || len(a.pl.Queue()) != 3 {
		t.Fatalf("playing %+v of %d", cur, len(a.pl.Queue()))
	}
}

// The sidebar's entry for a section's songs is selected on their page whether
// it or Home's feature card opened it, and Home isn't selected there.
func TestSectionEntrySelectedFromEither(t *testing.T) {
	a, ch, _ := sectionNavApp(func() (*music.Browse, error) {
		b, _ := dailyHome()
		b.Sections[0], b.Sections[1] = b.Sections[1], b.Sections[0]
		return b, nil
	})
	a.loadHome(a.library)
	until(t, ch, func() bool { return !a.homeLoading })
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("For You"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	items := a.sidebarItems()
	if !a.navSelected(items[1].route) || a.navSelected(items[0].route) {
		t.Fatalf("on %+v, the section's entry selected %v, Home's %v", a.route, a.navSelected(items[1].route), a.navSelected(items[0].route))
	}
	if err := tt.Click("Daily Songs"); err != nil {
		t.Fatal(err)
	}
	if len(a.history) != 1 {
		t.Errorf("the entry opened the page it's on again: %+v", a.history)
	}
}

// Opened from the sidebar while Home fails, the page says why with a way to
// try again, which shows the songs once Home comes.
func TestSectionPageRetriesHome(t *testing.T) {
	fail := true
	a, ch, _ := sectionNavApp(func() (*music.Browse, error) {
		if fail {
			return nil, errors.New("offline")
		}
		return dailyHome()
	})
	tt := ui.NewTester(a.view, 1200, 800)
	tt.Frame()
	if err := tt.Click("Daily Songs"); err != nil {
		t.Fatal(err)
	}
	until(t, ch, func() bool { return !a.homeLoading })
	tt.Frame()
	if !tt.HasText("Couldn't load this page: offline") {
		t.Fatalf("shows %q", tt.Texts())
	}
	fail = false
	if err := tt.Click("Retry"); err != nil {
		t.Fatal(err)
	}
	until(t, ch, func() bool { return !a.homeLoading })
	tt.Frame()
	if _, ok := tt.Find("三"); !ok {
		t.Errorf("shows %q", tt.Texts())
	}
}
