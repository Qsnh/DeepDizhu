package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"

	"deepdizhu/ai"
)

// saved is what the game keeps between launches: the settings, and the
// scores so far.
type saved struct {
	// Version 1 added Cap; files without it keep the default cap.
	Version int         `json:"version"`
	Levels  [3]ai.Level `json:"levels"` // of seats 1 to 3
	Pace    int         `json:"pace"`
	Base    int         `json:"base"`
	Cap     int         `json:"cap"`
	Counter bool        `json:"counter"`
	Scores  [4]int      `json:"scores"`
	Deals   int         `json:"deals"`
}

func (t *table) snapshot() saved {
	return saved{
		Version: 1,
		Levels:  [3]ai.Level(t.set.levels[1:]),
		Pace:    t.set.pace,
		Base:    t.set.base,
		Cap:     t.set.cap,
		Counter: t.set.counter,
		Scores:  t.scores,
		Deals:   t.dealsDone,
	}
}

// restore takes back what was saved, ignoring values out of range, as from
// a file of a later version.
func (t *table) restore(s saved) {
	for i, level := range s.Levels {
		if level >= ai.Easy && level <= ai.Hard {
			t.setLevel(i+1, level)
		}
	}
	if s.Pace >= 0 && s.Pace < len(paces) {
		t.set.pace = s.Pace
	}
	if s.Base >= 0 && s.Base < len(baseScores) {
		t.set.base = s.Base
	}
	if s.Version >= 1 && s.Cap >= 0 && s.Cap < len(caps) {
		t.set.cap = s.Cap
	}
	t.set.counter = s.Counter
	t.scores = s.Scores
	t.dealsDone = max(s.Deals, 0)
	t.lastSaved = t.snapshot()
}

// store keeps what the game saves in a JSON file, writing on a goroutine
// of its own so that the window never waits for the disk.
type store struct {
	path   string
	mu     sync.Mutex // guards latest
	latest saved
	write  sync.Mutex // one write at a time
}

func (s *store) load() (saved, bool) {
	var v saved
	data, err := os.ReadFile(s.path)
	if err != nil || json.Unmarshal(data, &v) != nil {
		return v, false
	}
	return v, true
}

// save writes v in the background. Of saves made close together, the last
// is the one that stays on disk.
func (s *store) save(v saved) {
	s.mu.Lock()
	s.latest = v
	s.mu.Unlock()
	go func() {
		s.write.Lock()
		defer s.write.Unlock()
		s.mu.Lock()
		v := s.latest
		s.mu.Unlock()
		if err := s.writeFile(v); err != nil {
			log.Printf("saving the game: %v", err)
		}
	}()
}

func (s *store) writeFile(v saved) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
