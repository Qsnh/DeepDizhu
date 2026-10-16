package game

import (
	"fmt"
	"slices"
)

// ComboType is the kind of a played combination.
type ComboType int

// Combo types.
const (
	Invalid ComboType = iota
	Single
	Pair
	Triple
	TripleSingle
	TriplePair
	Straight
	PairStraight
	Plane
	PlaneSingles
	PlanePairs
	FourTwoSingles
	FourTwoPairs
	Bomb
	JokerBomb
)

var comboNames = [...]string{
	Invalid:        "无效",
	Single:         "单张",
	Pair:           "对子",
	Triple:         "三张",
	TripleSingle:   "三带一",
	TriplePair:     "三带一对",
	Straight:       "顺子",
	PairStraight:   "连对",
	Plane:          "飞机",
	PlaneSingles:   "飞机带单",
	PlanePairs:     "飞机带对",
	FourTwoSingles: "四带二",
	FourTwoPairs:   "四带两对",
	Bomb:           "炸弹",
	JokerBomb:      "天王炸",
}

// String returns the Chinese name of the combo type.
func (t ComboType) String() string {
	if t >= Invalid && t <= JokerBomb {
		return comboNames[t]
	}
	return comboNames[Invalid]
}

// IsChain reports whether the type is a chain (straight, pair straight or a
// plane variant), whose Rank is the lowest rank of the chain.
func (t ComboType) IsChain() bool { return t >= Straight && t <= PlanePairs }

// Combo is a parsed play.
type Combo struct {
	Type ComboType
	// Rank is the main rank used for comparison: for chains and planes the
	// LOWEST rank of the chain; for everything else the body rank (the big
	// joker for the joker bomb).
	Rank Rank
	// Length is the number of chain units for Straight/PairStraight/Plane*,
	// the card count for Bomb (4..8), 4 for JokerBomb and 1 otherwise.
	Length int
	// Cards are the actual cards, display-sorted.
	Cards []Card
}

// IsBomb reports whether the combo is a bomb or the joker bomb.
func (c Combo) IsBomb() bool { return c.Type == Bomb || c.Type == JokerBomb }

// Beats reports whether c may be played on top of prev. An Invalid prev
// (nothing to beat) is beaten by any valid combo.
func (c Combo) Beats(prev Combo) bool {
	return beats(c.Type, c.Rank, c.Length, prev.Type, prev.Rank, prev.Length)
}

// Move returns the rank-count form of the combo.
func (c Combo) Move() Move {
	return Move{Type: c.Type, Rank: c.Rank, Length: c.Length, Use: CountRanks(c.Cards)}
}

// Clone returns a deep copy of the combo.
func (c Combo) Clone() Combo {
	c.Cards = slices.Clone(c.Cards)
	return c
}

// Describe returns a short Chinese label, e.g. "顺子 3-7", "炸弹×6 K",
// "对子 K", "天王炸".
func (c Combo) Describe() string { return describe(c.Type, c.Rank, c.Length) }

func describe(t ComboType, r Rank, l int) string {
	switch t {
	case Single, Pair, Triple, TripleSingle, TriplePair, FourTwoSingles, FourTwoPairs:
		return fmt.Sprintf("%s %s", t, r)
	case Straight, PairStraight, Plane, PlaneSingles, PlanePairs:
		return fmt.Sprintf("%s %s-%s", t, r, r+Rank(l-1))
	case Bomb:
		return fmt.Sprintf("炸弹×%d %s", l, r)
	case JokerBomb:
		return "天王炸"
	}
	return Invalid.String()
}

func beats(t ComboType, r Rank, l int, pt ComboType, pr Rank, pl int) bool {
	switch {
	case t == Invalid:
		return false
	case pt == Invalid:
		return true
	case t == JokerBomb:
		return pt != JokerBomb
	case pt == JokerBomb:
		return false
	case t == Bomb:
		if pt != Bomb {
			return true
		}
		if l != pl {
			return l > pl
		}
		return r > pr
	case pt == Bomb:
		return false
	}
	return t == pt && l == pl && r > pr
}

// Move is a combo described by rank counts only: which ranks (and how many
// of each) it uses. Suits never matter, so a Move stands for every set of
// concrete cards with the same ranks. It is the allocation-free form used by
// move generation and the AI.
type Move struct {
	Type   ComboType
	Rank   Rank
	Length int
	Use    RankCounts
}

// IsBomb reports whether the move is a bomb or the joker bomb.
func (m Move) IsBomb() bool { return m.Type == Bomb || m.Type == JokerBomb }

// Beats reports whether m may be played on top of prev (see Combo.Beats).
func (m Move) Beats(prev Move) bool {
	return beats(m.Type, m.Rank, m.Length, prev.Type, prev.Rank, prev.Length)
}

// Size returns the number of cards in the move.
func (m Move) Size() int { return m.Use.Total() }

// Describe returns the same label as Combo.Describe.
func (m Move) Describe() string { return describe(m.Type, m.Rank, m.Length) }

// PickCards picks concrete cards for the move from hand deterministically:
// for each rank the first cards of that rank in hand order. The result is in
// hand order. It returns nil if the hand lacks the cards.
func (m Move) PickCards(hand []Card) []Card {
	need := m.Use
	n := need.Total()
	out := make([]Card, 0, n)
	for _, c := range hand {
		r := c.Rank()
		if need[r] > 0 {
			need[r]--
			out = append(out, c)
		}
	}
	if len(out) != n {
		return nil
	}
	return out
}

// Combo converts the move into a Combo with cards picked from hand (see
// PickCards). The cards are display-sorted.
func (m Move) Combo(hand []Card) Combo {
	cards := m.PickCards(hand)
	SortHand(cards)
	return Combo{Type: m.Type, Rank: m.Rank, Length: m.Length, Cards: cards}
}

// leadBetter orders interpretations of the same cards by how natural they
// are as a lead: bombs first, then the longest chain, then the highest main
// rank, then the lower type.
func leadBetter(a, b Move) bool {
	ab, bb := a.IsBomb(), b.IsBomb()
	if ab != bb {
		return ab
	}
	if a.Length != b.Length {
		return a.Length > b.Length
	}
	if a.Rank != b.Rank {
		return a.Rank > b.Rank
	}
	return a.Type < b.Type
}

// classify appends every interpretation of the cards counted in c to out.
func classify(c *RankCounts, out []Move) []Move {
	n, distinct := 0, 0
	var lo, hi Rank
	var maxc, minc uint8 = 0, 255
	for r := MinRank; r <= MaxRank; r++ {
		k := c[r]
		if k == 0 {
			continue
		}
		n += int(k)
		distinct++
		if lo == 0 {
			lo = r
		}
		hi = r
		maxc = max(maxc, k)
		minc = min(minc, k)
	}
	if n == 0 {
		return out
	}
	add := func(t ComboType, r Rank, l int) {
		out = append(out, Move{Type: t, Rank: r, Length: l, Use: *c})
	}
	if distinct == 1 {
		switch {
		case n == 1:
			add(Single, lo, 1)
		case n == 2:
			add(Pair, lo, 1)
		case n == 3 && lo <= R2:
			add(Triple, lo, 1)
		case n >= 4 && lo <= R2:
			add(Bomb, lo, n)
		}
		return out
	}
	if n == 4 && c[RSmallJoker] == 2 && c[RBigJoker] == 2 {
		add(JokerBomb, RBigJoker, 4)
		return out
	}
	if hi <= RA && int(hi-lo)+1 == distinct && maxc == minc {
		switch {
		case maxc == 1 && distinct >= 5:
			add(Straight, lo, distinct)
		case maxc == 2 && distinct >= 3:
			add(PairStraight, lo, distinct)
		case maxc == 3:
			add(Plane, lo, distinct)
		}
	}
	switch n {
	case 4, 5:
		// Triple with one single, or with one pair.
		for r := R3; r <= R2; r++ {
			if c[r] != 3 {
				continue
			}
			if n == 4 {
				add(TripleSingle, r, 1)
			} else if distinct == 2 {
				add(TriplePair, r, 1)
			}
		}
	case 6:
		for r := R3; r <= R2; r++ {
			if c[r] == 4 {
				add(FourTwoSingles, r, 1)
			}
		}
	case 8:
		for r := R3; r <= R2; r++ {
			if c[r] == 4 && othersEven(c, r, r) {
				add(FourTwoPairs, r, 1)
			}
		}
	}
	// Planes with kickers: the body ranks must hold exactly three cards each,
	// since kickers may not share a rank with the body.
	if n%4 == 0 && n >= 8 {
		k := n / 4
		for s := R3; int(s)+k-1 <= int(RA); s++ {
			if runOfExactly(c, s, k, 3) {
				add(PlaneSingles, s, k)
			}
		}
	}
	if n%5 == 0 && n >= 10 {
		k := n / 5
		for s := R3; int(s)+k-1 <= int(RA); s++ {
			if runOfExactly(c, s, k, 3) && othersEven(c, s, s+Rank(k-1)) {
				add(PlanePairs, s, k)
			}
		}
	}
	return out
}

func runOfExactly(c *RankCounts, s Rank, k int, want uint8) bool {
	for i := 0; i < k; i++ {
		if c[s+Rank(i)] != want {
			return false
		}
	}
	return true
}

// othersEven reports whether every rank outside lo..hi has an even count, so
// those cards split into real pairs (a small and a big joker never pair).
func othersEven(c *RankCounts, lo, hi Rank) bool {
	for r := MinRank; r <= MaxRank; r++ {
		if (r < lo || r > hi) && c[r]%2 != 0 {
			return false
		}
	}
	return true
}

func sortByLeadPreference(ms []Move) {
	slices.SortStableFunc(ms, func(a, b Move) int {
		if leadBetter(a, b) {
			return -1
		}
		if leadBetter(b, a) {
			return 1
		}
		return 0
	})
}

// ClassifyCounts returns every interpretation of the cards counted in c,
// most natural lead first (see ParseLead).
func ClassifyCounts(c *RankCounts) []Move {
	ms := classify(c, nil)
	sortByLeadPreference(ms)
	return ms
}

// ClassifyLead returns the interpretation ParseLead would choose.
func ClassifyLead(c *RankCounts) (Move, bool) {
	var buf [8]Move
	ms := classify(c, buf[:0])
	if len(ms) == 0 {
		return Move{}, false
	}
	best := 0
	for i := 1; i < len(ms); i++ {
		if leadBetter(ms[i], ms[best]) {
			best = i
		}
	}
	return ms[best], true
}

// ClassifyFollow returns the interpretation ParseFollow would choose: the
// most natural interpretation that beats prev.
func ClassifyFollow(c *RankCounts, prev Move) (Move, bool) {
	var buf [8]Move
	ms := classify(c, buf[:0])
	best := -1
	for i := range ms {
		if ms[i].Beats(prev) && (best < 0 || leadBetter(ms[i], ms[best])) {
			best = i
		}
	}
	if best < 0 {
		return Move{}, false
	}
	return ms[best], true
}

// distinctValid reports whether all cards are valid and pairwise distinct.
func distinctValid(cards []Card) bool {
	var seen [NumCards]bool
	for _, c := range cards {
		if !c.Valid() || seen[c] {
			return false
		}
		seen[c] = true
	}
	return true
}

// ParseCombos returns every interpretation of the cards (empty if they form
// no valid combo, contain duplicates or invalid ids). The result is ordered
// by lead preference (see ParseLead). Each combo gets its own display-sorted
// copy of the cards.
func ParseCombos(cards []Card) []Combo {
	if len(cards) == 0 || !distinctValid(cards) {
		return nil
	}
	c := CountRanks(cards)
	ms := ClassifyCounts(&c)
	if len(ms) == 0 {
		return nil
	}
	sorted := slices.Clone(cards)
	SortHand(sorted)
	out := make([]Combo, len(ms))
	for i, m := range ms {
		out[i] = Combo{Type: m.Type, Rank: m.Rank, Length: m.Length, Cards: slices.Clone(sorted)}
	}
	return out
}

// ParseLead returns the most natural interpretation of the cards for leading
// a trick. Preference order: a literal bomb / joker bomb first; otherwise the
// interpretation with the longest chain (e.g. 333444555666 is a 4-unit plane
// rather than a 3-unit plane with three singles); then the highest main rank
// (e.g. 33334444 is 四带两对 of 4s).
func ParseLead(cards []Card) (Combo, bool) {
	cs := ParseCombos(cards)
	if len(cs) == 0 {
		return Combo{}, false
	}
	return cs[0], true
}

// ParseFollow returns an interpretation of the cards that beats prev, using
// the same preference order as ParseLead among the ones that beat it.
func ParseFollow(cards []Card, prev Combo) (Combo, bool) {
	for _, c := range ParseCombos(cards) {
		if c.Beats(prev) {
			return c, true
		}
	}
	return Combo{}, false
}
