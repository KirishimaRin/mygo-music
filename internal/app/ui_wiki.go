package app

import (
	"net/url"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"mygo-music/internal/music"
)

type wikiData struct {
	page    *music.SongWiki
	at      time.Time
	loading bool
	err     string
}

type wikiState struct {
	lib    *library
	id     string
	scroll ui.ScrollState
}

func (a *app) loadWiki(lib *library, id string, d *wikiData) {
	src, ok := lib.src.(music.WikiSource)
	if !ok || d.loading {
		return
	}
	d.loading, d.err = true, ""
	go func() {
		page, err := answered(src.SongWiki(id))
		a.post(func() {
			if lib.wikis[id] != d {
				return
			}
			d.loading = false
			if err != nil {
				d.err = T("Couldn't load the encyclopedia")
				return
			}
			d.page, d.at = page, time.Now()
		})
	}()
}

func (a *app) wikiPane(c *ui.Context, tr *music.Track) {
	lib, id := a.trackLib(tr), tr.ID
	if a.wiki.lib != lib || a.wiki.id != id {
		a.wiki = wikiState{lib: lib, id: id}
	}
	d := lib.wikis[id]
	if d == nil || d.page != nil && time.Since(d.at) > listsFresh {
		if d == nil && len(lib.wikis) >= 24 {
			for key := range lib.wikis {
				delete(lib.wikis, key)
				break
			}
		}
		d = &wikiData{}
		lib.wikis[id] = d
		a.loadWiki(lib, id, d)
	}
	c = c.Key(id)
	if d.page == nil || len(d.page.Sections) == 0 {
		ui.Column(c).Grow(1).Center().Gap(12).Children(func() {
			switch {
			case d.loading:
				ui.Text(c, T("Loading…")).TextColor(white.Alpha(0.65))
			case d.err != "":
				ui.Text(c, d.err).TextColor(white.Alpha(0.65))
				if pillButton(c.Key("retry"), iconRefresh, T("Retry"), white.Alpha(0.14), white).Clicked() {
					a.loadWiki(lib, id, d)
				}
			default:
				ui.Text(c, T("No encyclopedia information")).TextColor(white.Alpha(0.65))
			}
		})
		return
	}
	ui.Scroll(c).Grow(1).TrackScroll(&a.wiki.scroll).Padding(4, 8, 48, 0).Children(func() {
		ui.Column(c).Gap(16).Children(func() {
			for i, section := range d.page.Sections {
				a.wikiSection(c.Key(i), lib, section)
			}
		})
	})
}

func (a *app) wikiSection(c *ui.Context, lib *library, section music.WikiSection) {
	ui.Column(c).Radius(12).Background(white.Alpha(0.06)).Padding(16).Gap(10).Children(func() {
		ui.Text(c, section.Title).FontSize(17).FontWeight(700).TextColor(white)
		if section.Description != "" {
			ui.Text(c, section.Description).FontSize(14).LineHeight(1.45).TextColor(white.Alpha(0.76))
		}
		entries := section.Entries
		for i := 0; i < len(entries); {
			entry := entries[i]
			if entry.Style != "tag" {
				a.wikiRow(c.Key(i), entry, a.wikiAction(lib, entry))
				i++
				continue
			}
			end := i + 1
			for end < len(entries) && entries[end].Style == "tag" && entries[end].Subtitle == entry.Subtitle {
				end++
			}
			a.wikiTags(c.Key(i), lib, entry.Subtitle, entries[i:end])
			i = end
		}
		for i, track := range section.Tracks {
			a.wikiRow(c.Key("track").Key(i), music.WikiEntry{
				Title: track.Name, Subtitle: artistNames(track.Artists), ImageURL: track.Album.ImageURL,
			}, func() { a.pl.PlayTrack(section.Tracks, i) })
		}
	})
}

func (a *app) wikiTags(c *ui.Context, lib *library, label string, entries []music.WikiEntry) {
	ui.Column(c).Gap(6).PaddingX(6).Children(func() {
		if label != "" {
			ui.Text(c, label).FontSize(12).TextColor(white.Alpha(0.65))
		}
		ui.Row(c).Wrap().Gap(6).Children(func() {
			for i, entry := range entries {
				action := a.wikiAction(lib, entry)
				if action == nil {
					ui.Text(c.Key(i), entry.Title).Padding(4, 9).Radius(10).Background(white.Alpha(0.09)).FontSize(12).TextColor(white)
					continue
				}
				if pillButton(c.Key(i), nil, entry.Title, white.Alpha(0.12), white).Clicked() {
					action()
				}
			}
		})
	})
}

func (a *app) wikiRow(c *ui.Context, entry music.WikiEntry, action func()) {
	content := func() {
		if entry.ImageURL != "" {
			a.cover(c, entry.ImageURL, 112, 44, 6)
		}
		if entry.Style == "fact" {
			ui.Text(c, entry.Title).FontSize(13).TextColor(white.Alpha(0.65)).Width(84)
			ui.Text(c, entry.Text).FontSize(14).TextColor(white).Shrink(1).MinWidth(0)
			return
		}
		ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
			if entry.Title != "" {
				ui.Text(c, entry.Title).FontSize(14).FontWeight(600).TextColor(white)
			}
			if entry.Subtitle != "" {
				ui.Text(c, entry.Subtitle).FontSize(12).TextColor(white.Alpha(0.65))
			}
			if entry.Text != "" {
				ui.Text(c, entry.Text).FontSize(13.5).LineHeight(1.45).TextColor(white.Alpha(0.76))
			}
		})
	}
	if action == nil {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Padding(4, 6).Children(content)
		return
	}
	button := ui.ButtonBase(c).Label(entry.Title).Gap(10).Padding(4, 6).Radius(8).AlignItems(ui.Center)
	if button.Hovered() {
		button.Background(white.Alpha(0.08))
	}
	button.Children(content)
	if button.Clicked() {
		action()
	}
}

func (a *app) wikiAction(lib *library, entry music.WikiEntry) func() {
	var target route
	switch {
	case entry.ArtistID != "" && lib.can("artist"):
		target = route{page: pageArtist, id: entry.ArtistID}
	case entry.PlaylistID != "" && lib.can("playlist"):
		target = route{page: pagePlaylist, id: entry.PlaylistID}
	case entry.ShowID != "" && lib.can("show"):
		target = route{page: pageShow, id: entry.ShowID}
	default:
		link, err := url.Parse(entry.URL)
		if err == nil && knownWikiLink(link) {
			return func() { mygo.Shell.OpenExternal(link.String()) }
		}
		return nil
	}
	target.src = lib.id
	return func() { a.open(target) }
}

func knownWikiLink(link *url.URL) bool {
	if link.User != nil {
		return false
	}
	if link.Scheme == "https" {
		return link.Hostname() != ""
	}
	if link.Scheme == "orpheus" && link.Host == "rnpage" && link.Path == "" {
		switch link.Query().Get("component") {
		case "rn-genre", "rn-tag-detail", "rn-movie-variety":
			return true
		}
	}
	return false
}
