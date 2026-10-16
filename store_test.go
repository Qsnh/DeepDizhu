package main

import (
	"path/filepath"
	"testing"
	"time"

	"deepdizhu/ai"
)

func TestSettingsAndScoresKeep(t *testing.T) {
	st := &store{path: filepath.Join(t.TempDir(), "deepdizhu.json")}
	tb := newTable(&queueRunner{}, 3)
	tb.setLevel(1, ai.Hard)
	tb.setLevel(3, ai.Easy)
	tb.set.pace, tb.set.base, tb.set.cap, tb.set.counter = 2, 1, 0, false
	tb.scores = [4]int{6, -2, -2, -2}
	tb.dealsDone = 4
	st.save(tb.snapshot())

	// The write happens in the background.
	var got saved
	deadline := time.Now().Add(5 * time.Second)
	for {
		var ok bool
		if got, ok = st.load(); ok || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	other := newTable(&queueRunner{}, 4)
	other.restore(got)
	if other.snapshot() != tb.snapshot() {
		t.Errorf("restored %+v, saved %+v", other.snapshot(), tb.snapshot())
	}
	if other.dealNumber() != 5 {
		t.Errorf("deal number %d after 4 deals", other.dealNumber())
	}
}

func TestRestoreIgnoresValuesOutOfRange(t *testing.T) {
	tb := newTable(&queueRunner{}, 3)
	tb.restore(saved{Version: 1, Levels: [3]ai.Level{7, -1, ai.Hard}, Pace: 9, Base: -3, Cap: 9, Deals: -5})
	if tb.set.levels[1] != ai.Normal || tb.set.levels[3] != ai.Hard || tb.set.pace != 1 || tb.set.base != 0 ||
		tb.set.cap != defaultCap || tb.dealsDone != 0 {
		t.Errorf("settings %+v, deals %d", tb.set, tb.dealsDone)
	}
}

func TestSavesBeforeTheCapKeepTheDefaultCap(t *testing.T) {
	tb := newTable(&queueRunner{}, 3)
	tb.restore(saved{Levels: [3]ai.Level{ai.Easy, ai.Easy, ai.Easy}, Counter: true})
	if tb.set.cap != defaultCap || tb.set.levels[2] != ai.Easy {
		t.Errorf("settings %+v", tb.set)
	}
}
