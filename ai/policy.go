package ai

import "deepdizhu/game"

// ctx is what a seat may use to decide a play: its own hand and public
// information (hand sizes, the current trick, which cards are still out).
type ctx struct {
	me, landlord int
	hand         counts
	outside      counts // cards held by the other three seats (split unknown)
	n            [4]int // hand sizes
	lastSeat     int    // -1 when leading
	last         game.Move
}

// opp reports whether s is an opponent of the deciding seat.
func (cx *ctx) opp(s int) bool {
	return s != cx.me && (s == cx.landlord) != (cx.me == cx.landlord)
}

// mate reports whether s is a teammate (another farmer).
func (cx *ctx) mate(s int) bool {
	return s != cx.me && s != cx.landlord && cx.me != cx.landlord
}

func (cx *ctx) minOpp() int {
	m := 1 << 10
	for s := 0; s < 4; s++ {
		if cx.opp(s) && cx.n[s] < m {
			m = cx.n[s]
		}
	}
	return m
}

// actsBefore reports whether seat s acts after the deciding seat and before
// the trick returns to the last player.
func (cx *ctx) actsBefore(s int) bool {
	for x := (cx.me + 1) & 3; x != cx.lastSeat; x = (x + 1) & 3 {
		if x == s {
			return true
		}
	}
	return false
}

// params are the tunable knobs of the heuristic policy.
type params struct {
	landlordBreak   int  // min decomposition gain for the landlord to break a structure to answer
	farmerBreak     int  // same for a farmer when a teammate still acts before the landlord
	farmerLastBreak int  // same for the farmer acting right before the landlord
	bossBonus       int  // gain bonus for an answer nobody can beat
	urgentOpp       int  // answer at any cost when an opponent holds <= this many cards
	urgentLast      int  // ... or the player who made the play holds <= this many
	bombOpp         int  // bomb when an opponent holds <= this many cards
	bombLast        int  // ... or the player who made the play holds <= this many
	bombTurns       int  // ... or the rest of the hand needs <= this many turns
	leadSize        int  // how much leading many cards is preferred (option key weight)
	farmerBombOpp   int  // bombOpp/bombLast override for farmers (0 = same)
	ctrlMargin      int  // bomb when the rest keeps >= small groups + ctrlMargin controls (-100 = off)
	farmerCtrl      int  // ctrlMargin override for farmers (0 = same)
	bossBomb        bool // under threat, bomb with the weakest unbeatable bomb
}

var defaultParams = params{
	landlordBreak:   -3,
	farmerBreak:     -3,
	farmerLastBreak: -3,
	bossBonus:       5,
	urgentOpp:       3,
	urgentLast:      4,
	bombOpp:         4,
	bombLast:        5,
	bombTurns:       2,
	leadSize:        15,
	ctrlMargin:      0,
	farmerCtrl:      -2,
	bossBomb:        true,
}

// policy is the heuristic player (the Normal strategy). Hard uses it for
// rollouts with a cheap decomposition. Not safe for concurrent use.
type policy struct {
	dz      decomposer
	scratch decomp
	wide    bool // search width of the decompositions it runs itself
	limit   int
	prm     params
}

func newPolicy(wide bool) *policy {
	p := &policy{wide: wide, limit: fastLimit, prm: defaultParams}
	if wide {
		p.limit = fullLimit / 4
	}
	return p
}

// debugChecks makes the policy panic on internal inconsistencies (tests).
var debugChecks = false

// decide returns the move for the seat described by cx, given the
// decomposition dc of its hand; ok is false for a pass.
func (p *policy) decide(cx *ctx, dc *decomp) (game.Move, bool) {
	if cx.lastSeat < 0 {
		m := p.lead(cx, dc)
		if om, ok := game.ClassifyLead(&m.Use); ok && cx.hand.Contains(&m.Use) {
			return om, true
		}
		if debugChecks {
			panic("policy: invalid lead " + m.Use.String())
		}
		return lowestSingle(&cx.hand), true
	}
	m, ok := p.follow(cx, dc)
	if !ok {
		return game.Move{}, false
	}
	if om, ok := game.ClassifyFollow(&m.Use, cx.last); ok && cx.hand.Contains(&m.Use) {
		return om, true
	}
	if debugChecks {
		panic("policy: invalid follow " + m.Use.String() + " on " + cx.last.Describe())
	}
	return game.Move{}, false
}

// beatable reports whether some non-bomb combo of the same shape could be
// formed from out that beats m (bombs are ignored, except for bombs).
func beatable(m *game.Move, out *counts) bool {
	r := int(m.Rank)
	switch m.Type {
	case game.Single:
		for x := r + 1; x <= rBJ; x++ {
			if out[x] > 0 {
				return true
			}
		}
	case game.Pair:
		for x := r + 1; x <= rBJ; x++ {
			if out[x] >= 2 {
				return true
			}
		}
	case game.Triple, game.TripleSingle, game.TriplePair:
		for x := r + 1; x <= r2; x++ {
			if out[x] >= 3 {
				return true
			}
		}
	case game.FourTwoSingles, game.FourTwoPairs:
		for x := r + 1; x <= r2; x++ {
			if out[x] >= 4 {
				return true
			}
		}
	case game.Straight:
		return chainBeatable(out, r, m.Length, 1)
	case game.PairStraight:
		return chainBeatable(out, r, m.Length, 2)
	case game.Plane, game.PlaneSingles, game.PlanePairs:
		return chainBeatable(out, r, m.Length, 3)
	case game.Bomb:
		for x := rLo; x <= r2; x++ {
			if int(out[x]) > m.Length || (int(out[x]) == m.Length && x > r) {
				return true
			}
		}
		return out[rSJ] >= 2 && out[rBJ] >= 2
	}
	return false
}

func chainBeatable(out *counts, lo, l int, k uint8) bool {
	run := 0
	for x := lo + 1; x <= rA; x++ {
		if out[x] >= k {
			run++
			if run >= l {
				return true
			}
		} else {
			run = 0
		}
	}
	return false
}

// hasBombAgainst reports whether out could hold a bomb.
func outsideBombs(out *counts) bool {
	for x := rLo; x <= r2; x++ {
		if out[x] >= 4 {
			return true
		}
	}
	return out[rSJ] >= 2 && out[rBJ] >= 2
}

func single(r int) game.Move {
	m := game.Move{Type: game.Single, Rank: game.Rank(r), Length: 1}
	m.Use[r] = 1
	return m
}

func pairMove(r int) game.Move {
	m := game.Move{Type: game.Pair, Rank: game.Rank(r), Length: 1}
	m.Use[r] = 2
	return m
}

// bombRank reports whether rank r of hand h belongs to a bomb (a 4+ set or
// a joker of a complete joker bomb).
func bombRank(h *counts, r int) bool {
	if r >= rSJ {
		return h[rSJ] >= 2 && h[rBJ] >= 2
	}
	return h[r] >= 4
}

// lowestSingle returns a single of the lowest rank, avoiding bombs when
// possible.
func lowestSingle(h *counts) game.Move {
	for r := rLo; r <= rBJ; r++ {
		if h[r] > 0 && !bombRank(h, r) {
			return single(r)
		}
	}
	for r := rLo; r <= rBJ; r++ {
		if h[r] > 0 {
			return single(r)
		}
	}
	return game.Move{}
}

// highestAbove returns a set of k cards (single or pair) of the highest
// non-bomb rank above r.
func highestAbove(h *counts, r, k int) (game.Move, bool) {
	for x := rBJ; x > r; x-- {
		if int(h[x]) >= k && !bombRank(h, x) {
			if k == 1 {
				return single(x), true
			}
			return pairMove(x), true
		}
	}
	return game.Move{}, false
}

type option struct {
	m    game.Move
	boss bool
	size int
}

func optKey(o *option, w int) int { return int(o.m.Rank)*10 - w*o.size }

// kickerSingles collects up to need single kicker ranks (< 2) from the
// decomposition: single groups first, then cards of pair groups.
func kickerSingles(gs []group, need int, use *counts) bool {
	got := 0
	for _, g := range gs {
		if got == need {
			break
		}
		if g.t == uint8(game.Single) && int(g.r) < r2 {
			use[g.r]++
			got++
		}
	}
	if got < need {
		for _, g := range gs {
			if got == need {
				break
			}
			if g.t == uint8(game.Pair) && int(g.r) < r2 {
				t := min(2, need-got)
				use[g.r] += uint8(t)
				got += t
			}
		}
	}
	return got == need
}

func kickerPairs(gs []group, need int, use *counts) bool {
	got := 0
	for _, g := range gs {
		if got == need {
			break
		}
		if g.t == uint8(game.Pair) && int(g.r) < r2 {
			use[g.r] += 2
			got++
		}
	}
	return got == need
}

// lowestKickerGroup returns the rank of the lowest single and pair groups
// below 2 (0 if none).
func lowestKickerGroups(gs []group) (s, p int) {
	for _, g := range gs {
		if int(g.r) >= r2 {
			continue
		}
		if g.t == uint8(game.Single) && (s == 0 || int(g.r) < s) {
			s = int(g.r)
		}
		if g.t == uint8(game.Pair) && (p == 0 || int(g.r) < p) {
			p = int(g.r)
		}
	}
	return
}

// withKickers turns a triple or plane group into a lead carrying the
// cheapest kickers available in the decomposition.
func withKickers(g group, gs []group) game.Move {
	m := g.move()
	if g.t == uint8(game.Triple) {
		s, p := lowestKickerGroups(gs)
		switch {
		case s != 0 && (p == 0 || s <= p):
			m.Use[s]++
			m.Type = game.TripleSingle
		case p != 0:
			m.Use[p] += 2
			m.Type = game.TriplePair
		}
		return m
	}
	if g.t == uint8(game.Plane) {
		u := m.Use
		if kickerSingles(gs, int(g.l), &u) {
			m.Use, m.Type = u, game.PlaneSingles
			return m
		}
		u = m.Use
		if kickerPairs(gs, int(g.l), &u) {
			m.Use, m.Type = u, game.PlanePairs
		}
	}
	return m
}

func (p *policy) leadOptions(cx *ctx, dc *decomp, opts []option) []option {
	gs := dc.groups()
	for _, g := range gs {
		if g.isBomb() {
			continue
		}
		var m game.Move
		if g.t == uint8(game.Triple) || g.t == uint8(game.Plane) {
			m = withKickers(g, gs)
			if om, ok := game.ClassifyLead(&m.Use); ok {
				m = om
			} else {
				m = g.move()
			}
		} else {
			m = g.move()
		}
		opts = append(opts, option{m: m, boss: !beatable(&m, &cx.outside), size: m.Size()})
	}
	return opts
}

// Lead filters for pickLead.
const (
	pickAny = iota
	pickPairs
	pickSingles
	pickNotSingles
	pickBig // three or more cards
)

func pickOK(o *option, mode int) bool {
	switch mode {
	case pickPairs:
		return o.m.Type == game.Pair
	case pickSingles:
		return o.m.Type == game.Single
	case pickNotSingles:
		return o.m.Type != game.Single
	case pickBig:
		return o.size >= 3
	}
	return true
}

// pickLead returns the index of the preferred option among those accepted
// by mode (-1 if none): non-boss options first, then the lowest key.
func (p *policy) pickLead(opts []option, mode int) int {
	best := -1
	for i := range opts {
		o := &opts[i]
		if !pickOK(o, mode) {
			continue
		}
		if best < 0 {
			best = i
			continue
		}
		b := &opts[best]
		if o.boss != b.boss {
			if !o.boss {
				best = i
			}
			continue
		}
		if optKey(o, p.prm.leadSize) < optKey(b, p.prm.leadSize) {
			best = i
		}
	}
	return best
}

func smallestBomb(dc *decomp) (game.Move, bool) {
	best := -1
	gs := dc.groups()
	for i, g := range gs {
		if !g.isBomb() {
			continue
		}
		if best < 0 || bombLess(g, gs[best]) {
			best = i
		}
	}
	if best < 0 {
		return game.Move{}, false
	}
	return gs[best].move(), true
}

func bombLess(a, b group) bool {
	ma, mb := a.move(), b.move()
	return mb.Beats(ma)
}

func (p *policy) lead(cx *ctx, dc *decomp) game.Move {
	if m, ok := game.ClassifyLead(&cx.hand); ok {
		return m // the whole hand goes out at once
	}
	var buf [maxGroups]option
	opts := p.leadOptions(cx, dc, buf[:0])
	if len(opts) == 0 {
		if m, ok := smallestBomb(dc); ok {
			return m
		}
		return lowestSingle(&cx.hand)
	}
	nonBoss := 0
	for i := range opts {
		if !opts[i].boss {
			nonBoss++
		}
	}
	// Finish: cash in the unbeatable groups while at most one other remains.
	if nonBoss <= 1 {
		best := -1
		for i := range opts {
			if opts[i].boss && (best < 0 || opts[i].size > opts[best].size) {
				best = i
			}
		}
		if best >= 0 {
			return opts[best].m
		}
	}
	// Feed a teammate who is about to go out.
	next := (cx.me + 1) & 3
	if cx.mate(next) {
		if cx.n[next] == 1 {
			return lowestSingle(&cx.hand)
		}
		if cx.n[next] == 2 {
			if i := p.pickLead(opts, pickPairs); i >= 0 {
				return opts[i].m
			}
		}
	}
	switch cx.minOpp() {
	case 1:
		// Never hand a one-card opponent an easy single.
		if i := p.pickLead(opts, pickNotSingles); i >= 0 {
			return opts[i].m
		}
		best := -1
		for i := range opts {
			if best < 0 || opts[i].m.Rank > opts[best].m.Rank {
				best = i
			}
		}
		return opts[best].m
	case 2:
		if i := p.pickLead(opts, pickBig); i >= 0 {
			return opts[i].m
		}
		if i := p.pickLead(opts, pickSingles); i >= 0 {
			return opts[i].m
		}
		best := -1
		for i := range opts {
			if best < 0 || opts[i].m.Rank > opts[best].m.Rank {
				best = i
			}
		}
		return opts[best].m
	}
	return opts[p.pickLead(opts, pickAny)].m
}

func (p *policy) follow(cx *ctx, dc *decomp) (game.Move, bool) {
	if m, ok := game.ClassifyFollow(&cx.hand, cx.last); ok {
		return m, true // goes out
	}
	if cx.mate(cx.lastSeat) {
		return p.followMate(cx, dc)
	}
	return p.followOpp(cx, dc)
}

// matches lists answers to the current play made of whole groups of the
// decomposition (no structure broken), weakest first.
func (p *policy) matches(cx *ctx, dc *decomp, out []game.Move) []game.Move {
	prev := &cx.last
	gs := dc.groups()
	pr := int(prev.Rank)
	for _, g := range gs {
		if int(g.r) <= pr || g.isBomb() {
			continue
		}
		var m game.Move
		switch prev.Type {
		case game.Single, game.Pair, game.Triple:
			if g.typ() != prev.Type {
				continue
			}
			m = g.move()
		case game.Straight, game.PairStraight, game.Plane:
			if g.typ() != prev.Type || int(g.l) != prev.Length {
				continue
			}
			m = g.move()
		case game.TripleSingle, game.TriplePair:
			if g.t != uint8(game.Triple) {
				continue
			}
			m = g.move()
			s, pp := lowestKickerGroups(gs)
			if prev.Type == game.TripleSingle {
				if s == 0 {
					continue
				}
				m.Use[s]++
			} else {
				if pp == 0 {
					continue
				}
				m.Use[pp] += 2
			}
			m.Type = prev.Type
		case game.PlaneSingles, game.PlanePairs:
			if g.t != uint8(game.Plane) || int(g.l) != prev.Length {
				continue
			}
			m = g.move()
			if prev.Type == game.PlaneSingles {
				if !kickerSingles(gs, prev.Length, &m.Use) {
					continue
				}
			} else if !kickerPairs(gs, prev.Length, &m.Use) {
				continue
			}
			m.Type = prev.Type
		default:
			continue
		}
		if om, ok := game.ClassifyFollow(&m.Use, *prev); ok {
			out = append(out, om)
		}
	}
	// insertion sort by rank (short list)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].Rank < out[j-1].Rank; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// answers lists non-bomb answers to the current play from the whole hand
// (structures may be broken; bombs are not), with cheap kickers, weakest
// first.
func (p *policy) answers(cx *ctx, out []game.Move) []game.Move {
	prev := &cx.last
	avail := cx.hand
	for r := rLo; r <= rBJ; r++ {
		if bombRank(&cx.hand, r) {
			avail[r] = 0
		}
	}
	pr := int(prev.Rank)
	emit := func(m game.Move) {
		if om, ok := game.ClassifyFollow(&m.Use, *prev); ok && !om.IsBomb() {
			out = append(out, om)
		}
	}
	switch prev.Type {
	case game.Single:
		for r := pr + 1; r <= rBJ; r++ {
			if avail[r] >= 1 {
				emit(single(r))
			}
		}
	case game.Pair:
		for r := pr + 1; r <= rBJ; r++ {
			if avail[r] >= 2 {
				emit(pairMove(r))
			}
		}
	case game.Triple, game.TripleSingle, game.TriplePair:
		for r := pr + 1; r <= r2; r++ {
			if avail[r] < 3 {
				continue
			}
			m := game.Move{Type: prev.Type, Rank: game.Rank(r), Length: 1}
			m.Use[r] = 3
			switch prev.Type {
			case game.TripleSingle:
				if !cheapKickers(&avail, r, r, 1, 1, &m.Use) {
					continue
				}
			case game.TriplePair:
				if !cheapKickers(&avail, r, r, 1, 2, &m.Use) {
					continue
				}
			}
			emit(m)
		}
	case game.Straight, game.PairStraight, game.Plane, game.PlaneSingles, game.PlanePairs:
		k := uint8(1)
		switch prev.Type {
		case game.PairStraight:
			k = 2
		case game.Plane, game.PlaneSingles, game.PlanePairs:
			k = 3
		}
		l := prev.Length
		for s := pr + 1; s+l-1 <= rA; s++ {
			ok := true
			for i := 0; i < l; i++ {
				if avail[s+i] < k {
					ok = false
					break
				}
			}
			if !ok {
				continue
			}
			m := game.Move{Type: prev.Type, Rank: game.Rank(s), Length: l}
			for i := 0; i < l; i++ {
				m.Use[s+i] = k
			}
			switch prev.Type {
			case game.PlaneSingles:
				if !cheapKickers(&avail, s, s+l-1, l, 1, &m.Use) {
					continue
				}
			case game.PlanePairs:
				if !cheapKickers(&avail, s, s+l-1, l, 2, &m.Use) {
					continue
				}
			}
			emit(m)
		}
	}
	return out
}

// cheapKickers adds need kicker units (unit 1 or 2 cards of one rank) from
// avail outside lo..hi to use, lowest first: ranks holding exactly one unit
// below 2, then any rank below 2, then 2s and jokers as a last resort.
func cheapKickers(avail *counts, lo, hi, need int, unit uint8, use *counts) bool {
	got := 0
	for pass := 0; pass < 3 && got < need; pass++ {
		top := r2 - 1
		if pass == 2 {
			top = rBJ
		}
		for r := rLo; r <= top && got < need; r++ {
			if r >= lo && r <= hi {
				continue
			}
			left := avail[r] - use[r]
			if left < unit || (pass == 0 && left != unit) {
				continue
			}
			for left >= unit && got < need {
				use[r] += unit
				left -= unit
				got++
				if unit == 2 {
					break // at most one kicker pair per rank
				}
			}
		}
	}
	return got == need
}

func (p *policy) followMate(cx *ctx, dc *decomp) (game.Move, bool) {
	var buf [24]game.Move
	// Overtake a teammate only when it lets us go out right after.
	for _, m := range p.matches(cx, dc, buf[:0]) {
		if beatable(&m, &cx.outside) {
			continue
		}
		rest := cx.hand
		rest.Sub(&m.Use)
		if _, ok := game.ClassifyLead(&rest); ok {
			return m, true
		}
	}
	// Block the landlord when they are about to go out and could beat the
	// teammate's play.
	l := cx.landlord
	if cx.n[l] <= 2 && cx.actsBefore(l) && beatable(&cx.last, &cx.outside) {
		as := p.answers(cx, buf[:0])
		if len(as) > 0 {
			return as[len(as)-1], true
		}
	}
	return game.Move{}, false
}

func (p *policy) followOpp(cx *ctx, dc *decomp) (game.Move, bool) {
	prev := &cx.last
	minOpp := cx.minOpp()
	// Stop an opponent about to go out: answer with the highest card.
	if (prev.Type == game.Single && minOpp == 1) || (prev.Type == game.Pair && minOpp <= 2) {
		k := 1
		if prev.Type == game.Pair {
			k = 2
		}
		if m, ok := highestAbove(&cx.hand, int(prev.Rank), k); ok {
			return m, true
		}
	}
	var buf [24]game.Move
	if ms := p.matches(cx, dc, buf[:0]); len(ms) > 0 {
		return ms[0], true
	}
	threshold := p.prm.landlordBreak
	if cx.me != cx.landlord {
		threshold = p.prm.farmerBreak
		if (cx.me+1)&3 == cx.landlord {
			threshold = p.prm.farmerLastBreak
		}
	}
	if minOpp <= p.prm.urgentOpp || cx.n[cx.lastSeat] <= p.prm.urgentLast {
		threshold = -1000
	}
	if m, ok := p.breakAnswer(cx, dc, threshold); ok {
		return m, true
	}
	return p.bombAnswer(cx, dc, minOpp)
}

// breakAnswer looks for an answer that breaks structures, judging each by
// how much the rest of the hand loses.
func (p *policy) breakAnswer(cx *ctx, dc *decomp, threshold int) (game.Move, bool) {
	var buf [24]game.Move
	as := p.answers(cx, buf[:0])
	if len(as) == 0 {
		return game.Move{}, false
	}
	const maxEval = 6
	best, bestGain := -1, 0
	for i := range as {
		if i >= maxEval && i != len(as)-1 {
			continue // the weakest few and the strongest
		}
		rest := cx.hand
		rest.Sub(&as[i].Use)
		p.dz.decompose(&rest, p.wide, p.limit, &p.scratch)
		gain := p.scratch.value - dc.value
		if !beatable(&as[i], &cx.outside) {
			gain += p.prm.bossBonus // wins the lead back
		}
		if best < 0 || gain > bestGain {
			best, bestGain = i, gain
		}
	}
	if bestGain < threshold {
		return game.Move{}, false
	}
	return as[best], true
}

// control counts, for a decomposition, the groups that win the lead back
// (bombs and groups nobody can beat) and the other groups still needing a
// lead of their own after kickers are attached.
func control(dc *decomp, out *counts) (ctrl, small int) {
	nonBomb := 0
	for _, g := range dc.groups() {
		if g.isBomb() {
			ctrl++
			continue
		}
		nonBomb++
		m := g.move()
		if beatable(&m, out) {
			small++
		} else {
			ctrl++
		}
	}
	small -= nonBomb - dc.turns // kickers ride along with triples and planes
	return ctrl, max(small, 0)
}

// bombAnswer decides whether to bomb the current play.
func (p *policy) bombAnswer(cx *ctx, dc *decomp, minOpp int) (game.Move, bool) {
	bombOpp, bombLast := p.prm.bombOpp, p.prm.bombLast
	if cx.me != cx.landlord && p.prm.farmerBombOpp > 0 {
		bombOpp, bombLast = p.prm.farmerBombOpp, p.prm.farmerBombOpp
	}
	threat := minOpp <= bombOpp || cx.n[cx.lastSeat] <= bombLast
	gs := dc.groups()
	best, bestBoss := -1, -1
	for i, g := range gs {
		if !g.isBomb() {
			continue
		}
		m := g.move()
		if !m.Beats(cx.last) {
			continue
		}
		if best < 0 || bombLess(g, gs[best]) {
			best = i
		}
		if !beatable(&m, &cx.outside) && (bestBoss < 0 || bombLess(g, gs[bestBoss])) {
			bestBoss = i
		}
	}
	if best < 0 {
		return game.Move{}, false
	}
	if threat && bestBoss >= 0 && p.prm.bossBomb {
		best = bestBoss // an opponent close to going out must not out-bomb us
	}
	m := gs[best].move()
	rest := cx.hand
	rest.Sub(&m.Use)
	if _, ok := game.ClassifyLead(&rest); ok {
		return m, true
	}
	if threat {
		return m, true
	}
	p.dz.decompose(&rest, p.wide, p.limit, &p.scratch)
	if p.scratch.turns <= p.prm.bombTurns {
		return m, true
	}
	margin := p.prm.ctrlMargin
	if cx.me != cx.landlord && p.prm.farmerCtrl != 0 {
		margin = p.prm.farmerCtrl
	}
	if margin > -100 {
		ctrl, small := control(&p.scratch, &cx.outside)
		if ctrl >= small+margin {
			return m, true
		}
	}
	return game.Move{}, false
}
