// Package listening keeps the listening history in the app's user data.
package listening

import (
	"database/sql"
	"errors"
	"log"
	"os"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Track identifies a track within its source.
type Track struct {
	Source string
	ID     string
}

type Entry struct {
	ID      string
	Track   Track
	Title   string
	Artists string
	Started time.Time
	Ended   time.Time
	Heard   time.Duration
	Reason  string
}

type View struct {
	Days    [32]bool
	Entries []Entry
	More    bool
}

type operation func(*sql.DB, error)

// Store serializes inserts and reads on one worker. Scheduling never waits
// for disk I/O, and a read observes the writes scheduled before it.
type Store struct {
	mu     sync.Mutex
	wake   *sync.Cond
	queue  []operation
	closed bool
	done   chan struct{}
}

func Open(path string) *Store {
	s := &Store{done: make(chan struct{})}
	s.wake = sync.NewCond(&s.mu)
	go s.run(path)
	return s
}

// Close finishes accepted work and closes the database. Callbacks on the
// worker must return without waiting for the main thread.
func (s *Store) Close() {
	s.mu.Lock()
	s.closed = true
	s.wake.Signal()
	s.mu.Unlock()
	<-s.done
}

func (s *Store) schedule(job operation) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	s.queue = append(s.queue, job)
	s.wake.Signal()
	return true
}

func (s *Store) run(path string) {
	defer close(s.done)
	db, err := openDB(path)
	if err == nil {
		defer db.Close()
	}
	if err != nil {
		log.Printf("opening listening history: %v", err)
	}
	for {
		s.mu.Lock()
		for len(s.queue) == 0 && !s.closed {
			s.wake.Wait()
		}
		if len(s.queue) == 0 {
			s.mu.Unlock()
			return
		}
		jobs := s.queue
		s.queue = nil
		s.mu.Unlock()
		for _, job := range jobs {
			job(db, err)
		}
	}
}

func openDB(path string) (*sql.DB, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	if err := file.Close(); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

const schema = `
PRAGMA busy_timeout = 500;
CREATE TABLE IF NOT EXISTS plays (
	id TEXT NOT NULL PRIMARY KEY,
	source TEXT NOT NULL,
	track TEXT NOT NULL,
	title TEXT NOT NULL,
	artists TEXT NOT NULL,
	started INTEGER NOT NULL,
	ended INTEGER NOT NULL,
	heard INTEGER NOT NULL CHECK (heard > 0),
	reason TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS plays_by_date ON plays(started DESC, id DESC);
CREATE INDEX IF NOT EXISTS plays_by_track ON plays(source, track, started DESC, id DESC);
`

func (s *Store) Append(e Entry) {
	heard := e.Heard.Milliseconds()
	if heard <= 0 {
		return
	}
	s.schedule(func(db *sql.DB, err error) {
		if err == nil {
			_, err = db.Exec(`INSERT INTO plays VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT(id) DO NOTHING`, e.ID, e.Track.Source, e.Track.ID,
				e.Title, e.Artists, e.Started.UnixMilli(), e.Ended.UnixMilli(), heard, e.Reason)
		}
		if err != nil {
			log.Printf("saving listening history: %v", err)
		}
	})
}

// Read reports the selected month's marked days and a page of the selected
// day's plays. For a track, the list is instead that track's entire history.
// The callback runs on the worker. day.Location is the viewer's time zone.
func (s *Store) Read(day time.Time, track *Track, limit int, ready func(View, error)) {
	if !s.schedule(func(db *sql.DB, err error) {
		if err != nil {
			ready(View{}, err)
			return
		}
		v, err := read(db, day, track, limit)
		ready(v, err)
	}) {
		ready(View{}, errors.New("listening history is closed"))
	}
}

func read(db *sql.DB, day time.Time, track *Track, limit int) (v View, err error) {
	year, month, date := day.Date()
	loc := day.Location()
	start := time.Date(year, month, date, 0, 0, 0, 0, loc)
	columns := `SELECT id, source, track, title, artists, started, ended, heard, reason FROM plays`
	order := ` ORDER BY started DESC, id DESC LIMIT ?`
	var rows *sql.Rows
	if track == nil {
		first := time.Date(year, month, 1, 0, 0, 0, 0, loc)
		v.Days, err = markedDays(db, first)
		if err != nil {
			return v, err
		}
		rows, err = db.Query(columns+` WHERE started >= ? AND started < ?`+order,
			start.UnixMilli(), start.AddDate(0, 0, 1).UnixMilli(), limit+1)
	} else {
		rows, err = db.Query(columns+` WHERE source = ? AND track = ?`+order,
			track.Source, track.ID, limit+1)
	}
	if err != nil {
		return v, err
	}
	defer rows.Close()
	for rows.Next() {
		if len(v.Entries) == limit {
			v.More = true
			break
		}
		var e Entry
		var started, ended, heard int64
		if err := rows.Scan(&e.ID, &e.Track.Source, &e.Track.ID, &e.Title, &e.Artists,
			&started, &ended, &heard, &e.Reason); err != nil {
			return v, err
		}
		e.Started = time.UnixMilli(started).In(loc)
		e.Ended = time.UnixMilli(ended).In(loc)
		e.Heard = time.Duration(heard) * time.Millisecond
		v.Entries = append(v.Entries, e)
	}
	return v, rows.Err()
}

func markedDays(db *sql.DB, first time.Time) (days [32]bool, err error) {
	rows, err := db.Query(`SELECT started FROM plays WHERE started >= ? AND started < ?`,
		first.UnixMilli(), first.AddDate(0, 1, 0).UnixMilli())
	if err != nil {
		return days, err
	}
	defer rows.Close()
	for rows.Next() {
		var started int64
		if err := rows.Scan(&started); err != nil {
			return days, err
		}
		days[time.UnixMilli(started).In(first.Location()).Day()] = true
	}
	return days, rows.Err()
}
