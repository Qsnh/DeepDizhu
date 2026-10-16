package game

import (
	"testing"
)

func mustCards(t testing.TB, spec string) []Card {
	t.Helper()
	cs, err := CardsFromSpec(spec)
	if err != nil {
		t.Fatalf("CardsFromSpec(%q): %v", spec, err)
	}
	return cs
}

func TestRankSuitCard(t *testing.T) {
	if R3 != 3 || R10 != 10 || RJ != 11 || RA != 14 || R2 != 15 || RSmallJoker != 16 || RBigJoker != 17 {
		t.Fatal("rank values changed")
	}
	names := map[Rank]string{R3: "3", R10: "10", RJ: "J", RA: "A", R2: "2", RSmallJoker: "小王", RBigJoker: "大王"}
	for r, n := range names {
		if r.String() != n {
			t.Errorf("%d.String() = %q, want %q", r, r.String(), n)
		}
	}
	deck := NewDeck()
	if len(deck) != 108 {
		t.Fatalf("deck has %d cards", len(deck))
	}
	var counts RankCounts
	suits := map[Suit]int{}
	for _, c := range deck {
		counts[c.Rank()]++
		suits[c.Suit()]++
	}
	for r := R3; r <= R2; r++ {
		if counts[r] != 8 {
			t.Errorf("rank %v has %d copies", r, counts[r])
		}
	}
	if counts[RSmallJoker] != 2 || counts[RBigJoker] != 2 {
		t.Errorf("jokers: %d small, %d big", counts[RSmallJoker], counts[RBigJoker])
	}
	if suits[Joker] != 4 || suits[Spade] != 26 || suits[Diamond] != 26 {
		t.Errorf("suit counts %v", suits)
	}
	if c := NewCard(RA, Spade, 0); c.String() != "♠A" || c.Rank() != RA || c.Suit() != Spade {
		t.Errorf("NewCard(A,♠) = %v", c)
	}
	if c := NewCard(R10, Heart, 1); c.String() != "♥10" || !c.Suit().IsRed() || c < 54 {
		t.Errorf("NewCard(10,♥,1) = %v (%d)", c, c)
	}
	sj, bj := NewCard(RSmallJoker, Joker, 0), NewCard(RBigJoker, Joker, 1)
	if sj.String() != "小王" || bj.String() != "大王" || !bj.IsRedJoker() || sj.IsRedJoker() {
		t.Errorf("jokers %v %v", sj, bj)
	}
	if Joker.Symbol() != "" || Joker.IsRed() || Spade.Symbol() != "♠" || !Diamond.IsRed() || Club.IsRed() {
		t.Error("suit symbols/colours wrong")
	}
}

func TestSortHand(t *testing.T) {
	hand := []Card{
		NewCard(R3, Diamond, 0), NewCard(RBigJoker, Joker, 0), NewCard(R3, Spade, 1),
		NewCard(R2, Heart, 0), NewCard(RSmallJoker, Joker, 1), NewCard(RA, Club, 0),
		NewCard(R3, Spade, 0), NewCard(R10, Heart, 0),
	}
	SortHand(hand)
	got := CardsString(hand)
	want := "大王 小王 ♥2 ♣A ♥10 ♠3 ♠3 ♦3"
	if got != want {
		t.Fatalf("SortHand = %q, want %q", got, want)
	}
	if hand[5] != NewCard(R3, Spade, 0) || hand[6] != NewCard(R3, Spade, 1) {
		t.Errorf("equal cards not ordered by id: %v %v", hand[5], hand[6])
	}
}

type parseCase struct {
	spec  string
	types []ComboType // all interpretations, lead preference first; nil = invalid
	rank  Rank        // of the lead interpretation
	len   int         // of the lead interpretation
}

func TestParseCombos(t *testing.T) {
	cases := []parseCase{
		// singles and pairs
		{"3", []ComboType{Single}, R3, 1},
		{"x", []ComboType{Single}, RSmallJoker, 1},
		{"X", []ComboType{Single}, RBigJoker, 1},
		{"33", []ComboType{Pair}, R3, 1},
		{"22", []ComboType{Pair}, R2, 1},
		{"xx", []ComboType{Pair}, RSmallJoker, 1},
		{"XX", []ComboType{Pair}, RBigJoker, 1},
		{"xX", nil, 0, 0},
		{"34", nil, 0, 0},
		// triples
		{"333", []ComboType{Triple}, R3, 1},
		{"222", []ComboType{Triple}, R2, 1},
		{"xxX", nil, 0, 0},
		{"3334", []ComboType{TripleSingle}, R3, 1},
		{"333X", []ComboType{TripleSingle}, R3, 1},
		{"2223", []ComboType{TripleSingle}, R2, 1},
		{"33344", []ComboType{TriplePair}, R3, 1},
		{"333xx", []ComboType{TriplePair}, R3, 1},
		{"333XX", []ComboType{TriplePair}, R3, 1},
		{"333xX", nil, 0, 0},
		{"33345", nil, 0, 0},
		// straights
		{"34567", []ComboType{Straight}, R3, 5},
		{"TJQKA", []ComboType{Straight}, R10, 5},
		{"3456789TJQKA", []ComboType{Straight}, R3, 12},
		{"JQKA2", nil, 0, 0},
		{"QKA2x", nil, 0, 0},
		{"3456", nil, 0, 0},
		{"34568", nil, 0, 0},
		{"345672", nil, 0, 0},
		// pair straights
		{"334455", []ComboType{PairStraight}, R3, 3},
		{"QQKKAA", []ComboType{PairStraight}, RQ, 3},
		{"33445566778899TTJJQQKKAA", []ComboType{PairStraight}, R3, 12},
		{"3344", nil, 0, 0},
		{"KKAA22", nil, 0, 0},
		{"334466", nil, 0, 0},
		// planes
		{"333444", []ComboType{Plane}, R3, 2},
		{"KKKAAA", []ComboType{Plane}, RK, 2},
		{"333444555", []ComboType{Plane}, R3, 3},
		{"AAA222", nil, 0, 0},
		{"333555", nil, 0, 0},
		{"33344456", []ComboType{PlaneSingles}, R3, 2},
		{"33344455", []ComboType{PlaneSingles}, R3, 2},
		{"333444xX", []ComboType{PlaneSingles}, R3, 2},
		{"333444X2", []ComboType{PlaneSingles}, R3, 2},
		{"33334445", nil, 0, 0}, // kicker shares a rank with the body
		{"333444555789", []ComboType{PlaneSingles}, R3, 3},
		{"333444555777", []ComboType{PlaneSingles}, R3, 3},
		{"3334445566", []ComboType{PlanePairs}, R3, 2},
		{"333444xxXX", []ComboType{PlanePairs}, R3, 2},
		{"3334445555", []ComboType{PlanePairs}, R3, 2},
		{"333444xX55", nil, 0, 0},
		{"3334445567", nil, 0, 0},
		{"KKKAAA2222", []ComboType{PlanePairs}, RK, 2},
		{"AAA222345", nil, 0, 0}, // 2 cannot be in a plane
		// four with two
		{"333345", []ComboType{FourTwoSingles}, R3, 1},
		{"333344", []ComboType{FourTwoSingles}, R3, 1},
		{"3333xX", []ComboType{FourTwoSingles}, R3, 1},
		{"22223X", []ComboType{FourTwoSingles}, R2, 1},
		{"33334455", []ComboType{FourTwoPairs}, R3, 1},
		{"33335555", []ComboType{FourTwoPairs, FourTwoPairs}, R5, 1},
		{"3333xxXX", []ComboType{FourTwoPairs}, R3, 1},
		{"3333xX55", nil, 0, 0},
		{"3333456", nil, 0, 0},
		// bombs
		{"3333", []ComboType{Bomb}, R3, 4},
		{"22222", []ComboType{Bomb}, R2, 5},
		{"777777", []ComboType{Bomb}, R7, 6},
		{"8888888", []ComboType{Bomb}, R8, 7},
		{"AAAAAAAA", []ComboType{Bomb}, RA, 8},
		{"xxXX", []ComboType{JokerBomb}, RBigJoker, 4},
		{"xxX", nil, 0, 0},
		{"xXX", nil, 0, 0},
		// ambiguous hands
		{"333444555666", []ComboType{Plane, PlaneSingles, PlaneSingles}, R3, 4},
		{"333444555666777888", []ComboType{Plane}, R3, 6},
		{"3333344", nil, 0, 0},
	}
	for _, tc := range cases {
		cards := mustCards(t, tc.spec)
		got := ParseCombos(cards)
		if len(got) != len(tc.types) {
			t.Errorf("%s: got %d interpretations %v, want %v", tc.spec, len(got), descr(got), tc.types)
			continue
		}
		for i, c := range got {
			if c.Type != tc.types[i] {
				t.Errorf("%s: interpretation %d is %v, want %v", tc.spec, i, c.Type, tc.types[i])
			}
			if len(c.Cards) != len(cards) {
				t.Errorf("%s: combo has %d cards", tc.spec, len(c.Cards))
			}
		}
		lead, ok := ParseLead(cards)
		if ok != (len(tc.types) > 0) {
			t.Errorf("%s: ParseLead ok=%v", tc.spec, ok)
			continue
		}
		if ok && (lead.Type != tc.types[0] || lead.Rank != tc.rank || lead.Length != tc.len) {
			t.Errorf("%s: ParseLead = %v %v len %d, want %v %v len %d", tc.spec, lead.Type, lead.Rank, lead.Length, tc.types[0], tc.rank, tc.len)
		}
	}
}

func descr(cs []Combo) []string {
	var s []string
	for _, c := range cs {
		s = append(s, c.Describe())
	}
	return s
}

func TestParseRejectsBadCards(t *testing.T) {
	c := NewCard(R5, Spade, 0)
	if cs := ParseCombos([]Card{c, c}); len(cs) != 0 {
		t.Errorf("duplicate card parsed as %v", descr(cs))
	}
	if cs := ParseCombos([]Card{200}); len(cs) != 0 {
		t.Errorf("invalid id parsed")
	}
	if cs := ParseCombos(nil); len(cs) != 0 {
		t.Errorf("empty parsed")
	}
	// Two physical copies of the same card are different cards.
	if cs := ParseCombos([]Card{NewCard(R5, Spade, 0), NewCard(R5, Spade, 1)}); len(cs) != 1 || cs[0].Type != Pair {
		t.Errorf("two ♠5 from both decks: %v", descr(cs))
	}
}

func TestComboCardsSorted(t *testing.T) {
	c, ok := ParseLead(mustCards(t, "7 3 5 4 6"))
	if !ok || CardsString(c.Cards) != "♠7 ♠6 ♠5 ♠4 ♠3" {
		t.Fatalf("got %v %q", ok, CardsString(c.Cards))
	}
}

func lead(t *testing.T, spec string) Combo {
	t.Helper()
	c, ok := ParseLead(mustCards(t, spec))
	if !ok {
		t.Fatalf("%s does not parse", spec)
	}
	return c
}

func TestBeats(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"4", "3", true},
		{"3", "3", false},
		{"3", "4", false},
		{"X", "x", true},
		{"x", "2", true},
		{"2", "A", true},
		{"44", "33", true},
		{"XX", "xx", true},
		{"xx", "22", true},
		{"44", "3", false},
		{"4", "33", false},
		{"444", "333", true},
		{"4443", "3335", true},
		{"44433", "3335", false},
		{"44433", "33355", true},
		{"45678", "34567", true},
		{"456789", "34567", false},
		{"34567", "45678", false},
		{"445566", "334455", true},
		{"44556677", "334455", false},
		{"444555", "333444", true},
		{"44455567", "33344456", true},
		{"444555", "33344456", false},
		{"4445556677", "3334445566", true},
		{"4445556677", "33344456", false},
		{"444456", "333378", true},
		{"44445566", "333345", false},
		{"44445566", "33337788", true},
		// bombs
		{"3333", "X", true},
		{"3333", "333444555666777888", true},
		{"3333", "3456789TJQKA", true},
		{"4444", "3333", true},
		{"33333", "2222", true},
		{"2222", "33333", false},
		{"22222", "33333", true},
		{"333333", "22222", true},
		{"33333333", "2222222", true},
		{"X", "3333", false},
		{"xxXX", "33333333", true},
		{"xxXX", "X", true},
		{"33333333", "xxXX", false},
		{"222222", "3333", true},
	}
	for _, tc := range cases {
		a, b := lead(t, tc.a), lead(t, tc.b)
		if got := a.Beats(b); got != tc.want {
			t.Errorf("%s (%v) beats %s (%v) = %v, want %v", tc.a, a.Describe(), tc.b, b.Describe(), got, tc.want)
		}
	}
	// Same rank, same size bombs from the two decks do not beat each other.
	b1 := Combo{Type: Bomb, Rank: R7, Length: 4}
	if b1.Beats(b1) {
		t.Error("equal bombs beat each other")
	}
	if !(Combo{Type: Single, Rank: R3, Length: 1}).Beats(Combo{}) {
		t.Error("a valid combo should beat nothing")
	}
	if (Combo{}).Beats(Combo{Type: Single, Rank: R3, Length: 1}) {
		t.Error("invalid combo beats")
	}
}

func TestParseFollowAmbiguous(t *testing.T) {
	cases := []struct {
		cards, prev string
		ok          bool
		typ         ComboType
		rank        Rank
	}{
		// 333444555666 led as a 4-plane; as an answer to 3-plane-with-singles
		// it is read as a 3-plane with singles.
		{"444555666777", "333444555789", true, PlaneSingles, R5},
		{"444555666777", "333444555666", true, Plane, R4},
		{"333444555666", "33344455578T", true, PlaneSingles, R4},
		{"333444555666", "444555666789", false, Invalid, 0},
		{"33334444", "33335566", true, FourTwoPairs, R4},
		{"33334444", "55556677", false, Invalid, 0},
		{"55556666", "33334444", true, FourTwoPairs, R6},
		{"3333", "XX", true, Bomb, R3},
		{"34567", "3", false, Invalid, 0},
		{"xxXX", "22222222", true, JokerBomb, RBigJoker},
	}
	for _, tc := range cases {
		prev := lead(t, tc.prev)
		got, ok := ParseFollow(mustCards(t, tc.cards), prev)
		if ok != tc.ok {
			t.Errorf("%s over %s: ok=%v want %v", tc.cards, tc.prev, ok, tc.ok)
			continue
		}
		if ok && (got.Type != tc.typ || got.Rank != tc.rank) {
			t.Errorf("%s over %s: %v, want %v %v", tc.cards, tc.prev, got.Describe(), tc.typ, tc.rank)
		}
	}
}

func TestDescribe(t *testing.T) {
	cases := map[string]string{
		"34567":        "顺子 3-7",
		"777777":       "炸弹×6 7",
		"KK":           "对子 K",
		"X":            "单张 大王",
		"xxXX":         "天王炸",
		"334455":       "连对 3-5",
		"TTTJJJ":       "飞机 10-J",
		"3334445566":   "飞机带对 3-4",
		"33344456":     "飞机带单 3-4",
		"QQQ5":         "三带一 Q",
		"QQQ55":        "三带一对 Q",
		"999934":       "四带二 9",
		"99993344":     "四带两对 9",
		"999":          "三张 9",
		"3456789TJQKA": "顺子 3-A",
	}
	for spec, want := range cases {
		if got := lead(t, spec).Describe(); got != want {
			t.Errorf("%s: Describe = %q, want %q", spec, got, want)
		}
	}
	names := []string{"单张", "对子", "三张", "三带一", "三带一对", "顺子", "连对", "飞机", "飞机带单", "飞机带对", "四带二", "四带两对", "炸弹", "天王炸"}
	for i, n := range names {
		if ComboType(i+1).String() != n {
			t.Errorf("ComboType(%d) = %q, want %q", i+1, ComboType(i+1).String(), n)
		}
	}
}
