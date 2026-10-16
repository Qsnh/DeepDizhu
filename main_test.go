package main

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"deepdizhu/ai"
	"deepdizhu/game"
)

// queueRunner keeps the computer players' moves until the test runs them,
// at once and on the test's goroutine.
type queueRunner struct{ jobs []func() func() }

func (q *queueRunner) Go(_ time.Duration, work func() func()) { q.jobs = append(q.jobs, work) }

// next runs the oldest move waiting, and reports whether there was one.
func (q *queueRunner) next() bool {
	if len(q.jobs) == 0 {
		return false
	}
	work := q.jobs[0]
	q.jobs = q.jobs[1:]
	work()()
	return true
}

func newTestTable(seed uint64) (*table, *queueRunner, *ui.Tester) {
	q := &queueRunner{}
	tb := newTable(q, seed)
	tt := ui.NewTester(tb.view, 1280, 820)
	// Nothing moves, so that frames settle at once.
	tt.SetPreferences(ui.Preferences{ReduceMotion: true, TextScale: 1})
	return tb, q, tt
}

// snapshot saves the tester's frame when SNAPSHOTS names a directory, to
// look at the table without opening the app.
func snapshot(t *testing.T, tt *ui.Tester, name string) {
	dir := os.Getenv("SNAPSHOTS")
	if dir == "" {
		return
	}
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, tt.Image()); err != nil {
		t.Fatal(err)
	}
}

func TestLobbyStartsTheGame(t *testing.T) {
	tb, _, tt := newTestTable(1)
	snapshot(t, tt, "lobby")
	if !tt.HasText("开始游戏") {
		t.Fatalf("no start button in %q", tt.Texts())
	}
	if err := tt.Click("开始游戏"); err != nil {
		t.Fatal(err)
	}
	if !tb.started || tb.g == nil || tb.g.Phase != game.PhaseBidding {
		t.Fatalf("started %v, game %v", tb.started, tb.g)
	}
	if n := len(tb.g.Hands[human]); n != 25 {
		t.Errorf("dealt %d cards to you, want 25", n)
	}
}

func TestLevelsChosenInTheLobby(t *testing.T) {
	tb, _, tt := newTestTable(1)
	// The segmented controls are in the order of the seats; choose 困难
	// for each.
	for range 3 {
		if err := tt.Click("困难"); err != nil {
			t.Fatal(err)
		}
	}
	hard := 0
	for seat := 1; seat < 4; seat++ {
		if tb.set.levels[seat] == ai.Hard {
			hard++
		}
	}
	if hard == 0 {
		t.Errorf("levels %v after choosing 困难", tb.set.levels)
	}
}

// playDeal plays a deal to its end through the buttons, with you taking
// the hints, and returns how many of your turns it took.
func playDeal(t *testing.T, tb *table, q *queueRunner, tt *ui.Tester, bid string) int {
	t.Helper()
	turns := 0
	for steps := 0; steps < 2000; steps++ {
		if tb.g.Phase == game.PhaseFinished {
			return turns
		}
		if !tb.myTurn() {
			if !q.next() {
				t.Fatalf("nothing to do in phase %v, turn %d", tb.g.Phase, tb.g.Turn)
			}
			tt.Frame()
			continue
		}
		turns++
		if tb.g.Phase == game.PhaseBidding {
			label := bid
			if b, ok := tt.Find(label); !ok || b.W == 0 || tb.g.HighestBid >= 3 {
				label = "不叫"
			}
			if err := tt.Click(label); err != nil {
				t.Fatal(err)
			}
			if tb.g.Turn == human && tb.g.Phase == game.PhaseBidding {
				// The bid was too low; pass instead.
				if err := tt.Click("不叫"); err != nil {
					t.Fatal(err)
				}
			}
			continue
		}
		if err := tt.Click("提示"); err != nil {
			t.Fatal(err)
		}
		if _, ok := tb.selection(); ok {
			if err := tt.Click("出牌"); err != nil {
				t.Fatal(err)
			}
		} else if err := tt.Click("不出"); err != nil {
			t.Fatal(err)
		}
		if tb.myTurn() && tb.g.Phase == game.PhasePlaying {
			t.Fatalf("still your turn after acting; notice %q", tb.notice)
		}
	}
	t.Fatal("the deal did not end")
	return 0
}

func TestPlayDealsThroughTheButtons(t *testing.T) {
	tb, q, tt := newTestTable(7)
	tb.set.pace = 2
	if err := tt.Click("开始游戏"); err != nil {
		t.Fatal(err)
	}
	snapshot(t, tt, "bidding")
	for deal, bid := range []string{"3 分", "不叫", "1 分"} {
		playDeal(t, tb, q, tt, bid)
		r := tb.g.Result
		if r == nil || !tb.resultOpen {
			t.Fatalf("deal %d: no result shown", deal)
		}
		sum := 0
		for _, s := range r.Scores {
			sum += s
		}
		if sum != 0 {
			t.Errorf("deal %d: scores %v do not add up to zero", deal, r.Scores)
		}
		if !tt.HasText("再来一局") {
			t.Fatalf("deal %d: no 再来一局 in %q", deal, tt.Texts())
		}
		if deal == 0 {
			snapshot(t, tt, "result")
		}
		if err := tt.Click("再来一局"); err != nil {
			t.Fatal(err)
		}
		if tb.g.Phase != game.PhaseBidding || tb.resultOpen {
			t.Fatalf("deal %d: phase %v after 再来一局", deal, tb.g.Phase)
		}
	}
	total := 0
	for _, s := range tb.scores {
		total += s
	}
	if total != 0 || tb.dealsDone != 3 {
		t.Errorf("scores %v over %d deals", tb.scores, tb.dealsDone)
	}
}

func TestSelectionEnablesPlay(t *testing.T) {
	tb, q, tt := newTestTable(11)
	tt.Click("开始游戏")
	// Bid 3 when it comes to you, so that you lead.
	for steps := 0; tb.g.Phase == game.PhaseBidding; steps++ {
		if steps > 20 {
			t.Fatalf("the bidding goes on: bids %v", tb.g.Bids)
		}
		if tb.myTurn() {
			if err := tt.Click("3 分"); err != nil {
				t.Fatal(err)
			}
			continue
		}
		q.next()
		tt.Frame()
	}
	if tb.g.Landlord != human {
		t.Skipf("someone bid 3 before you, landlord %d", tb.g.Landlord)
	}
	if len(tb.g.Hands[human]) != 33 || len(tb.fresh) != 8 {
		t.Fatalf("landlord holds %d cards, %d fresh", len(tb.g.Hands[human]), len(tb.fresh))
	}
	snapshot(t, tt, "landlord")
	// Select the lowest card by clicking it, then the button plays it.
	low := tb.g.Hands[human][len(tb.g.Hands[human])-1]
	box, ok := tt.Find(low.String())
	if !ok {
		t.Fatalf("no card %v on the table", low)
	}
	tt.ClickAt(box.X+box.W-10, box.Y+box.H/2)
	// Two decks have two of each card: either may be the one clicked.
	if sel := tb.selectedCards(); len(sel) != 1 || sel[0].String() != low.String() {
		t.Fatalf("clicking %v selected %v", low, sel)
	}
	if _, ok := tb.selection(); !ok {
		t.Fatal("a single card is not a play")
	}
	snapshot(t, tt, "selected")
	if err := tt.Click("出牌"); err != nil {
		t.Fatal(err)
	}
	if len(tb.g.Hands[human]) != 32 || tb.shown[human].kind != showPlay {
		t.Errorf("after playing: %d cards, shown %v", len(tb.g.Hands[human]), tb.shown[human])
	}
	q.next()
	tt.Frame()
	tt.Advance(time.Second)
	snapshot(t, tt, "playing")
}

func TestSettingsDuringAGame(t *testing.T) {
	tb, _, tt := newTestTable(5)
	tt.Click("开始游戏")
	if err := tt.Click("设置"); err != nil {
		t.Fatal(err)
	}
	if !tb.settingsOpen || !tt.HasText("封顶") || !tt.HasText("出牌速度") {
		t.Fatalf("settings not shown: %q", tt.Texts())
	}
	snapshot(t, tt, "settings")
	if err := tt.Click("128 倍"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("完成"); err != nil {
		t.Fatal(err)
	}
	if tb.settingsOpen || caps[tb.set.cap] != 128 {
		t.Errorf("open %v, cap %d", tb.settingsOpen, caps[tb.set.cap])
	}
	// The cap applies from the next deal on.
	if tb.g.Config.MaxMultiplier != 64 {
		t.Errorf("this deal's cap changed to %d", tb.g.Config.MaxMultiplier)
	}
	if err := tt.Click("规则"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("四人斗地主规则") {
		t.Fatalf("rules not shown: %q", tt.Texts())
	}
	snapshot(t, tt, "rules")
	tt.Click("知道了")
	if tb.rulesOpen {
		t.Error("rules still open")
	}
}
