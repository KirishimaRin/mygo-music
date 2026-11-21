package listening

import (
	"path/filepath"
	"testing"
	"time"
)

// Pending plays survive close, and repeated writes of one session stay one row.
func TestHistorySurvivesClose(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.db")
	s := Open(path)
	zone := time.FixedZone("UTC+8", 8*60*60)
	start := time.Date(2026, 11, 20, 23, 59, 30, 0, zone)
	one := Entry{"one", Track{"local", "42"}, "Name", "Artist", start, start.Add(time.Minute), time.Minute, "finished"}
	two := one
	two.ID = "two"
	two.Started = start.Add(time.Minute)
	two.Ended = two.Started.Add(time.Minute)
	other := two
	other.ID, other.Track.Source = "other", "plugin"
	s.Append(one)
	s.Append(one)
	s.Append(two)
	s.Append(other)
	s.Close()

	s = Open(path)
	defer s.Close()
	day := historyResult(t, s, start, nil, 2)
	if len(day.Entries) != 1 || day.Entries[0] != one || day.More || !day.Days[20] || !day.Days[21] {
		t.Fatalf("first day and month markers: %+v", day)
	}
	tomorrow := historyResult(t, s, start.AddDate(0, 0, 1), nil, 4)
	if len(tomorrow.Entries) != 2 {
		t.Fatalf("following day: %+v", tomorrow)
	}
	track := historyResult(t, s, start, &one.Track, 1)
	if len(track.Entries) != 1 || track.Entries[0] != two || !track.More {
		t.Fatalf("track paging and source isolation: %+v", track)
	}
}

func historyResult(t *testing.T, s *Store, day time.Time, track *Track, limit int) View {
	t.Helper()
	type result struct {
		View
		error
	}
	ready := make(chan result, 1)
	s.Read(day, track, limit, func(v View, err error) { ready <- result{v, err} })
	select {
	case r := <-ready:
		if r.error != nil {
			t.Fatal(r.error)
		}
		return r.View
	case <-time.After(2 * time.Second):
		t.Fatal("listening history was not read")
		return View{}
	}
}
