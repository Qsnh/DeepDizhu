package ai

import (
	"math"
	"math/rand/v2"
	"runtime"
	"sync"
	"time"

	"deepdizhu/game"
)

// HardOptions tunes the Hard player. Zero fields take the defaults.
type HardOptions struct {
	// Budget is the thinking time per playing decision (default 500ms).
	Budget time.Duration
	// MaxRollouts caps the rollouts per playing decision (default 60000).
	MaxRollouts int
	// Candidates is the number of candidate moves searched (default 10).
	Candidates int
	// BidRollouts is the number of simulated deals used to judge a bid
	// (default 400; negative uses the Normal hand evaluation only).
	BidRollouts int
	// BidBudget caps the bidding time (default 300ms).
	BidBudget time.Duration
	// Workers is the number of goroutines used by the search (default:
	// min(4, GOMAXPROCS)).
	Workers int
}

func (o HardOptions) withDefaults() HardOptions {
	if o.Budget <= 0 {
		o.Budget = 500 * time.Millisecond
	}
	if o.MaxRollouts <= 0 {
		o.MaxRollouts = 60000
	}
	if o.Candidates <= 0 {
		o.Candidates = 10
	}
	if o.BidRollouts == 0 {
		o.BidRollouts = 400
	}
	if o.BidBudget <= 0 {
		o.BidBudget = 300 * time.Millisecond
	}
	if o.Workers <= 0 {
		o.Workers = min(4, runtime.GOMAXPROCS(0))
	}
	return o
}

// HardStats describes the last playing decision of a Hard player.
type HardStats struct {
	Candidates int
	Rollouts   int
	Elapsed    time.Duration
}

// HardPlayer is the Hard level: the Normal heuristics generate candidate
// moves, and a determinized Monte Carlo search picks among them: it samples
// deals of the unseen cards consistent with the hand sizes (unplayed bottom
// cards stay with the landlord), plays every candidate out with fast
// heuristic policies for all seats and keeps the move whose team does best.
type HardPlayer struct {
	mu      sync.Mutex
	opt     HardOptions
	rng     *rand.Rand
	pol     *policy
	last    HardStats
	workers []*searchWorker
}

// NewHard returns a Hard player with the given options.
func NewHard(seed uint64, opt HardOptions) *HardPlayer {
	return &HardPlayer{opt: opt.withDefaults(), rng: newRand(seed), pol: newPolicy(true)}
}

// LastStats returns statistics about the last playing decision.
func (h *HardPlayer) LastStats() HardStats {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.last
}

// Bid judges the hand by simulating deals with this hand as landlord.
func (h *HardPlayer) Bid(v game.View) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return safeBid(&v, func() int {
		hand := game.CountRanks(v.Hand)
		if h.opt.BidRollouts < 0 {
			return bidFromStrength(handStrength(&hand), v.HighestBid)
		}
		return bidFromWinRate(h.landlordWinRate(&v), v.HighestBid)
	})
}

// landlordWinRate estimates how often the viewer would win as landlord.
func (h *HardPlayer) landlordWinRate(v *game.View) float64 {
	me := v.Seat
	hand := game.CountRanks(v.Hand)
	pool := rankPool(game.CountRanks(v.Unseen()), nil)
	need := [4]int{}
	for s := 0; s < 4; s++ {
		need[s] = v.HandCounts[s]
	}
	need[me] = 0
	extra := game.BottomSize
	if len(pool) < extra+need[(me+1)&3]+need[(me+2)&3]+need[(me+3)&3] {
		return 0
	}
	deadline := time.Now().Add(h.opt.BidBudget)
	roll := newPolicy(false)
	var s sim
	wins, n := 0, 0
	for n < h.opt.BidRollouts && (n < 16 || time.Now().Before(deadline)) {
		shuffleRanks(h.rng, pool)
		var hands [4]counts
		hands[me] = hand
		i := 0
		for ; i < extra; i++ {
			hands[me][pool[i]]++
		}
		for d := 1; d < 4; d++ {
			seat := (me + d) & 3
			for k := 0; k < need[seat]; k++ {
				hands[seat][pool[i]]++
				i++
			}
		}
		s = sim{landlord: me, turn: me, lastSeat: -1}
		s.setHands(&hands)
		if s.run(roll) == me {
			wins++
		}
		n++
	}
	return float64(wins) / float64(n)
}

// Play picks a move with Monte Carlo search.
func (h *HardPlayer) Play(v game.View) []game.Card {
	h.mu.Lock()
	defer h.mu.Unlock()
	return safePlay(&v, func() []game.Card { return h.choose(&v) })
}

// deviationHook, when set (tests), is told whenever the search overrides
// the Normal choice.
var deviationHook func(cx *ctx, normal, chosen cand)

type cand struct {
	m    game.Move
	play bool
}

func (h *HardPlayer) choose(v *game.View) []game.Card {
	start := time.Now()
	cx := ctxFromView(v)
	var dc decomp
	h.pol.dz.decompose(&cx.hand, true, fullLimit, &dc)
	nm, nok := h.pol.decide(&cx, &dc)
	h.last = HardStats{Candidates: 1}
	if nok && nm.Size() == len(v.Hand) {
		return nm.PickCards(v.Hand) // goes out
	}
	cands := h.candidates(&cx, &dc, cand{nm, nok})
	h.last.Candidates = len(cands)
	best := 0
	if len(cands) > 1 {
		var rollouts int
		best, rollouts = h.search(v, &cx, cands, start)
		h.last.Rollouts = rollouts
	}
	h.last.Elapsed = time.Since(start)
	if deviationHook != nil && best != 0 {
		deviationHook(&cx, cands[0], cands[best])
	}
	if !cands[best].play {
		return nil
	}
	return cands[best].m.PickCards(v.Hand)
}

// candidates lists the moves worth searching: the Normal choice first, then
// passing, the decomposition-based leads or answers, and a few other legal
// moves of different shapes, deduplicated and capped.
func (h *HardPlayer) candidates(cx *ctx, dc *decomp, normal cand) []cand {
	limit := h.opt.Candidates
	out := []cand{normal}
	seen := map[counts]bool{}
	if normal.play {
		seen[normal.m.Use] = true
	}
	add := func(m game.Move) {
		if len(out) >= limit || seen[m.Use] {
			return
		}
		seen[m.Use] = true
		out = append(out, cand{m, true})
	}
	if cx.lastSeat < 0 {
		var buf [maxGroups]option
		opts := h.pol.leadOptions(cx, dc, buf[:0])
		// Weakest structural leads first.
		for len(opts) > 0 && len(out) < limit {
			i := h.pol.pickLead(opts, pickAny)
			add(opts[i].m)
			opts = append(opts[:i], opts[i+1:]...)
		}
		// One move per shape from the full list (weakest of each type), so
		// other ways to split the hand are considered too.
		legal := game.GenLeads(&cx.hand)
		var typeSeen [game.JokerBomb + 1]bool
		for _, m := range legal {
			if !typeSeen[m.Type] {
				typeSeen[m.Type] = true
				add(m)
			}
		}
		return out
	}
	if normal.play {
		out = append(out, cand{}) // pass
	}
	var buf [24]game.Move
	for _, m := range h.pol.matches(cx, dc, buf[:0]) {
		add(m)
	}
	for _, m := range h.pol.answers(cx, buf[:0]) {
		add(m)
	}
	legal := game.GenFollows(&cx.hand, cx.last)
	// The weakest bomb and the joker bomb, then other legal answers.
	var bombs []game.Move
	for _, m := range legal {
		if m.IsBomb() {
			bombs = append(bombs, m)
		}
	}
	if len(bombs) > 0 {
		add(bombs[0])
		if last := bombs[len(bombs)-1]; last.Type == game.JokerBomb {
			add(last)
		}
	}
	for _, m := range legal {
		add(m)
	}
	return out
}

// rankPool lists the ranks of the cards in c minus known.
func rankPool(c counts, known *counts) []game.Rank {
	if known != nil {
		c.Sub(known)
	}
	pool := make([]game.Rank, 0, c.Total())
	for r := rLo; r <= rBJ; r++ {
		for i := 0; i < int(c[r]); i++ {
			pool = append(pool, game.Rank(r))
		}
	}
	return pool
}

func shuffleRanks(rng *rand.Rand, p []game.Rank) {
	for i := len(p) - 1; i > 0; i-- {
		j := rng.IntN(i + 1)
		p[i], p[j] = p[j], p[i]
	}
}

// rolloutWide/rolloutLimit configure the rollout policy's decompositions.
var (
	rolloutWide  = false
	rolloutLimit = fastLimit
)

func newRolloutPolicy() *policy {
	p := newPolicy(rolloutWide)
	p.limit = rolloutLimit
	return p
}

// searchWorker is the per-goroutine scratch state of the search.
type searchWorker struct {
	rng  *rand.Rand
	roll *policy
	pool []game.Rank
	res  []float64
	done int
}

// searchSetup is the shared, read-only description of a search.
type searchSetup struct {
	me, l int
	hand  counts
	known counts
	need  [4]int
	base  sim
	cands []cand
}

// sample deals one consistent world and plays every active candidate out
// in it, storing the outcomes at res[k*len(cands)+ci].
func (w *searchWorker) sample(st *searchSetup, active []bool, k int) {
	shuffleRanks(w.rng, w.pool)
	var hands [4]counts
	hands[st.me] = st.hand
	hands[st.l].Add(&st.known)
	i := 0
	for seat := 0; seat < 4; seat++ {
		for j := 0; j < st.need[seat]; j++ {
			hands[seat][w.pool[i]]++
			i++
		}
	}
	nc := len(st.cands)
	var s sim
	for ci := range st.cands {
		if !active[ci] {
			continue
		}
		s = st.base
		s.setHands(&hands)
		winner := st.me
		if !s.apply(&st.cands[ci].m, st.cands[ci].play) {
			winner = s.run(w.roll)
		}
		w.res[k*nc+ci] = outcome(&s, st.me, winner)
	}
}

// Racing parameters: candidates are evaluated on shared deals in batches;
// after each batch a candidate whose paired difference to the leader is
// significantly negative is dropped, and the search stops early once no
// remaining candidate can be better than the leader by more than raceEps.
const (
	raceBatch      = 4    // samples per worker per batch
	raceMinSamples = 48   // samples before anything is dropped
	raceZ          = 3.0  // confidence multiplier
	raceEps        = 0.01 // indifference margin (outcome units)
)

// search evaluates the candidates and returns the index of the best one and
// the number of rollouts run.
func (h *HardPlayer) search(v *game.View, cx *ctx, cands []cand, start time.Time) (int, int) {
	st := &searchSetup{me: v.Seat, l: v.Landlord, hand: cx.hand, cands: cands}
	if st.me != st.l {
		st.known = game.CountRanks(v.KnownInHand(st.l))
	}
	pool := rankPool(cx.outside, &st.known)
	for s := 0; s < 4; s++ {
		if s != st.me {
			st.need[s] = v.HandCounts[s]
		}
	}
	st.need[st.l] -= st.known.Total()
	if st.need[st.l] < 0 || len(pool) != st.need[0]+st.need[1]+st.need[2]+st.need[3] {
		return 0, 0 // inconsistent view; trust the heuristics
	}
	st.base = sim{landlord: st.l, turn: st.me, lastSeat: cx.lastSeat, last: cx.last, plays: v.PlayCounts}

	nc := len(cands)
	for len(h.workers) < h.opt.Workers {
		h.workers = append(h.workers, &searchWorker{roll: newRolloutPolicy()})
	}
	ws := h.workers[:h.opt.Workers]
	for _, w := range ws {
		w.rng = newRand(h.rng.Uint64())
		w.pool = append(w.pool[:0], pool...)
		if cap(w.res) < raceBatch*nc {
			w.res = make([]float64, raceBatch*nc)
		}
		w.res = w.res[:raceBatch*nc]
	}
	active := make([]bool, nc)
	for i := range active {
		active[i] = true
	}
	nActive := nc
	vals := make([][]float64, nc)
	deadline := start.Add(h.opt.Budget)
	rollouts, n := 0, 0
	var wg sync.WaitGroup
	for nActive > 1 && rollouts < h.opt.MaxRollouts {
		if n >= 2 && time.Now().After(deadline) {
			break
		}
		per := raceBatch
		if left := (h.opt.MaxRollouts - rollouts + nActive - 1) / nActive; left < per*len(ws) {
			per = max(1, (left+len(ws)-1)/len(ws))
		}
		for _, w := range ws {
			wg.Add(1)
			go func(w *searchWorker) {
				defer wg.Done()
				w.done = 0
				for k := 0; k < per; k++ {
					if (n > 0 || k > 0) && time.Now().After(deadline) {
						return
					}
					w.sample(st, active, k)
					w.done = k + 1
				}
			}(w)
		}
		wg.Wait()
		for _, w := range ws {
			for k := 0; k < w.done; k++ {
				for ci := range cands {
					if active[ci] {
						vals[ci] = append(vals[ci], w.res[k*nc+ci])
					}
				}
				n++
				rollouts += nActive
			}
		}
		if n >= raceMinSamples {
			var stop bool
			nActive, stop = race(vals, active, n)
			if stop {
				break
			}
		}
	}
	if n == 0 {
		return 0, 0
	}
	best, bestV := -1, 0.0
	for ci := range cands {
		if !active[ci] {
			continue
		}
		m := mean(vals[ci][:n])
		if ci == 0 {
			m += 0.005 // ties go to the Normal choice
		}
		if best < 0 || m > bestV {
			best, bestV = ci, m
		}
	}
	return best, rollouts
}

func mean(x []float64) float64 {
	t := 0.0
	for _, v := range x {
		t += v
	}
	return t / float64(len(x))
}

// race drops candidates that are significantly worse than the leader and
// reports whether the remaining ones are all within raceEps of it.
func race(vals [][]float64, active []bool, n int) (int, bool) {
	lead, leadV := -1, 0.0
	for ci := range vals {
		if active[ci] {
			if m := mean(vals[ci][:n]); lead < 0 || m > leadV {
				lead, leadV = ci, m
			}
		}
	}
	nActive, close := 1, true
	for ci := range vals {
		if !active[ci] || ci == lead {
			continue
		}
		var sd, sd2 float64
		a, b := vals[ci][:n], vals[lead][:n]
		for k := range a {
			d := a[k] - b[k]
			sd += d
			sd2 += d * d
		}
		md := sd / float64(n)
		se := math.Sqrt(max(0, sd2/float64(n)-md*md) / float64(n))
		if md+raceZ*se < 0 {
			active[ci] = false
			continue
		}
		nActive++
		if md+raceZ*se >= raceEps {
			close = false
		}
	}
	return nActive, close
}

// outcome scores a finished rollout for seat me: +-1 for a win or loss,
// growing with the doublings (bombs, spring) the rollout produced.
func outcome(s *sim, me, winner int) float64 {
	if winner < 0 {
		return 0
	}
	l := s.landlord
	won := winner == me || (me != l && winner != l)
	k := s.bombs
	if winner == l {
		spring := true
		for i := 0; i < 4; i++ {
			if i != l && s.plays[i] > 0 {
				spring = false
			}
		}
		if spring {
			k++
		}
	} else if s.plays[l] == 1 {
		k++
	}
	// Linear in the doublings: scoring rollouts by the real exponential
	// stake made the search much noisier and clearly weaker in tests.
	v := 1 + 0.25*float64(k)
	if won {
		return v
	}
	return -v
}
