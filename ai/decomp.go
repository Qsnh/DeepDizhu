package ai

import "deepdizhu/game"

// Hand decomposition: split a hand (rank counts) into a small set of groups
// (bombs kept intact, chains and planes extracted, the rest as triples,
// pairs and singles) maximising a heuristic value that rewards few "turns"
// and high cards.

type counts = game.RankCounts

const (
	rLo = int(game.R3)
	rA  = int(game.RA)
	r2  = int(game.R2)
	rSJ = int(game.RSmallJoker)
	rBJ = int(game.RBigJoker)
)

// group is one unit of a decomposition. t is a game.ComboType among Single,
// Pair, Triple, Straight, PairStraight, Plane, Bomb and JokerBomb.
type group struct {
	t uint8 // game.ComboType
	r uint8 // lowest rank
	l uint8 // chain length, bomb size, 1 otherwise
}

func (g group) typ() game.ComboType { return game.ComboType(g.t) }

// unit returns the number of cards per rank of the group.
func (g group) unit() int {
	switch game.ComboType(g.t) {
	case game.Single, game.Straight:
		return 1
	case game.Pair, game.PairStraight:
		return 2
	case game.Triple, game.Plane:
		return 3
	case game.Bomb:
		return int(g.l)
	}
	return 0
}

func (g group) size() int {
	switch game.ComboType(g.t) {
	case game.Single, game.Pair, game.Triple:
		return g.unit()
	case game.Straight, game.PairStraight, game.Plane:
		return g.unit() * int(g.l)
	case game.Bomb:
		return int(g.l)
	case game.JokerBomb:
		return 4
	}
	return 0
}

func (g group) isBomb() bool { return g.t == uint8(game.Bomb) || g.t == uint8(game.JokerBomb) }

func (g group) isChain() bool {
	return g.t == uint8(game.Straight) || g.t == uint8(game.PairStraight) || g.t == uint8(game.Plane)
}

// span returns how many ranks the group covers.
func (g group) span() int {
	if g.isChain() {
		return int(g.l)
	}
	return 1
}

// addTo adds the group's cards to c.
func (g group) addTo(c *counts) {
	if g.t == uint8(game.JokerBomb) {
		c[rSJ] += 2
		c[rBJ] += 2
		return
	}
	u := uint8(g.unit())
	for i := 0; i < g.span(); i++ {
		c[int(g.r)+i] += u
	}
}

// move returns the game move for the group itself (no kickers).
func (g group) move() game.Move {
	m := game.Move{Type: g.typ(), Rank: game.Rank(g.r), Length: int(g.l)}
	if g.t == uint8(game.JokerBomb) {
		m.Rank = game.RBigJoker
		m.Length = 4
	}
	if !g.isChain() && !g.isBomb() {
		m.Length = 1
	}
	g.addTo(&m.Use)
	return m
}

// Heuristic weights. A "turn" is a group that must be led to get rid of it;
// high cards earn back roughly what their turn costs.
const turnCost = 7

func singleW(r int) int { return r - 10 }

func pairW(r int) int {
	if r >= rSJ {
		return r - 9
	}
	return r - 10
}

func tripleW(r int) int { return r - 10 }

func chainW(t game.ComboType, lo, l int) int {
	top := lo + l - 1
	switch t {
	case game.Straight:
		return top - 10 + 1
	case game.PairStraight:
		return top - 10 + 2
	}
	return top - 10 + 3 // plane
}

func bombW(r, size int) int { return 8 + 3*(size-4) + (r-3)/4 }

const jokerBombW = 25

func groupW(g group) int {
	switch g.typ() {
	case game.Single:
		return singleW(int(g.r))
	case game.Pair:
		return pairW(int(g.r))
	case game.Triple:
		return tripleW(int(g.r))
	case game.Straight, game.PairStraight, game.Plane:
		return chainW(g.typ(), int(g.r), int(g.l))
	case game.Bomb:
		return bombW(int(g.r), int(g.l))
	case game.JokerBomb:
		return jokerBombW
	}
	return 0
}

const maxGroups = 48

// decomp is a decomposition of a hand: bombs first, then chains, then
// triples, pairs and singles in ascending rank.
type decomp struct {
	g     [maxGroups]group
	n     int
	value int // heuristic value, higher is better
	turns int // estimated number of leads needed (bombs excluded)
}

func (d *decomp) groups() []group { return d.g[:d.n] }

// decomposer searches decompositions. It is reusable scratch space; not
// safe for concurrent use.
type decomposer struct {
	w      counts
	cur    [maxGroups]group
	ncur   int
	curVal int
	curNB  int // non-bomb groups in cur
	curCap int // kicker capacity of the planes in cur
	best   *decomp
	found  bool
	nodes  int
	limit  int
	wide   bool
}

var chainUnit = [3]int{3, 2, 1}
var chainMin = [3]int{2, 3, 5}
var chainType = [3]game.ComboType{game.Plane, game.PairStraight, game.Straight}

// decompose finds a good decomposition of c into out. wide explores every
// chain start and length (bounded by limit search nodes); otherwise only the
// lowest possible start of each chain kind with its shortest and longest
// length is tried, which is much cheaper.
func (d *decomposer) decompose(c *counts, wide bool, limit int, out *decomp) {
	d.w = *c
	d.ncur, d.curVal, d.curNB, d.curCap = 0, 0, 0, 0
	d.best, d.found, d.nodes, d.limit, d.wide = out, false, 0, limit, wide
	out.n = 0
	w := &d.w
	if w[rSJ] == 2 && w[rBJ] == 2 {
		d.push(group{t: uint8(game.JokerBomb), r: uint8(rBJ), l: 4})
		w[rSJ], w[rBJ] = 0, 0
	}
	for r := rLo; r <= r2; r++ {
		if w[r] >= 4 {
			d.push(group{t: uint8(game.Bomb), r: uint8(r), l: w[r]})
			w[r] = 0
		}
	}
	d.dfs(0, rLo)
}

func (d *decomposer) push(g group) {
	d.cur[d.ncur] = g
	d.ncur++
	d.curVal += groupW(g)
	if !g.isBomb() {
		d.curNB++
		if g.t == uint8(game.Plane) {
			d.curCap += int(g.l)
		}
	}
}

func (d *decomposer) pop() {
	d.ncur--
	g := d.cur[d.ncur]
	d.curVal -= groupW(g)
	if !g.isBomb() {
		d.curNB--
		if g.t == uint8(game.Plane) {
			d.curCap -= int(g.l)
		}
	}
}

func (d *decomposer) dfs(minKind, minStart int) {
	d.nodes++
	d.leaf()
	if d.nodes >= d.limit {
		return
	}
	w := &d.w
	for kind := minKind; kind < 3; kind++ {
		k, minLen := uint8(chainUnit[kind]), chainMin[kind]
		s := rLo
		if kind == minKind {
			s = minStart
		}
		for ; s+minLen-1 <= rA; s++ {
			if w[s] < k {
				continue
			}
			run := 1
			for s+run <= rA && w[s+run] >= k {
				run++
			}
			if run < minLen {
				s += run - 1
				continue
			}
			for l := run; l >= minLen; l-- {
				if !d.wide && l != run && l != minLen {
					continue
				}
				for i := 0; i < l; i++ {
					w[s+i] -= k
				}
				d.push(group{t: uint8(chainType[kind]), r: uint8(s), l: uint8(l)})
				d.dfs(kind, s)
				d.pop()
				for i := 0; i < l; i++ {
					w[s+i] += k
				}
				if d.nodes >= d.limit {
					return
				}
			}
			if !d.wide {
				break
			}
		}
	}
}

// leaf scores the current chains plus the leftover cards as triples, pairs
// and singles, and records the decomposition if it is the best so far.
func (d *decomposer) leaf() {
	w := &d.w
	v, nb, capa, abs := d.curVal, d.curNB, d.curCap, 0
	for r := rLo; r <= rBJ; r++ {
		switch w[r] {
		case 0:
		case 1:
			v += singleW(r)
			nb++
			abs++
		case 2:
			v += pairW(r)
			nb++
			abs++
		default:
			v += tripleW(r)
			nb++
			capa++
		}
	}
	turns := nb - min(capa, abs)
	v -= turnCost * turns
	if d.found && v <= d.best.value {
		return
	}
	d.found = true
	b := d.best
	b.value, b.turns = v, turns
	b.n = copy(b.g[:], d.cur[:d.ncur])
	for _, k := range [3]uint8{3, 2, 1} {
		for r := rLo; r <= rBJ; r++ {
			if w[r] != k {
				continue
			}
			t := game.Single
			if k == 2 {
				t = game.Pair
			} else if k == 3 {
				t = game.Triple
			}
			b.g[b.n] = group{t: uint8(t), r: uint8(r), l: 1}
			b.n++
		}
	}
}

// Search limits.
const (
	fastLimit = 48   // rollouts
	fullLimit = 4000 // real decisions
)
