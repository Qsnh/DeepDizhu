// Package ai implements computer players for DeepDizhu at three levels:
// Easy (random with a bias to small cards), Normal (hand decomposition
// heuristics) and Hard (Normal plus card counting and determinized Monte
// Carlo search). Players only look at a game.View.
package ai

import (
	"math/rand/v2"
	"slices"
	"sync"

	"deepdizhu/game"
)

// Level is an AI difficulty.
type Level int

// Levels.
const (
	Easy Level = iota
	Normal
	Hard
)

// String returns the Chinese name of the level.
func (l Level) String() string {
	switch l {
	case Easy:
		return "简单"
	case Normal:
		return "普通"
	case Hard:
		return "困难"
	}
	return "未知"
}

// Levels lists all levels from easiest to hardest.
var Levels = []Level{Easy, Normal, Hard}

// Player is a computer player. Implementations are safe to call from a
// background goroutine and always return legal decisions.
type Player interface {
	Bid(v game.View) int          // 0 (不叫) or a legal bid
	Play(v game.View) []game.Card // empty slice = pass
}

// New returns a player of the given level; seed makes it reproducible.
func New(level Level, seed uint64) Player {
	switch level {
	case Easy:
		return newEasy(seed)
	case Hard:
		return NewHard(seed, HardOptions{})
	}
	return newNormal(seed)
}

func newRand(seed uint64) *rand.Rand {
	return rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
}

// Hint suggests a play for the viewer using the Normal strategy. It returns
// nil to suggest passing, and is never nil when the viewer leads.
func Hint(v game.View) []game.Card {
	p := newPolicy(true)
	return safePlay(&v, func() []game.Card { return normalChoice(&v, p) })
}

// ctxFromView builds the policy context for the viewer.
func ctxFromView(v *game.View) ctx {
	cx := ctx{me: v.Seat, landlord: v.Landlord, hand: game.CountRanks(v.Hand), n: v.HandCounts, lastSeat: -1}
	cx.outside = game.CountRanks(v.Unseen())
	if v.LastCombo != nil && v.LastSeat >= 0 && v.LastSeat != v.Seat {
		cx.lastSeat = v.LastSeat
		cx.last = v.LastCombo.Move()
	}
	return cx
}

// normalChoice runs the Normal strategy for the viewer.
func normalChoice(v *game.View, p *policy) []game.Card {
	cx := ctxFromView(v)
	var dc decomp
	p.dz.decompose(&cx.hand, true, fullLimit, &dc)
	m, ok := p.decide(&cx, &dc)
	if !ok {
		return nil
	}
	return m.PickCards(v.Hand)
}

// validPlay reports whether cards is a legal decision for the viewer now.
func validPlay(v *game.View, cards []game.Card) bool {
	if len(cards) == 0 {
		return v.LastCombo != nil
	}
	in := map[game.Card]bool{}
	for _, c := range v.Hand {
		in[c] = true
	}
	for _, c := range cards {
		if !in[c] {
			return false
		}
		delete(in, c)
	}
	if v.LastCombo == nil {
		_, ok := game.ParseLead(cards)
		return ok
	}
	_, ok := game.ParseFollow(cards, *v.LastCombo)
	return ok
}

// fallbackPlay is the safe default: pass when following, otherwise lead the
// smallest card.
func fallbackPlay(v *game.View) []game.Card {
	if v.LastCombo != nil || len(v.Hand) == 0 {
		return nil
	}
	h := slices.Clone(v.Hand)
	game.SortHand(h)
	return h[len(h)-1:]
}

// safePlay runs choose, recovering from panics and replacing illegal
// results by the fallback.
func safePlay(v *game.View, choose func() []game.Card) (out []game.Card) {
	if v.Phase != game.PhasePlaying || v.Turn != v.Seat {
		return nil
	}
	defer func() {
		if r := recover(); r != nil {
			if debugChecks {
				panic(r)
			}
			out = fallbackPlay(v)
		}
	}()
	cards := choose()
	if validPlay(v, cards) {
		if len(cards) == 0 {
			return nil
		}
		return cards
	}
	if debugChecks {
		panic("ai: illegal play " + game.CardsString(cards))
	}
	return fallbackPlay(v)
}

// safeBid clamps a bid to a legal value.
func safeBid(v *game.View, choose func() int) (b int) {
	if v.Phase != game.PhaseBidding || v.Turn != v.Seat {
		return 0
	}
	defer func() {
		if r := recover(); r != nil {
			if debugChecks {
				panic(r)
			}
			b = 0
		}
	}()
	b = choose()
	if b < 0 || b > game.MaxBid || (b != 0 && b <= v.HighestBid) {
		return 0
	}
	return b
}

// normalPlayer is the Normal level.
type normalPlayer struct {
	mu  sync.Mutex
	pol *policy
}

func newNormal(seed uint64) *normalPlayer {
	_ = seed // Normal is deterministic
	return &normalPlayer{pol: newPolicy(true)}
}

func (n *normalPlayer) Bid(v game.View) int {
	return safeBid(&v, func() int {
		h := game.CountRanks(v.Hand)
		return bidFromStrength(handStrength(&h), v.HighestBid)
	})
}

func (n *normalPlayer) Play(v game.View) []game.Card {
	n.mu.Lock()
	defer n.mu.Unlock()
	return safePlay(&v, func() []game.Card { return normalChoice(&v, n.pol) })
}

// easyPlayer is the Easy level.
type easyPlayer struct {
	mu  sync.Mutex
	rng *rand.Rand
}

func newEasy(seed uint64) *easyPlayer { return &easyPlayer{rng: newRand(seed)} }

func (e *easyPlayer) Bid(v game.View) int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return safeBid(&v, func() int {
		h := game.CountRanks(v.Hand)
		s := 2*int(h[rBJ]) + 2*int(h[rSJ]) + int(h[r2])
		for r := rLo; r <= r2; r++ {
			if h[r] >= 4 {
				s += 2
			}
		}
		p := 0.1
		if s >= 6 {
			p = 0.7
		}
		if e.rng.Float64() >= p || v.HighestBid >= game.MaxBid {
			return 0
		}
		return v.HighestBid + 1 + e.rng.IntN(game.MaxBid-v.HighestBid)
	})
}

func (e *easyPlayer) Play(v game.View) []game.Card {
	e.mu.Lock()
	defer e.mu.Unlock()
	return safePlay(&v, func() []game.Card {
		moves := v.LegalMoves()
		if v.LastCombo != nil && (len(moves) == 0 || e.rng.Float64() < 0.3) {
			return nil
		}
		if len(moves) == 0 {
			return nil
		}
		// Moves are sorted weakest first; squaring the uniform draw biases
		// the choice to small cards.
		u := e.rng.Float64()
		return moves[int(u*u*float64(len(moves)))].Cards
	})
}
