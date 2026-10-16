package ai

import (
	"math"
	"math/rand/v2"
	"os"
	"runtime"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"deepdizhu/game"
)

type simDeal struct {
	hands    [4]counts
	landlord int
}

var (
	dealsOnce sync.Once
	dealCache []simDeal
)

// tuneDeals returns random deals with the landlord chosen as the seat with
// the strongest hand (a stand-in for bidding).
func tuneDeals(n int) []simDeal {
	dealsOnce.Do(func() {
		out := make([]simDeal, 20000)
		var wg sync.WaitGroup
		for w := 0; w < 12; w++ {
			wg.Add(1)
			go func(w int) {
				defer wg.Done()
				for i := w; i < len(out); i += 12 {
					g := game.NewGame(rand.New(rand.NewPCG(123, uint64(i))), 0, game.Config{})
					var d simDeal
					best := -1 << 30
					for s := 0; s < 4; s++ {
						d.hands[s] = game.CountRanks(g.Hands[s])
						if st := handStrength(&d.hands[s]); st > best {
							best, d.landlord = st, s
						}
					}
					b := game.CountRanks(g.Bottom)
					d.hands[d.landlord].Add(&b)
					out[i] = d
				}
			}(w)
		}
		wg.Wait()
		dealCache = out
	})
	return dealCache[:n]
}

// simDuel returns how often policy maker a's team wins against b over n
// deals, each played twice with roles swapped, plus the landlord win rate.
func simDuel(mkA, mkB func() *policy, n int) (float64, float64) {
	a, l, _ := simDuelScore(mkA, mkB, n)
	return a, l
}

// simDuelScore also returns a's mean team score per game in bid units
// (+-3 x 2^bombs, doubled for spring / anti-spring).
func simDuelScore(mkA, mkB func() *policy, n int) (float64, float64, float64) {
	deals := tuneDeals(n)
	var mu sync.Mutex
	aw, lw := 0, 0
	score, score2 := 0.0, 0.0
	var wg sync.WaitGroup
	for w := 0; w < 12; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			pa, pb := mkA(), mkB()
			la, ll := 0, 0
			sc, sc2 := 0.0, 0.0
			for i := w; i < len(deals); i += 12 {
				d := &deals[i]
				for k := 0; k < 2; k++ {
					var ps [4]*policy
					for s := 0; s < 4; s++ {
						if (s == d.landlord) == (k == 0) {
							ps[s] = pa
						} else {
							ps[s] = pb
						}
					}
					s := sim{landlord: d.landlord, turn: d.landlord, lastSeat: -1}
					s.setHands(&d.hands)
					win := s.runSeats(&ps)
					lwin := win == d.landlord
					if lwin {
						ll++
					}
					mult := float64(int(1) << min(s.bombs, 30))
					if lwin && s.plays[(d.landlord+1)&3]+s.plays[(d.landlord+2)&3]+s.plays[(d.landlord+3)&3] == 0 {
						mult *= 2
					}
					if !lwin && s.plays[d.landlord] == 1 {
						mult *= 2
					}
					if lwin == (k == 0) {
						la++
						sc += 3 * mult
					} else {
						sc -= 3 * mult
					}
					sc2 += 9 * mult * mult
				}
			}
			mu.Lock()
			aw += la
			lw += ll
			score += sc
			score2 += sc2
			mu.Unlock()
		}(w)
	}
	wg.Wait()
	m := score / float64(2*n)
	scoreSE = math.Sqrt((score2/float64(2*n) - m*m) / float64(2*n))
	return float64(aw) / float64(2*n), float64(lw) / float64(2*n), m
}

// scoreSE is the standard error of the last simDuelScore score (approximate:
// games treated as independent).
var scoreSE float64

func withParams(f func(*params)) func() *policy {
	return func() *policy {
		p := newPolicy(false)
		f(&p.prm)
		return p
	}
}

// The tests below are development tools, skipped unless EXPLORE=1:
//
//	EXPLORE=1 go test ./ai -run TestTune -v
//
// They recalibrate the bidding thresholds and compare policy variants.

func explore(t *testing.T) {
	if os.Getenv("EXPLORE") == "" {
		t.Skip("set EXPLORE=1 to run tuning tools")
	}
}

// TestTuneParams compares policy parameter variants against the defaults in
// fast simulated duplicate matches.
func TestTuneParams(t *testing.T) {
	explore(t)
	base := withParams(func(*params) {})
	n := 20000
	_, ll := simDuel(base, base, n)
	t.Logf("baseline landlord win rate %.3f", ll)
	variants := map[string]func(*params){
		"leadSize8":    func(p *params) { p.leadSize = 8 },
		"farmerBomb8":  func(p *params) { p.farmerBombOpp = 8 },
		"noCtrl":       func(p *params) { p.ctrlMargin = -100; p.farmerCtrl = 0 },
		"farmerBreak5": func(p *params) { p.farmerBreak = -5; p.farmerLastBreak = -5 },
	}
	for name, f := range variants {
		a, ll, sc := simDuelScore(withParams(f), base, n)
		t.Logf("%-14s vs base: %.3f (landlord rate %.3f) score %+.2f ± %.2f", name, a, ll, sc, scoreSE)
	}
}

// TestTuneBidPercentiles prints the hand-strength and simulated landlord
// win-rate percentiles used for bidThresholds and bidWinRates.
func TestTuneBidPercentiles(t *testing.T) {
	explore(t)
	h := NewHard(1, HardOptions{BidRollouts: 400, Workers: 1})
	var ps []float64
	var sts []int
	for i := 0; i < 400; i++ {
		g := game.NewGame(rand.New(rand.NewPCG(31, uint64(i))), 0, game.Config{})
		v := g.View(0)
		ps = append(ps, h.landlordWinRate(&v))
		c := game.CountRanks(g.Hands[0])
		sts = append(sts, handStrength(&c))
	}
	slices.Sort(ps)
	slices.Sort(sts)
	q := func(p float64) int { return int(p * float64(len(ps))) }
	t.Logf("win rate percentiles 40/67/85: %.2f %.2f %.2f", ps[q(.4)], ps[q(.67)], ps[q(.85)])
	t.Logf("strength percentiles 40/67/85: %d %d %d", sts[q(.4)], sts[q(.67)], sts[q(.85)])
}

// TestTuneDeviations shows where the Hard search overrides the Normal
// heuristics most often: hints for improving the heuristics.
func TestTuneDeviations(t *testing.T) {
	explore(t)
	saved := testHardOpts
	defer func() { testHardOpts = saved; deviationHook = nil }()
	testHardOpts.MaxRollouts = 2400
	var mu sync.Mutex
	stats := map[string]int{}
	deviationHook = func(cx *ctx, n, c cand) {
		role := "farmer"
		if cx.me == cx.landlord {
			role = "landlord"
		}
		phase := "follow"
		if cx.lastSeat < 0 {
			phase = "lead"
		} else if cx.mate(cx.lastSeat) {
			phase = "follow-mate"
		}
		var kind string
		switch {
		case !n.play:
			kind = "play instead of pass: " + c.m.Type.String()
		case !c.play:
			kind = "pass instead of " + n.m.Type.String()
		case n.m.Type != c.m.Type:
			kind = n.m.Type.String() + " -> " + c.m.Type.String()
		case c.m.Rank > n.m.Rank:
			kind = "higher " + c.m.Type.String()
		case c.m.Rank < n.m.Rank:
			kind = "lower " + c.m.Type.String()
		default:
			kind = "other kickers"
		}
		mu.Lock()
		stats[role+" "+phase+": "+kind]++
		mu.Unlock()
	}
	duel(t, Hard, Normal, 200, 99, runtime.GOMAXPROCS(0))
	type kv struct {
		k string
		v int
	}
	var list []kv
	for k, v := range stats {
		list = append(list, kv{k, v})
	}
	slices.SortFunc(list, func(a, b kv) int { return b.v - a.v })
	for _, e := range list[:min(25, len(list))] {
		t.Logf("%5d %s", e.v, e.k)
	}
}

// TestTuneTrace prints one Normal-vs-Normal game (SEED selects the deal).
func TestTuneTrace(t *testing.T) {
	explore(t)
	seed, _ := strconv.Atoi(os.Getenv("SEED"))
	g := game.NewGame(rand.New(rand.NewPCG(uint64(seed), 1)), 0, game.Config{})
	ps := [4]Player{New(Normal, 1), New(Normal, 2), New(Normal, 3), New(Normal, 4)}
	if err := bidAll(g, ps); err != nil || g.Phase != game.PhasePlaying {
		t.Skip("redeal")
	}
	for s := 0; s < 4; s++ {
		h := game.CountRanks(g.Hands[s])
		t.Logf("seat %d landlord=%v bid %d: %s", s, s == g.Landlord, g.Bids[s], h.String())
	}
	for g.Phase == game.PhasePlaying {
		s := g.Turn
		lead := g.Leading()
		cards := ps[s].Play(g.View(s))
		if err := g.Play(s, cards); err != nil {
			t.Fatal(err)
		}
		desc := "过"
		if len(cards) > 0 {
			desc = g.History[len(g.History)-1].Combo.Describe() + " [" + ranks(cards) + "]"
		}
		mark := "  "
		if lead {
			mark = "> "
		}
		t.Logf("%sseat %d (%2d left): %s", mark, s, len(g.Hands[s]), desc)
	}
	t.Logf("result %+v", *g.Result)
}

// TestTuneRollouts compares rollout policy decomposition widths.
func TestTuneRollouts(t *testing.T) {
	explore(t)
	saved := testHardOpts
	defer func() { testHardOpts = saved; rolloutWide, rolloutLimit = false, fastLimit }()
	testHardOpts = strengthHardOptions
	for _, c := range []struct {
		wide  bool
		limit int
	}{{false, fastLimit}, {true, 200}, {true, 1000}} {
		rolloutWide, rolloutLimit = c.wide, c.limit
		start := time.Now()
		r := duel(t, Hard, Normal, 400, 11, runtime.GOMAXPROCS(0))
		t.Logf("rollout wide=%v limit=%d: %.1f%% in %v", c.wide, c.limit, 100*r.aWinRate(), time.Since(start))
	}
}
