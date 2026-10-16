package ai

import (
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"sync"
	"testing"
	"time"

	"deepdizhu/game"
)

func init() { debugChecks = true }

// bidAll runs the bidding phase with the given players.
func bidAll(g *game.Game, players [4]Player) error {
	for g.Phase == game.PhaseBidding {
		s := g.Turn
		b := players[s].Bid(g.View(s))
		if err := g.Bid(s, b); err != nil {
			return fmt.Errorf("seat %d bid %d: %w", s, b, err)
		}
	}
	return nil
}

// playAll plays the deal out with the given players.
func playAll(g *game.Game, players [4]Player) error {
	for steps := 0; g.Phase == game.PhasePlaying; steps++ {
		if steps > 1500 {
			return errors.New("game does not terminate")
		}
		s := g.Turn
		v := g.View(s)
		cards := players[s].Play(v)
		if err := g.Play(s, cards); err != nil {
			return fmt.Errorf("seat %d (%T) played %s on %v: %w", s, players[s], game.CardsString(cards), v.LastCombo, err)
		}
	}
	if g.Phase != game.PhaseFinished || g.Result == nil {
		return errors.New("game not finished")
	}
	sum := 0
	for _, x := range g.Result.Scores {
		sum += x
	}
	if sum != 0 {
		return fmt.Errorf("scores %v do not sum to zero", g.Result.Scores)
	}
	return nil
}

var testHardOpts = HardOptions{Budget: 40 * time.Millisecond, MaxRollouts: 600, Candidates: 6, Workers: 1, BidRollouts: -1}

func testHardOptions() HardOptions { return testHardOpts }

func makePlayer(l Level, seed uint64) Player {
	if l == Hard {
		return NewHard(seed, testHardOptions())
	}
	return New(l, seed)
}

// duelResult aggregates duplicate matches of level a against level b.
type duelResult struct {
	deals             int
	aWinsAsLandlord   int
	aWinsAsFarmers    int
	aScore, bScore    int
	landlordWinsTotal int
	diffs             []int // per-deal score advantage of a
	mults             []int // per-deal multipliers of both games
}

// scoreSE returns the mean per-deal score advantage of a and its standard
// error.
func (d duelResult) scoreSE() (float64, float64) {
	n := float64(len(d.diffs))
	var s, s2 float64
	for _, x := range d.diffs {
		s += float64(x)
		s2 += float64(x) * float64(x)
	}
	m := s / n
	return m, math.Sqrt((s2/n - m*m) / n)
}

func (d duelResult) aWinRate() float64 {
	return float64(d.aWinsAsLandlord+d.aWinsAsFarmers) / float64(2*d.deals)
}

// duel plays each deal twice: once with a as landlord against b farmers and
// once the other way round. Bidding is done by Normal players so both
// games start from the same position.
func duel(t testing.TB, a, b Level, deals int, seed uint64, parallel int) duelResult {
	jobs := make(chan int)
	var mu sync.Mutex
	var res duelResult
	var wg sync.WaitGroup
	var firstErr error
	for w := 0; w < parallel; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				rng := rand.New(rand.NewPCG(seed, uint64(i)))
				var g *game.Game
				for {
					g = game.NewGame(rng, i%4, game.Config{})
					bidders := [4]Player{}
					for s := range bidders {
						bidders[s] = New(Normal, 0)
					}
					if err := bidAll(g, bidders); err != nil {
						panic(err)
					}
					if g.Phase == game.PhasePlaying {
						break
					}
				}
				l := g.Landlord
				var out [2]*game.Result
				for k := 0; k < 2; k++ {
					gg := g.Clone()
					var ps [4]Player
					for s := 0; s < 4; s++ {
						lv := b
						if (s == l) == (k == 0) {
							lv = a
						}
						ps[s] = makePlayer(lv, uint64(i*8+s+k*4)+seed)
					}
					if err := playAll(gg, ps); err != nil {
						mu.Lock()
						if firstErr == nil {
							firstErr = err
						}
						mu.Unlock()
						return
					}
					out[k] = gg.Result
				}
				mu.Lock()
				res.deals++
				if out[0].LandlordWon {
					res.aWinsAsLandlord++
					res.landlordWinsTotal++
				}
				if !out[1].LandlordWon {
					res.aWinsAsFarmers++
				} else {
					res.landlordWinsTotal++
				}
				res.aScore += out[0].Scores[l] - out[1].Scores[l]
				res.diffs = append(res.diffs, out[0].Scores[l]-out[1].Scores[l])
				res.mults = append(res.mults, out[0].Multiplier, out[1].Multiplier)
				mu.Unlock()
			}
		}()
	}
	for i := 0; i < deals; i++ {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	if firstErr != nil {
		t.Fatal(firstErr)
	}
	return res
}

func BenchmarkRollout(b *testing.B) {
	rng := rand.New(rand.NewPCG(1, 2))
	p := newPolicy(false)
	var games []sim
	for i := 0; i < 64; i++ {
		g := game.NewGame(rng, 0, game.Config{})
		var hands [4]counts
		for s := 0; s < 4; s++ {
			hands[s] = game.CountRanks(g.Hands[s])
		}
		bottom := game.CountRanks(g.Bottom)
		hands[0].Add(&bottom)
		s := sim{landlord: 0, turn: 0, lastSeat: -1}
		s.setHands(&hands)
		games = append(games, s)
	}
	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		s := games[i%len(games)]
		s.run(p)
	}
}
