// DeepDizhu is four-player Dou Dizhu (斗地主) against three computer
// players, each of which plays at a level you choose, drawn in native UI
// with MyGo.
package main

import (
	"log"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// windowRunner thinks on a goroutine of its own and delivers the move to
// the window's UI thread, no sooner than the delay.
type windowRunner struct{ win *mygo.Window }

func (r *windowRunner) Go(delay time.Duration, work func() func()) {
	go func() {
		start := time.Now()
		apply := work()
		if wait := delay - time.Since(start); wait > 0 {
			time.Sleep(wait)
		}
		r.win.Update(apply)
	}()
}

// smoke, when built with the smoke tag, plays the app by itself in its
// window (see smoke.go).
var smoke func(t *table, win *mygo.Window)

func main() {
	run := &windowRunner{}
	t := newTable(run, uint64(time.Now().UnixNano()))
	mygo.App.WhenReady(func() {
		// A smoke run keeps nothing, so as not to leave its scores behind.
		if dir, err := mygo.App.Path(mygo.PathUserData); err == nil && smoke == nil {
			st := &store{path: filepath.Join(dir, "deepdizhu.json")}
			if s, ok := st.load(); ok {
				t.restore(s)
			}
			t.persist = st.save
		}
		run.win = mygo.NewWindow(mygo.WindowOptions{
			Title:           "斗地主",
			Width:           1280,
			Height:          820,
			MinWidth:        1120,
			MinHeight:       720,
			StateKey:        "main",
			TitleBarStyle:   mygo.TitleBarHidden,
			BackgroundColor: "#0d1724",
			Content:         ui.View(t.view),
		})
	})
	if smoke != nil {
		mygo.App.WhenReady(func() { go smoke(t, run.win) })
	}
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
