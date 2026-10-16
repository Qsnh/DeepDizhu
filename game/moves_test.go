package game

import (
	"math/rand/v2"
	"slices"
	"testing"
	"time"
)

type sig struct {
	use RankCounts
	t   ComboType
	r   Rank
	l   int
}

// brute enumerates every distinct sub-multiset of ranks of the hand (every
// subset up to suit), picks concrete cards for it and keeps the ones that
// ParseLead accepts (or ParseFollow accepts over prev when prev != nil).
// It is independent of the move generator.
func brute(hand []Card, prev *Combo) map[RankCounts]sig {
	out := map[RankCounts]sig{}
	byRank := map[Rank][]Card{}
	var ranks []Rank
	for _, c := range hand {
		if byRank[c.Rank()] == nil {
			ranks = append(ranks, c.Rank())
		}
		byRank[c.Rank()] = append(byRank[c.Rank()], c)
	}
	var sub []Card
	var rec func(i int)
	rec = func(i int) {
		if i == len(ranks) {
			if len(sub) == 0 {
				return
			}
			var c Combo
			var ok bool
			if prev == nil {
				c, ok = ParseLead(sub)
			} else {
				c, ok = ParseFollow(sub, *prev)
			}
			if ok {
				u := CountRanks(sub)
				out[u] = sig{u, c.Type, c.Rank, c.Length}
			}
			return
		}
		cs := byRank[ranks[i]]
		for k := 0; k <= len(cs); k++ {
			n := len(sub)
			sub = append(sub, cs[:k]...)
			rec(i + 1)
			sub = sub[:n]
		}
	}
	rec(0)
	return out
}

func checkMoves(t *testing.T, name string, hand []Card, got []Combo, want map[RankCounts]sig, prev *Combo) {
	t.Helper()
	seen := map[RankCounts]bool{}
	for _, c := range got {
		u := CountRanks(c.Cards)
		if seen[u] {
			t.Errorf("%s: duplicate move %v (%s)", name, c.Describe(), CardsString(c.Cards))
		}
		seen[u] = true
		if !ownsAll(hand, c.Cards) {
			t.Errorf("%s: move %s not in hand", name, CardsString(c.Cards))
		}
		var p Combo
		var ok bool
		if prev == nil {
			p, ok = ParseLead(c.Cards)
		} else {
			p, ok = ParseFollow(c.Cards, *prev)
		}
		if !ok || p.Type != c.Type || p.Rank != c.Rank || p.Length != c.Length {
			t.Errorf("%s: move %v (%s) does not parse back (%v %v)", name, c.Describe(), CardsString(c.Cards), ok, p.Describe())
		}
		if prev != nil && !c.Beats(*prev) {
			t.Errorf("%s: move %v does not beat %v", name, c.Describe(), prev.Describe())
		}
		if _, ok := want[u]; !ok {
			t.Errorf("%s: unexpected move %v", name, c.Describe())
		}
	}
	for u, s := range want {
		if !seen[u] {
			t.Errorf("%s: missing move %v (%s)", name, describe(s.t, s.r, s.l), u.String())
		}
	}
}

func randomHand(rng *rand.Rand, n int) []Card {
	deck := NewDeck()
	rng.Shuffle(len(deck), func(i, j int) { deck[i], deck[j] = deck[j], deck[i] })
	h := slices.Clone(deck[:n])
	SortHand(h)
	return h
}

// clusteredHand draws n cards concentrated on few ranks so that bombs,
// planes and kicker combos show up often.
func clusteredHand(rng *rand.Rand, n int) []Card {
	deck := NewDeck()
	lo := R3 + Rank(rng.IntN(8))
	var pool []Card
	for _, c := range deck {
		r := c.Rank()
		if (r >= lo && r < lo+5) || r >= R2 {
			pool = append(pool, c)
		}
	}
	rng.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
	h := slices.Clone(pool[:n])
	SortHand(h)
	return h
}

func TestLegalLeadsMatchesBruteForce(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	hands := [][]Card{
		mustCards(t, "333444555666xX"),
		mustCards(t, "33334444556677"),
		mustCards(t, "3456789TJQKA2"),
		mustCards(t, "xxXX2222AAAKKQ"),
		mustCards(t, "33344455566677"),
	}
	for i := 0; i < 25; i++ {
		hands = append(hands, randomHand(rng, 16+rng.IntN(5)))
		hands = append(hands, clusteredHand(rng, 16+rng.IntN(5)))
	}
	for _, h := range hands {
		checkMoves(t, "lead "+CardsString(h), h, LegalLeads(h), brute(h, nil), nil)
	}
}

func TestLegalFollowsMatchesBruteForce(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 4))
	prevs := []string{
		"3", "T", "2", "x", "44", "QQ", "xx", "555", "6663", "77744", "34567", "456789",
		"334455", "556677", "333444", "33344456", "3334445566", "333345", "33334455",
		"3333", "44444", "2222", "777777",
	}
	for i := 0; i < 40; i++ {
		var h []Card
		if i%2 == 0 {
			h = randomHand(rng, 18)
		} else {
			h = clusteredHand(rng, 18)
		}
		for _, ps := range prevs {
			prev := lead(t, ps)
			name := "follow " + prev.Describe() + " with " + CardsString(h)
			checkMoves(t, name, h, LegalFollows(h, prev), brute(h, &prev), &prev)
		}
	}
}

func TestLegalMovesKnown(t *testing.T) {
	hand := mustCards(t, "333444555666778899TxX")
	leads := LegalLeads(hand)
	want := []string{"飞机 3-6", "飞机带单 3-5", "飞机带单 4-6", "飞机带对 3-5", "连对 7-9", "顺子 5-10", "顺子 3-10", "三带一对 6", "单张 大王", "对子 7"}
	have := map[string]bool{}
	for _, c := range leads {
		have[c.Describe()] = true
	}
	for _, w := range want {
		if !have[w] {
			t.Errorf("lead %q missing", w)
		}
	}
	if have["对子 小王"] || have["天王炸"] {
		t.Errorf("small+big joker treated as pair/bomb")
	}
	// Followers: weakest first, bombs last.
	hand = mustCards(t, "34567889TJQKA2222xxXX")
	fs := LegalFollows(hand, lead(t, "45678"))
	var got []string
	for _, c := range fs {
		got = append(got, c.Describe())
	}
	wantF := []string{"顺子 5-9", "顺子 6-10", "顺子 7-J", "顺子 8-Q", "顺子 9-K", "顺子 10-A", "炸弹×4 2", "天王炸"}
	if !slices.Equal(got, wantF) {
		t.Errorf("follows = %v, want %v", got, wantF)
	}
	if fs := LegalFollows(hand, lead(t, "xxXX")); len(fs) != 0 {
		t.Errorf("something beats the joker bomb: %v", descr(fs))
	}
	fs = LegalFollows(mustCards(t, "333333334444"), lead(t, "22222"))
	got = got[:0]
	for _, c := range fs {
		got = append(got, c.Describe())
	}
	wantB := []string{"炸弹×6 3", "炸弹×7 3", "炸弹×8 3"}
	if !slices.Equal(got, wantB) {
		t.Errorf("bomb follows = %v, want %v", got, wantB)
	}
}

func TestLegalMovesSortedAndDistinct(t *testing.T) {
	rng := rand.New(rand.NewPCG(5, 6))
	for i := 0; i < 200; i++ {
		h := randomHand(rng, 25+rng.IntN(9))
		leads := LegalLeads(h)
		for j := 1; j < len(leads); j++ {
			if compareMoves(leads[j-1].Move(), leads[j].Move()) > 0 {
				t.Fatalf("leads not sorted at %d: %v then %v", j, leads[j-1].Describe(), leads[j].Describe())
			}
		}
		seen := map[RankCounts]bool{}
		for _, c := range leads {
			u := CountRanks(c.Cards)
			if seen[u] {
				t.Fatalf("duplicate lead %v", c.Describe())
			}
			seen[u] = true
			p, ok := ParseLead(c.Cards)
			if !ok || p.Type != c.Type || p.Rank != c.Rank || p.Length != c.Length {
				t.Fatalf("lead %v reparses as %v", c.Describe(), p.Describe())
			}
		}
		prev := leads[rng.IntN(len(leads))]
		other := randomHand(rng, 25)
		fs := LegalFollows(other, prev)
		for j, c := range fs {
			if !c.Beats(prev) {
				t.Fatalf("%v does not beat %v", c.Describe(), prev.Describe())
			}
			if _, ok := ParseFollow(c.Cards, prev); !ok {
				t.Fatalf("%v rejected by ParseFollow", c.Describe())
			}
			if j > 0 && compareMoves(fs[j-1].Move(), c.Move()) > 0 {
				t.Fatalf("follows not sorted")
			}
		}
	}
}

func TestLegalLeadsPerformance(t *testing.T) {
	// 11 triples in a row is about the worst case for kicker enumeration.
	worst := mustCards(t, "333444555666777888999TTTJJJQQQKKK")
	rng := rand.New(rand.NewPCG(7, 8))
	hands := [][]Card{worst, mustCards(t, "33334444555566667777888899xxXX2")}
	for i := 0; i < 20; i++ {
		hands = append(hands, randomHand(rng, 33))
	}
	for _, h := range hands {
		if len(h) > 33 {
			t.Fatalf("hand has %d cards", len(h))
		}
		start := time.Now()
		leads := LegalLeads(h)
		el := time.Since(start)
		if el > 500*time.Millisecond {
			t.Errorf("LegalLeads on %s took %v (%d moves)", CardsString(h), el, len(leads))
		}
		if len(h) == 33 && &h[0] == &worst[0] {
			t.Logf("worst-case 33-card hand: %d leads in %v", len(leads), el)
		}
	}
}

func BenchmarkLegalLeads33(b *testing.B) {
	rng := rand.New(rand.NewPCG(9, 10))
	h := randomHand(rng, 33)
	for b.Loop() {
		LegalLeads(h)
	}
}
