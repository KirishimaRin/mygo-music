package app

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"mygo-music/internal/music"
)

type cloudData struct {
	query  string
	tracks []music.Track
	next   int
	at     time.Time
	paged
}

func (a *app) loadCloud(lib *library, query string, next bool) {
	src, ok := lib.src.(music.CloudSource)
	if !ok || !lib.can("cloudTracks") || lib.signedOut() {
		return
	}
	d := lib.cloud
	if d == nil || d.query != query || !next && d.loaded && !d.loading && time.Since(d.at) > listsFresh {
		d = &cloudData{query: query}
		lib.cloud = d
	}
	if d.loading || d.loaded && !next {
		return
	}
	offset := d.next
	d.loading, d.err = true, ""
	go func() {
		page, err := answered(src.CloudTracks(offset, query))
		a.post(func() {
			if lib.cloud != d {
				return
			}
			d.loading = false
			if err != nil {
				d.err = a.problem(lib, err, T("Couldn't load your music cloud"))
				return
			}
			d.tracks = append(d.tracks, page.Tracks...)
			d.next = page.Next
			d.loaded, d.more = true, page.Next > offset
			if offset == 0 {
				d.at = time.Now()
			}
		})
	}()
}

func (a *app) cloudPage(c *ui.Context) {
	lib, query := a.library, a.route.query
	if lib.signedOut() {
		ui.Column(c).Grow(1).Center().Children(func() {
			if ui.PrimaryButton(c.Key("sign-in"), T("Sign In")).Clicked() {
				a.promptSignIn(lib)
			}
		})
		return
	}
	d := lib.cloud
	if d == nil {
		return
	}
	a.trackRows(c, d.tracks, true, func(c *ui.Context) {
		ui.Column(c).PaddingX(12).Gap(16).Children(func() {
			a.heading(c, a.builtinTitle("cloudTracks"), "")
			label := T("Search Music Cloud")
			in := ui.TextInput(c.Key("cloud-query"), &a.cloudQuery).Bind(&a.cloudSearchBox).
				Width(320).MaxWidthPercent(100).Label(label).Placeholder(label).FontSize(14)
			if in.Submitted() {
				q := strings.TrimSpace(a.cloudQuery)
				if q != query {
					a.open(route{page: pageCloud, query: q})
				}
			}
		})
	}, func(c *ui.Context) {
		if d.loaded && !d.more && len(d.tracks) == 0 {
			message := T("No songs in your music cloud.")
			if query != "" {
				message = T("No matching cloud songs.")
			}
			a.empty(c, message)
		}
		a.pageFoot(c, &d.paged, func() { a.loadCloud(lib, query, true) })
	})
}
