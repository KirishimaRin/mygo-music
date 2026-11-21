package app

import (
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"

	"mygo-music/internal/i18n"
)

func (a *app) listeningPage(c *ui.Context) {
	top, bottom := a.insets(c)
	ui.Scroll(c.Key("listening")).TrackScroll(&a.pageScroll).Grow(1).
		Padding(top+pageGap, pagePad, bottom+24, pagePad).ScrollbarInsets(top, 0, bottom, 0).Children(func() {
		a.startMark(c)
		a.heading(c, T("Listening History"), "")
		ui.Row(c).Gap(24).AlignItems(ui.Start).Children(func() {
			a.listeningCalendar(c)
			a.listeningTimeline(c)
		})
		a.endMark(c)
	})
}

func (a *app) listeningCalendar(c *ui.Context) {
	h, p := &a.listening, a.pal
	day := h.day
	first := time.Date(day.Year(), day.Month(), 1, 0, 0, 0, 0, day.Location())
	last := first.AddDate(0, 1, -1).Day()
	month := i18n.Month(first)
	ui.Column(c).Width(250).Shrink(0).Padding(10).Gap(3).Radius(12).Background(p.hover).Children(func() {
		ui.Row(c).Height(32).Gap(2).AlignItems(ui.Center).Children(func() {
			if iconButton(c.Key("previous"), iconBack, 15, p.muted, T("Previous month")).Clicked() {
				a.stepListeningMonth(-1)
			}
			ui.Text(c, month).Grow(1).TextAlign(ui.Center).FontSize(14).FontWeight(650)
			if iconButton(c.Key("next"), iconForward, 15, p.muted, T("Next month")).Clicked() {
				a.stepListeningMonth(1)
			}
		})
		ui.Row(c).Gap(3).Children(func() {
			for _, name := range []string{"Su", "Mo", "Tu", "We", "Th", "Fr", "Sa"} {
				ui.Text(c, T(name)).Width(30).TextAlign(ui.Center).FontSize(11).TextColor(p.muted)
			}
		})
		for week := 0; week*7 < int(first.Weekday())+last; week++ {
			ui.Row(c.Key(week)).Gap(3).Children(func() {
				for weekday := 0; weekday < 7; weekday++ {
					n := week*7 + weekday + 1 - int(first.Weekday())
					if n < 1 || n > last {
						ui.Box(c).Size(30, 32)
						continue
					}
					at := first.AddDate(0, 0, n-1)
					color := p.text
					b := ui.ButtonBase(c.Key(n)).Size(30, 32).Radius(6).Center().Label(i18n.Date(at)).
						Selected(n == day.Day())
					switch {
					case n == day.Day():
						b.Background(p.accent)
						color = p.onAccent
					case b.Hovered():
						b.Background(p.press)
					}
					b.Children(func() {
						ui.Text(c, strconv.Itoa(n)).FontSize(13).TextColor(color)
						if h.view.Days[n] {
							ui.Box(c).Absolute().Left(13).Bottom(3).Size(4, 4).Radius(2).Background(color)
						}
					})
					if b.Clicked() {
						a.showListeningDay(at)
					}
				}
			})
		}
		if ui.Button(c.Key("today"), T("Today")).AlignSelf(ui.Center).Clicked() {
			a.showListeningDay(time.Now())
		}
	})
}

func (a *app) listeningTimeline(c *ui.Context) {
	h, p := &a.listening, a.pal
	ui.Column(c).Grow(1).MinWidth(0).Gap(4).Children(func() {
		title := i18n.Today(h.day)
		if h.track != nil {
			title = h.track.Title
			if ui.Button(c.Key("day"), T("Back to day")).AlignSelf(ui.Start).Clicked() {
				a.showListeningTrack(nil)
			}
		}
		ui.Text(c, title).FontSize(19).FontWeight(700).Margin(0, 0, 10, 0).MaxLines(2)
		switch {
		case h.failed:
			ui.Text(c, T("Could not load listening history.")).TextColor(p.muted)
			if ui.Button(c.Key("retry"), T("Retry")).AlignSelf(ui.Start).Clicked() {
				a.readListening()
			}
		case len(h.view.Entries) == 0:
			message := T("No plays on this day.")
			if h.loading {
				message = T("Loading…")
			}
			ui.Text(c, message).FontSize(13).TextColor(p.muted)
		}
		for _, entry := range h.view.Entries {
			when := entry.Started.Format("15:04")
			duration := clock(entry.Heard)
			if entry.Heard < time.Second {
				duration = "<0:01"
			}
			if h.track != nil {
				when = entry.Started.Format("2006-01-02  15:04")
				ui.Row(c.Key(entry.ID)).Height(40).PaddingX(8).Gap(12).AlignItems(ui.Center).Children(func() {
					ui.Text(c, when).FontSize(13).Grow(1)
					ui.Text(c, duration).FontSize(12).TextColor(p.muted).Shrink(0)
				})
				continue
			}
			b := ui.ButtonBase(c.Key(entry.ID)).Label(entry.Title).Height(46).PaddingX(8).
				Radius(7).Gap(12).AlignItems(ui.Center)
			if b.Hovered() {
				b.Background(p.hover)
			}
			b.Children(func() {
				ui.Text(c, when).FontSize(12).TextColor(p.muted).Shrink(0)
				ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
					ui.Text(c, entry.Title).FontSize(14).FontWeight(600).MaxLines(1)
					if entry.Artists != "" {
						ui.Text(c, entry.Artists).FontSize(12).TextColor(p.muted).MaxLines(1)
					}
				})
				ui.Text(c, duration).FontSize(12).TextColor(p.muted).Shrink(0)
			})
			if b.Clicked() {
				a.showListeningTrack(&entry)
			}
		}
		if h.view.More && !h.failed && ui.Button(c.Key("more"), T("Show more")).Disabled(h.loading).AlignSelf(ui.Start).Clicked() {
			h.limit += listeningPageSize
			a.readListening()
		}
	})
}
