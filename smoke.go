//go:build smoke

package main

import (
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"

	"deepdizhu/ai"
	"deepdizhu/game"
)

// Built with -tags smoke, the app plays a deal in its window by itself,
// taking the hints for you, saves screenshots of the window to the
// directory SMOKE_SHOTS names, and quits: a check of the app as it runs,
// with its threads and its window, rather than in a tester.
func init() {
	smoke = func(t *table, win *mygo.Window) {
		dir := os.Getenv("SMOKE_SHOTS")
		shot := func(name string) {
			done := make(chan struct{})
			win.Update(func() {
				defer close(done)
				png, err := win.CapturePage()
				if err != nil {
					log.Printf("smoke: capture %s: %v", name, err)
					return
				}
				if dir != "" {
					os.WriteFile(filepath.Join(dir, name+".png"), png, 0o644)
				}
			})
			<-done
		}
		step := func(fn func()) {
			done := make(chan struct{})
			win.Update(func() { fn(); close(done) })
			<-done
		}
		time.Sleep(time.Second)
		shot("smoke-lobby")
		// The heaviest table: three Hard players, at the fastest pace.
		step(func() {
			for seat := 1; seat < 4; seat++ {
				t.setLevel(seat, ai.Hard)
			}
			t.set.pace = len(paces) - 1
			t.start()
		})
		deadline := time.Now().Add(3 * time.Minute)
		plays, shotPlaying := 0, false
		for time.Now().Before(deadline) {
			time.Sleep(150 * time.Millisecond)
			var finished bool
			step(func() {
				finished = t.g.Phase == game.PhaseFinished
				if finished || !t.myTurn() {
					return
				}
				if t.g.Phase == game.PhaseBidding {
					t.bid(human, 3)
					return
				}
				t.hint()
				if _, ok := t.selection(); ok {
					t.playSelected()
				} else {
					t.pass()
				}
				plays++
			})
			if plays == 3 && !shotPlaying {
				shotPlaying = true
				shot("smoke-playing")
			}
			if finished {
				time.Sleep(700 * time.Millisecond)
				shot("smoke-result")
				log.Printf("smoke: deal finished, scores %v", t.scores)
				mygo.App.Quit()
				return
			}
		}
		log.Print("smoke: the deal did not finish in time")
		mygo.App.Exit(1)
	}
}
