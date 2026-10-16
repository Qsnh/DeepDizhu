package ai

import (
	"math/rand/v2"
	"strings"
	"sync"
	"testing"
	"time"

	"deepdizhu/game"
)

// table builds a game from rank specs (distinct card ids across seats),
// makes seat 0 the landlord with a bid of 1 and the given bottom, then runs
// the scripted plays ("" = pass).
func table(t *testing.T, hands [4]string, bottom string, script ...string) *game.Game {
	t.Helper()
	all, err := game.CardsFromSpec(strings.Join(hands[:], "") + bottom)
	if err != nil {
		t.Fatal(err)
	}
	var hs [4][]game.Card
	i := 0
	for s, h := range hands {
		n := len(strings.ReplaceAll(h, " ", ""))
		hs[s] = all[i : i+n]
		i += n
	}
	g := game.NewGameFromDeal(hs, all[i:], 0, game.Config{})
	for s, b := range []int{1, 0, 0, 0} {
		if err := g.Bid(s, b); err != nil {
			t.Fatal(err)
		}
	}
	for _, spec := range script {
		s := g.Turn
		var cards []game.Card
		if spec != "" {
			want, err := game.CardsFromSpec(spec)
			if err != nil {
				t.Fatal(err)
			}
			need := game.CountRanks(want)
			for _, c := range g.Hands[s] {
				if need[c.Rank()] > 0 {
					need[c.Rank()]--
					cards = append(cards, c)
				}
			}
		}
		if err := g.Play(s, cards); err != nil {
			t.Fatalf("script %q by seat %d: %v", spec, s, err)
		}
	}
	return g
}

func ranks(cards []game.Card) string {
	c := game.CountRanks(cards)
	return c.String()
}

func expectPlay(t *testing.T, name string, p Player, g *game.Game, want string) {
	t.Helper()
	got := p.Play(g.View(g.Turn))
	w, _ := game.CardsFromSpec(want)
	if ranks(got) != ranks(w) {
		t.Errorf("%s: seat %d played [%s], want [%s]", name, g.Turn, ranks(got), ranks(w))
	}
}

func TestNormalScenarios(t *testing.T) {
	n := New(Normal, 1)

	g := table(t, [4]string{"33344", "5", "6", "7"}, "")
	expectPlay(t, "whole hand at once", n, g, "33344")

	// A farmer does not overtake a teammate when it cannot go out.
	g = table(t, [4]string{"6TJQKA", "358", "9JQ3", "47"}, "", "6", "8")
	expectPlay(t, "no overtaking a teammate", n, g, "")

	// Beat a one-card opponent's single with the highest card.
	g = table(t, [4]string{"57", "69K233", "4", "8"}, "", "5")
	expectPlay(t, "stop the last card", n, g, "2")

	// Lead something other than a single when the landlord has one card.
	g = table(t, [4]string{"47", "8 35 99 K", "66T", "TT2"}, "", "4", "8", "", "", "")
	expectPlay(t, "no single against one card", n, g, "99")

	// Feed a teammate who has one card left.
	g = table(t, [4]string{"68TJQ", "K 3 55 77", "A", "99"}, "", "6", "K", "", "", "")
	expectPlay(t, "feed the teammate", n, g, "3")

	// Bomb when it lets the hand go out right after.
	g = table(t, [4]string{"K456", "7777 33", "8", "9"}, "", "K")
	expectPlay(t, "bomb to finish", n, g, "7777")

	// Not every bomb: keep it when far from the end and nobody is close.
	g = table(t, [4]string{"K456789TJQA", "7777 3 5 9 J 8 6 Q", "88 9", "99TT"}, "", "K")
	expectPlay(t, "keep the bomb", n, g, "")

	// Planes lead with the lowest loose cards as kickers.
	g = table(t, [4]string{"QQQ3 KKK", "4", "5", "6"}, "4 9")
	expectPlay(t, "plane with low kickers", n, g, "QQQKKK34")
}

func TestBidding(t *testing.T) {
	strong, _ := game.CardsFromSpec("XXxx222222AAAKKKKQQQ34567")
	// The weakest of a few hundred random hands.
	var weak []game.Card
	weakest := 1 << 30
	wrng := rand.New(rand.NewPCG(8, 8))
	for i := 0; i < 300; i++ {
		g := game.NewGame(wrng, 0, game.Config{})
		h := game.CountRanks(g.Hands[0])
		if st := handStrength(&h); st < weakest {
			weakest, weak = st, g.Hands[0]
		}
	}
	for _, l := range []Level{Normal, Hard} {
		p := makePlayer(l, 1)
		if l == Hard {
			p = NewHard(1, HardOptions{BidRollouts: 200, Workers: 1})
		}
		for _, tc := range []struct {
			hand []game.Card
			want int
		}{{strong, 3}, {weak, 0}} {
			v := game.View{Seat: 0, Phase: game.PhaseBidding, Hand: tc.hand, HandCounts: [4]int{25, 25, 25, 25}, Landlord: -1, LastSeat: -1, Bids: [4]int{-1, -1, -1, -1}}
			if b := p.Bid(v); b != tc.want {
				t.Errorf("%v bids %d on %s, want %d", l, b, ranks(tc.hand), tc.want)
			}
			v.HighestBid = 3
			if b := p.Bid(v); b != 0 {
				t.Errorf("%v bids %d over 3", l, b)
			}
		}
	}
	// Bids are always legal, for every level and situation.
	rng := rand.New(rand.NewPCG(1, 1))
	for i := 0; i < 50; i++ {
		g := game.NewGame(rng, i%4, game.Config{})
		for _, l := range Levels {
			v := g.View(g.Turn)
			v.HighestBid = rng.IntN(3)
			b := makePlayer(l, uint64(i)).Bid(v)
			if b != 0 && (b <= v.HighestBid || b > 3) {
				t.Fatalf("%v made illegal bid %d over %d", l, b, v.HighestBid)
			}
		}
	}
}

// randomPosition plays a random deal forward with random legal moves and
// returns it at a random point of the playing phase.
func randomPosition(rng *rand.Rand) *game.Game {
	for {
		g := game.NewGame(rng, rng.IntN(4), game.Config{})
		for g.Phase == game.PhaseBidding {
			ch := g.BidChoices()
			if err := g.Bid(g.Turn, ch[rng.IntN(len(ch))]); err != nil {
				panic(err)
			}
		}
		if g.Phase != game.PhasePlaying {
			continue
		}
		steps := rng.IntN(60)
		for i := 0; i < steps && g.Phase == game.PhasePlaying; i++ {
			s := g.Turn
			if g.CanPass(s) && rng.IntN(2) == 0 {
				_ = g.Play(s, nil)
				continue
			}
			ms := g.LegalMoves(s)
			if len(ms) == 0 {
				_ = g.Play(s, nil)
				continue
			}
			_ = g.Play(s, ms[rng.IntN(len(ms))].Cards)
		}
		if g.Phase == game.PhasePlaying {
			return g
		}
	}
}

func TestPlayersAlwaysLegal(t *testing.T) {
	rng := rand.New(rand.NewPCG(2, 2))
	players := map[string]Player{
		"easy":   New(Easy, 1),
		"normal": New(Normal, 1),
		"hard":   NewHard(1, HardOptions{Budget: 5 * time.Millisecond, MaxRollouts: 120, Candidates: 6, Workers: 2, BidRollouts: -1}),
	}
	n := 300
	if testing.Short() {
		n = 60
	}
	for i := 0; i < n; i++ {
		g := randomPosition(rng)
		v := g.View(g.Turn)
		for name, p := range players {
			c := g.Clone()
			if err := c.Play(c.Turn, p.Play(v)); err != nil {
				t.Fatalf("%s made an illegal play: %v", name, err)
			}
		}
		hint := Hint(v)
		if v.LastCombo == nil && len(hint) == 0 {
			t.Fatal("Hint is empty when leading")
		}
		if err := g.Clone().Play(g.Turn, hint); err != nil {
			t.Fatalf("illegal hint: %v", err)
		}
		// Not the viewer's turn: nothing to do.
		other := g.View((g.Turn + 1) % 4)
		if players["normal"].Play(other) != nil || Hint(other) != nil {
			t.Fatal("played out of turn")
		}
	}
}

func TestEasyBehaviour(t *testing.T) {
	e := New(Easy, 7)
	passes, plays := 0, 0
	for i := 0; i < 200; i++ {
		g := table(t, [4]string{"3456789", "TJQKA2X", "4", "5"}, "", "3")
		got := e.Play(g.View(1))
		if len(got) == 0 {
			passes++
		} else {
			plays++
		}
		lead := table(t, [4]string{"3456789", "TJQKA2X", "4", "5"}, "")
		if len(e.Play(lead.View(0))) == 0 {
			t.Fatal("Easy passed while leading")
		}
	}
	if passes < 30 || passes > 90 {
		t.Errorf("Easy passed %d/200 times when it could follow", passes)
	}
}

func TestConcurrentCalls(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 3))
	var views []game.View
	for i := 0; i < 8; i++ {
		g := randomPosition(rng)
		views = append(views, g.View(g.Turn))
	}
	ps := []Player{New(Easy, 1), New(Normal, 1), NewHard(1, HardOptions{Budget: 5 * time.Millisecond, MaxRollouts: 100, Workers: 2, BidRollouts: -1})}
	var wg sync.WaitGroup
	for _, p := range ps {
		for _, v := range views {
			wg.Add(2)
			go func() { defer wg.Done(); p.Play(v) }()
			go func() { defer wg.Done(); Hint(v) }()
		}
	}
	wg.Wait()
}

func TestHardRespectsBudget(t *testing.T) {
	h := NewHard(9, HardOptions{Budget: 60 * time.Millisecond, Workers: 2, BidRollouts: -1})
	rng := rand.New(rand.NewPCG(4, 4))
	worst := time.Duration(0)
	for i := 0; i < 15; i++ {
		g := randomPosition(rng)
		start := time.Now()
		h.Play(g.View(g.Turn))
		worst = max(worst, time.Since(start))
	}
	if worst > 400*time.Millisecond {
		t.Errorf("Hard took %v with a 60ms budget", worst)
	}
	o := HardOptions{}.withDefaults()
	if o.Budget > 600*time.Millisecond || o.MaxRollouts <= 0 || o.Workers <= 0 {
		t.Errorf("bad defaults %+v", o)
	}
}

func TestLevelNames(t *testing.T) {
	want := []string{"简单", "普通", "困难"}
	for i, l := range Levels {
		if l.String() != want[i] {
			t.Errorf("%d: %q", i, l.String())
		}
	}
}

func TestDecomposition(t *testing.T) {
	cases := []struct {
		spec string
		want []string // group descriptions expected in the decomposition
	}{
		{"3456789", []string{"顺子 3-9"}},
		{"33344455566", []string{"飞机 3-5", "对子 6"}},
		{"3333 45678 xxXX", []string{"炸弹×4 3", "顺子 4-8", "天王炸"}},
		{"334455 99", []string{"连对 3-5", "对子 9"}},
	}
	var dz decomposer
	for _, tc := range cases {
		cs, _ := game.CardsFromSpec(tc.spec)
		h := game.CountRanks(cs)
		var dc decomp
		dz.decompose(&h, true, fullLimit, &dc)
		got := map[string]bool{}
		var all counts
		for _, g := range dc.groups() {
			got[g.move().Describe()] = true
			g.addTo(&all)
		}
		if all != h {
			t.Errorf("%s: groups do not cover the hand", tc.spec)
		}
		for _, w := range tc.want {
			if !got[w] {
				t.Errorf("%s: missing %s in %v", tc.spec, w, got)
			}
		}
	}
	// Narrow and wide decompositions always partition the hand.
	rng := rand.New(rand.NewPCG(5, 5))
	for i := 0; i < 300; i++ {
		g := game.NewGame(rng, 0, game.Config{})
		h := game.CountRanks(append(g.Hands[0], g.Bottom...))
		for _, wide := range []bool{false, true} {
			var dc decomp
			dz.decompose(&h, wide, fullLimit, &dc)
			var all counts
			for _, gr := range dc.groups() {
				gr.addTo(&all)
				if _, ok := game.ClassifyLead(&[]game.Move{gr.move()}[0].Use); !ok {
					t.Fatalf("group %v is not a valid combo", gr)
				}
			}
			if all != h {
				t.Fatalf("decomposition does not partition %s", h.String())
			}
		}
	}
}
