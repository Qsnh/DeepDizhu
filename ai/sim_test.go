package ai

import (
	"fmt"
	"math/rand/v2"
	"runtime"
	"sync"
	"testing"
	"time"

	"deepdizhu/game"
)

// fastHard is a Hard player cheap enough for thousands of games.
func fastHard(seed uint64) Player {
	return NewHard(seed, HardOptions{Budget: 20 * time.Millisecond, MaxRollouts: 120, Candidates: 6, Workers: 1, BidRollouts: 40})
}

// TestSimulationMixedLevels plays thousands of complete deals (bidding
// included) between randomly seated AI levels, rotating the first bidder,
// and checks that every decision is legal, every deal terminates and the
// scores sum to zero.
func TestSimulationMixedLevels(t *testing.T) {
	n := 3000
	if testing.Short() {
		n = 300
	}
	type stat struct{ seatGames, wins, landlord, landlordWins, score int }
	var mu sync.Mutex
	stats := map[Level]*stat{Easy: {}, Normal: {}, Hard: {}}
	redeals := 0
	var firstErr error
	jobs := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < runtime.GOMAXPROCS(0); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				rng := rand.New(rand.NewPCG(2024, uint64(i)))
				var levels [4]Level
				var ps [4]Player
				for s := range ps {
					levels[s] = Levels[rng.IntN(2)] // Easy or Normal
					if rng.IntN(6) == 0 {
						levels[s] = Hard
					}
					if levels[s] == Hard {
						ps[s] = fastHard(uint64(i*4 + s))
					} else {
						ps[s] = New(levels[s], uint64(i*4+s))
					}
				}
				g := game.NewGame(rng, i%4, game.Config{BaseScore: 1 + i%2})
				err := bidAll(g, ps)
				if err == nil && g.Phase == game.PhasePlaying {
					err = playAll(g, ps)
				}
				mu.Lock()
				if err != nil && firstErr == nil {
					firstErr = fmt.Errorf("deal %d (levels %v): %w", i, levels, err)
				}
				if err == nil && g.Phase == game.PhaseRedeal {
					redeals++
				}
				if err == nil && g.Phase == game.PhaseFinished {
					for s, l := range levels {
						st := stats[l]
						st.seatGames++
						won := g.Result.Scores[s] > 0
						if won {
							st.wins++
						}
						if s == g.Landlord {
							st.landlord++
							if won {
								st.landlordWins++
							}
						}
						st.score += g.Result.Scores[s]
					}
				}
				mu.Unlock()
			}
		}()
	}
	for i := 0; i < n; i++ {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	if firstErr != nil {
		t.Fatal(firstErr)
	}
	t.Logf("%d deals, %d void (everybody passed)", n, redeals)
	for _, l := range Levels {
		st := stats[l]
		if st.seatGames == 0 {
			continue
		}
		t.Logf("%s: %5d seat-games, win %.1f%%, landlord %d times (won %.1f%%), avg score %+.2f",
			l, st.seatGames, 100*float64(st.wins)/float64(st.seatGames), st.landlord,
			100*float64(st.landlordWins)/float64(max(1, st.landlord)), float64(st.score)/float64(st.seatGames))
	}
}

func logDuel(t *testing.T, a, b Level, r duelResult, el time.Duration) {
	mean, se := r.scoreSE()
	t.Logf("%s vs %s (whole team at one level, every deal played both ways): %s wins %.1f%% of %d games "+
		"(as landlord %d/%d, as farmers %d/%d); score per deal %+.1f ± %.1f; %v",
		a, b, a, 100*r.aWinRate(), 2*r.deals, r.aWinsAsLandlord, r.deals, r.aWinsAsFarmers, r.deals, mean, se, el.Round(time.Millisecond))
}

func TestStrengthNormalBeatsEasy(t *testing.T) {
	deals := 400
	if testing.Short() {
		deals = 100
	}
	start := time.Now()
	r := duel(t, Normal, Easy, deals, 5, runtime.GOMAXPROCS(0))
	logDuel(t, Normal, Easy, r, time.Since(start))
	if r.aWinRate() < 0.75 {
		t.Errorf("Normal should beat Easy clearly, won %.1f%%", 100*r.aWinRate())
	}
}

// strengthHardOptions is the reduced Hard used by the strength tests: the
// rollout cap (not the clock) binds, so results are reproducible.
var strengthHardOptions = HardOptions{Budget: 2 * time.Second, MaxRollouts: 1500, Candidates: 8, Workers: 1, BidRollouts: -1}

func TestStrengthHardBeatsNormal(t *testing.T) {
	if testing.Short() {
		t.Skip("heavy: plays hundreds of Monte Carlo games")
	}
	saved := testHardOpts
	testHardOpts = strengthHardOptions
	defer func() { testHardOpts = saved }()
	start := time.Now()
	r := duel(t, Hard, Normal, 300, 11, runtime.GOMAXPROCS(0))
	logDuel(t, Hard, Normal, r, time.Since(start))
	if r.aWinRate() < 0.53 {
		t.Errorf("Hard should beat Normal, won %.1f%%", 100*r.aWinRate())
	}
}

func TestStrengthHardBeatsEasy(t *testing.T) {
	if testing.Short() {
		t.Skip("heavy: plays Monte Carlo games")
	}
	saved := testHardOpts
	testHardOpts = strengthHardOptions
	defer func() { testHardOpts = saved }()
	start := time.Now()
	r := duel(t, Hard, Easy, 100, 13, runtime.GOMAXPROCS(0))
	logDuel(t, Hard, Easy, r, time.Since(start))
	if r.aWinRate() < 0.8 {
		t.Errorf("Hard should crush Easy, won %.1f%%", 100*r.aWinRate())
	}
}

// TestHardTiming measures decision times of the default Hard player.
func TestHardTiming(t *testing.T) {
	if testing.Short() {
		t.Skip("measures real thinking time")
	}
	h := NewHard(5, HardOptions{})
	rng := rand.New(rand.NewPCG(6, 6))
	var total, worst time.Duration
	searched, rollouts, n := 0, 0, 12
	for i := 0; i < n; i++ {
		g := randomPosition(rng)
		start := time.Now()
		h.Play(g.View(g.Turn))
		el := time.Since(start)
		total += el
		worst = max(worst, el)
		if st := h.LastStats(); st.Rollouts > 0 {
			searched++
			rollouts += st.Rollouts
		}
	}
	g := game.NewGame(rng, 0, game.Config{})
	start := time.Now()
	h.Bid(g.View(0))
	bid := time.Since(start)
	t.Logf("default Hard: %d positions (%d searched), avg %v, max %v per decision, %d rollouts per searched decision; bid %v",
		n, searched, (total / time.Duration(n)).Round(time.Millisecond), worst.Round(time.Millisecond), rollouts/max(1, searched), bid.Round(time.Millisecond))
	if worst > 800*time.Millisecond {
		t.Errorf("a decision took %v", worst)
	}
}
