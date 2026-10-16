package game

import (
	"slices"
)

// Move generation works on rank counts: cards of equal rank are
// interchangeable, so every distinct move is identified by its rank
// signature (Move.Use). Kickers are enumerated as rank multisets. Concrete
// cards are picked deterministically afterwards (Move.PickCards).

// gen collects candidate rank signatures.
type gen struct {
	hand *RankCounts
	uses []RankCounts
}

func (g *gen) emit(u *RankCounts) { g.uses = append(g.uses, *u) }

// singles/pairs/triples/bombs of one rank, rank > minRank.
func (g *gen) sets(k uint8, maxRank, minRank Rank) {
	for r := max(minRank+1, MinRank); r <= maxRank; r++ {
		if g.hand[r] >= k {
			var u RankCounts
			u[r] = k
			g.emit(&u)
		}
	}
}

// chains of unit k (1 straight, 2 pair straight, 3 plane). If length > 0
// only that length is generated; starts must be > minStart.
func (g *gen) chains(k uint8, minLen, length int, minStart Rank) {
	for s := max(minStart+1, MinRank); s <= RA; s++ {
		run := 0
		for r := s; r <= RA && g.hand[r] >= k; r++ {
			run++
		}
		for l := minLen; l <= run; l++ {
			if length > 0 && l != length {
				continue
			}
			var u RankCounts
			for i := 0; i < l; i++ {
				u[s+Rank(i)] = k
			}
			g.emit(&u)
		}
	}
}

// kickers enumerates multisets of need kicker units (unit 1 = single,
// 2 = pair) from ranks >= from that are not in body lo..hi, adding them to
// base and emitting each completed signature.
func (g *gen) kickers(base *RankCounts, lo, hi Rank, need int, unit uint8, from Rank) {
	if need == 0 {
		g.emit(base)
		return
	}
	for r := from; r <= MaxRank; r++ {
		if r >= lo && r <= hi {
			continue
		}
		avail := int(g.hand[r] / unit)
		if avail == 0 {
			continue
		}
		for t := 1; t <= min(avail, need); t++ {
			base[r] = uint8(t) * unit
			g.kickers(base, lo, hi, need-t, unit, r+1)
		}
		base[r] = 0
	}
}

// withKickers generates a body of k cards on each rank of lo..hi plus
// units kickers of the given unit size.
func (g *gen) withKickers(lo, hi Rank, k uint8, units int, unit uint8) {
	var base RankCounts
	for r := lo; r <= hi; r++ {
		base[r] = k
	}
	g.kickers(&base, lo, hi, units, unit, MinRank)
}

func (g *gen) tripleKickers(unit uint8, minRank Rank) {
	for r := max(minRank+1, R3); r <= R2; r++ {
		if g.hand[r] >= 3 {
			g.withKickers(r, r, 3, 1, unit)
		}
	}
}

func (g *gen) fourKickers(unit uint8, minRank Rank) {
	for r := max(minRank+1, R3); r <= R2; r++ {
		if g.hand[r] >= 4 {
			g.withKickers(r, r, 4, 2, unit)
		}
	}
}

// planeKickers generates planes of length (or every length >= 2 when 0)
// with one kicker unit per triple.
func (g *gen) planeKickers(unit uint8, length int, minStart Rank) {
	total := g.hand.Total()
	for s := max(minStart+1, R3); s <= RA; s++ {
		run := 0
		for r := s; r <= RA && g.hand[r] >= 3; r++ {
			run++
		}
		for l := 2; l <= run; l++ {
			if length > 0 && l != length {
				continue
			}
			if l*(3+int(unit)) > total {
				break
			}
			g.withKickers(s, s+Rank(l-1), 3, l, unit)
		}
	}
}

// bombs generates every bomb (each size 4..count) beating prev when
// beatPrev is set, or every bomb otherwise.
func (g *gen) bombs(prev *Move) {
	for r := R3; r <= R2; r++ {
		for k := 4; k <= int(g.hand[r]); k++ {
			if prev != nil && !beats(Bomb, r, k, prev.Type, prev.Rank, prev.Length) {
				continue
			}
			var u RankCounts
			u[r] = uint8(k)
			g.emit(&u)
		}
	}
	if g.hand[RSmallJoker] >= 2 && g.hand[RBigJoker] >= 2 {
		if prev == nil || prev.Type != JokerBomb {
			var u RankCounts
			u[RSmallJoker], u[RBigJoker] = 2, 2
			g.emit(&u)
		}
	}
}

// finish dedupes the signatures and classifies each one, either as a lead
// (prev == nil) or as an answer to prev.
func (g *gen) finish(prev *Move) []Move {
	seen := make(map[RankCounts]struct{}, len(g.uses))
	out := make([]Move, 0, len(g.uses))
	for i := range g.uses {
		u := &g.uses[i]
		if _, dup := seen[*u]; dup {
			continue
		}
		seen[*u] = struct{}{}
		var m Move
		var ok bool
		if prev == nil {
			m, ok = ClassifyLead(u)
		} else {
			m, ok = ClassifyFollow(u, *prev)
		}
		if ok {
			out = append(out, m)
		}
	}
	SortMoves(out)
	return out
}

// GenLeads returns every distinct move that may lead a trick from the hand
// counted in c, sorted weakest first (see SortMoves). Each move carries the
// interpretation ParseLead would choose for its cards.
func GenLeads(c *RankCounts) []Move {
	g := gen{hand: c}
	g.sets(1, MaxRank, 0)
	g.sets(2, MaxRank, 0)
	g.sets(3, R2, 0)
	g.tripleKickers(1, 0)
	g.tripleKickers(2, 0)
	g.chains(1, 5, 0, 0)
	g.chains(2, 3, 0, 0)
	g.chains(3, 2, 0, 0)
	g.planeKickers(1, 0, 0)
	g.planeKickers(2, 0, 0)
	g.fourKickers(1, 0)
	g.fourKickers(2, 0)
	g.bombs(nil)
	return g.finish(nil)
}

// GenFollows returns every distinct move from the hand counted in c that
// beats prev (passing excluded), sorted weakest first (see SortMoves).
func GenFollows(c *RankCounts, prev Move) []Move {
	g := gen{hand: c}
	r := prev.Rank
	switch prev.Type {
	case Invalid:
		return GenLeads(c)
	case Single:
		g.sets(1, MaxRank, r)
	case Pair:
		g.sets(2, MaxRank, r)
	case Triple:
		g.sets(3, R2, r)
	case TripleSingle:
		g.tripleKickers(1, r)
	case TriplePair:
		g.tripleKickers(2, r)
	case Straight:
		g.chains(1, 5, prev.Length, r)
	case PairStraight:
		g.chains(2, 3, prev.Length, r)
	case Plane:
		g.chains(3, 2, prev.Length, r)
	case PlaneSingles:
		g.planeKickers(1, prev.Length, r)
	case PlanePairs:
		g.planeKickers(2, prev.Length, r)
	case FourTwoSingles:
		g.fourKickers(1, r)
	case FourTwoPairs:
		g.fourKickers(2, r)
	}
	g.bombs(&prev)
	return g.finish(&prev)
}

// SortMoves sorts moves weakest first: non-bombs by main rank, then type,
// then length, then lighter kickers; bombs after all non-bombs by strength
// (size, then rank), the joker bomb last.
func SortMoves(ms []Move) {
	slices.SortFunc(ms, compareMoves)
}

func compareMoves(a, b Move) int {
	ab, bb := a.IsBomb(), b.IsBomb()
	if ab != bb {
		if ab {
			return 1
		}
		return -1
	}
	if ab {
		aj, bj := a.Type == JokerBomb, b.Type == JokerBomb
		if aj != bj {
			if aj {
				return 1
			}
			return -1
		}
		if a.Length != b.Length {
			return a.Length - b.Length
		}
		return int(a.Rank) - int(b.Rank)
	}
	if a.Rank != b.Rank {
		return int(a.Rank) - int(b.Rank)
	}
	if a.Type != b.Type {
		return int(a.Type) - int(b.Type)
	}
	if a.Length != b.Length {
		return a.Length - b.Length
	}
	wa, wb := rankWeight(&a.Use), rankWeight(&b.Use)
	if wa != wb {
		return wa - wb
	}
	for r := MaxRank; r >= MinRank; r-- {
		if a.Use[r] != b.Use[r] {
			return int(a.Use[r]) - int(b.Use[r])
		}
	}
	return 0
}

func rankWeight(u *RankCounts) int {
	w := 0
	for r := MinRank; r <= MaxRank; r++ {
		w += int(u[r]) * int(r)
	}
	return w
}

func movesToCombos(ms []Move, hand []Card) []Combo {
	sorted := slices.Clone(hand)
	SortHand(sorted)
	out := make([]Combo, len(ms))
	for i, m := range ms {
		out[i] = m.Combo(sorted)
	}
	return out
}

// LegalLeads returns every distinct move that may lead a trick, sorted
// weakest to strongest (non-bombs by rank, then bombs by strength).
func LegalLeads(hand []Card) []Combo {
	c := CountRanks(hand)
	return movesToCombos(GenLeads(&c), hand)
}

// LegalFollows returns every distinct move that beats prev (passing
// excluded), sorted weakest to strongest: non-bombs by rank first, then
// bombs by strength.
func LegalFollows(hand []Card, prev Combo) []Combo {
	c := CountRanks(hand)
	return movesToCombos(GenFollows(&c, prev.Move()), hand)
}
