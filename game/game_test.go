package game

import (
	"errors"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
)

// specDeal builds a deal from rank specs, giving every card a distinct id.
func specDeal(t *testing.T, hands [4]string, bottom string) ([4][]Card, []Card) {
	t.Helper()
	all := mustCards(t, strings.Join(hands[:], "")+bottom)
	var out [4][]Card
	i := 0
	for s, h := range hands {
		n := len(strings.ReplaceAll(h, " ", ""))
		out[s] = all[i : i+n]
		i += n
	}
	return out, all[i:]
}

func ranksOf(t *testing.T, cards []Card, spec string) []Card {
	t.Helper()
	want := CountRanks(mustCards(t, spec))
	var out []Card
	for _, c := range cards {
		if want[c.Rank()] > 0 {
			want[c.Rank()]--
			out = append(out, c)
		}
	}
	if want.Total() != 0 {
		t.Fatalf("hand %s lacks %s", CardsString(cards), spec)
	}
	return out
}

func play(t *testing.T, g *Game, seat int, spec string) {
	t.Helper()
	var cards []Card
	if spec != "" {
		cards = ranksOf(t, g.Hands[seat], spec)
	}
	if err := g.Play(seat, cards); err != nil {
		t.Fatalf("seat %d plays %q: %v", seat, spec, err)
	}
}

func TestNewGameDeal(t *testing.T) {
	g := NewGame(rand.New(rand.NewPCG(1, 1)), 2, Config{})
	if g.Phase != PhaseBidding || g.Turn != 2 || g.FirstBidder != 2 || g.Landlord != -1 || g.LastSeat != -1 {
		t.Fatalf("bad initial state %+v", g)
	}
	var all []Card
	for s := range 4 {
		if len(g.Hands[s]) != 25 {
			t.Errorf("seat %d has %d cards", s, len(g.Hands[s]))
		}
		if !slices.IsSortedFunc(g.Hands[s], cardOrder) {
			t.Errorf("seat %d hand not sorted", s)
		}
		all = append(all, g.Hands[s]...)
		if g.Bids[s] != -1 {
			t.Errorf("bid %d = %d", s, g.Bids[s])
		}
	}
	if len(g.Bottom) != 8 {
		t.Fatalf("bottom has %d cards", len(g.Bottom))
	}
	all = append(all, g.Bottom...)
	slices.Sort(all)
	if !slices.Equal(all, NewDeck()) {
		t.Fatal("deal is not a permutation of the deck")
	}
	if v := g.View(0); v.Bottom != nil || len(v.Unseen()) != 83 {
		t.Errorf("bidding view: bottom %v, %d unseen", v.Bottom, len(v.Unseen()))
	}
	if g.Config.base() != 1 {
		t.Error("default base score should be 1")
	}
}

func TestBidding(t *testing.T) {
	newG := func(first int) *Game { return NewGame(rand.New(rand.NewPCG(2, 2)), first, Config{}) }

	g := newG(1)
	if err := g.Bid(0, 1); !errors.Is(err, ErrNotYourTurn) {
		t.Errorf("out of turn bid: %v", err)
	}
	if err := g.Bid(1, 4); !errors.Is(err, ErrBidRange) {
		t.Errorf("bid 4: %v", err)
	}
	if err := g.Play(1, g.Hands[1][:1]); !errors.Is(err, ErrNotPlaying) {
		t.Errorf("play during bidding: %v", err)
	}
	must := func(seat, pts int) {
		t.Helper()
		if err := g.Bid(seat, pts); err != nil {
			t.Fatalf("Bid(%d,%d): %v", seat, pts, err)
		}
	}
	must(1, 1)
	if err := g.Bid(2, 1); !errors.Is(err, ErrBidTooLow) {
		t.Errorf("equal bid: %v", err)
	}
	if got := g.BidChoices(); !slices.Equal(got, []int{0, 2, 3}) {
		t.Errorf("BidChoices = %v", got)
	}
	must(2, 0)
	must(3, 2)
	bottom := slices.Clone(g.Bottom)
	before := slices.Clone(g.Hands[3])
	must(0, 0)
	if g.Phase != PhasePlaying || g.Landlord != 3 || g.HighestBid != 2 || g.Turn != 3 || !g.Leading() {
		t.Fatalf("after bidding: phase %v landlord %d bid %d turn %d", g.Phase, g.Landlord, g.HighestBid, g.Turn)
	}
	if len(g.Hands[3]) != 33 {
		t.Fatalf("landlord has %d cards", len(g.Hands[3]))
	}
	want := append(before, bottom...)
	SortHand(want)
	if !slices.Equal(g.Hands[3], want) {
		t.Error("landlord hand is not hand + bottom")
	}
	if len(g.Bottom) != 8 {
		t.Error("bottom should stay visible")
	}
	if !slices.Equal(g.Bids[:], []int{0, 1, 0, 2}) {
		t.Errorf("bids %v", g.Bids)
	}
	if err := g.Bid(0, 3); !errors.Is(err, ErrNotBidding) {
		t.Errorf("bid after bidding: %v", err)
	}
	if len(g.History) != 4 || g.History[0].Kind != ActBid || g.History[2].Bid != 2 {
		t.Errorf("history %+v", g.History)
	}
	v := g.View(1)
	if len(v.Bottom) != 8 || v.HandCounts[3] != 33 || v.Landlord != 3 {
		t.Errorf("farmer view %+v", v)
	}
	if k := v.KnownInHand(3); len(k) != 8 {
		t.Errorf("farmer should know the 8 bottom cards, got %d", len(k))
	}
	if k := g.View(3).KnownInHand(3); k != nil {
		t.Errorf("landlord KnownInHand(self) = %v", k)
	}
	if !g.IsTeammate(0, 1) || g.IsTeammate(0, 3) || !g.IsTeammate(3, 3) || g.IsTeammate(3, 2) {
		t.Error("IsTeammate wrong")
	}

	// A bid of 3 ends bidding at once.
	g = newG(2)
	must(2, 0)
	must(3, 3)
	if g.Phase != PhasePlaying || g.Landlord != 3 || g.HighestBid != 3 || g.Bids[0] != -1 || g.Bids[1] != -1 {
		t.Errorf("bid 3: phase %v landlord %d bids %v", g.Phase, g.Landlord, g.Bids)
	}

	// The first bidder may win with a 1 if nobody overbids.
	g = newG(0)
	must(0, 1)
	must(1, 0)
	must(2, 0)
	must(3, 0)
	if g.Landlord != 0 || g.HighestBid != 1 || g.Turn != 0 {
		t.Errorf("landlord %d bid %d turn %d", g.Landlord, g.HighestBid, g.Turn)
	}

	// Everybody passes: void deal.
	g = newG(3)
	for _, s := range []int{3, 0, 1, 2} {
		must(s, 0)
	}
	if g.Phase != PhaseRedeal || g.Landlord != -1 {
		t.Errorf("all pass: phase %v landlord %d", g.Phase, g.Landlord)
	}
	if err := g.Bid(3, 1); !errors.Is(err, ErrNotBidding) {
		t.Errorf("bid after redeal: %v", err)
	}
}

func TestTrickFlow(t *testing.T) {
	hands, bottom := specDeal(t, [4]string{"33456", "789TJ", "QKA22", "445566x"}, "666")
	g := NewGameFromDeal(hands, bottom, 0, Config{})
	if err := g.Bid(0, 3); err != nil {
		t.Fatal(err)
	}
	if !g.Leading() || g.CanPass(0) {
		t.Fatal("landlord should lead and may not pass")
	}
	if err := g.Play(0, nil); !errors.Is(err, ErrLeaderMustPlay) {
		t.Errorf("leader pass: %v", err)
	}
	if err := g.Play(1, g.Hands[1][:1]); !errors.Is(err, ErrNotYourTurn) {
		t.Errorf("out of turn: %v", err)
	}
	if err := g.Play(0, g.Hands[1][:1]); !errors.Is(err, ErrNotOwned) {
		t.Errorf("foreign card: %v", err)
	}
	if err := g.Play(0, ranksOf(t, g.Hands[0], "34")); !errors.Is(err, ErrInvalidCombo) {
		t.Errorf("invalid combo: %v", err)
	}
	c := g.Hands[0][0]
	if err := g.Play(0, []Card{c, c}); !errors.Is(err, ErrInvalidCombo) {
		t.Errorf("duplicate card: %v", err)
	}
	play(t, g, 0, "3")
	if g.Leading() || !g.CanPass(1) || g.LastSeat != 0 || g.LastCombo.Type != Single {
		t.Fatal("trick state after lead")
	}
	if len(g.LegalMoves(1)) != 5 || g.LegalMoves(2) != nil {
		t.Errorf("legal moves: %d for seat 1, %v for seat 2", len(g.LegalMoves(1)), g.LegalMoves(2))
	}
	if err := g.Play(1, ranksOf(t, g.Hands[1], "78")); !errors.Is(err, ErrInvalidCombo) {
		t.Errorf("invalid follow: %v", err)
	}
	play(t, g, 1, "7")
	if err := g.Play(2, ranksOf(t, g.Hands[2], "22")); !errors.Is(err, ErrMustBeat) {
		t.Errorf("pair on single: %v", err)
	}
	play(t, g, 2, "")
	play(t, g, 3, "")
	if g.Leading() {
		t.Fatal("trick ended too early")
	}
	play(t, g, 0, "")
	// Everybody passed after seat 1: seat 1 leads a new trick.
	if !g.Leading() || g.Turn != 1 || g.LastSeat != -1 || g.LastCombo != nil {
		t.Fatalf("new trick: turn %d leading %v", g.Turn, g.Leading())
	}
	play(t, g, 1, "8")
	play(t, g, 2, "Q")
	play(t, g, 3, "x")
	play(t, g, 0, "")
	play(t, g, 1, "")
	play(t, g, 2, "")
	if g.Turn != 3 || !g.Leading() {
		t.Fatalf("seat 3 should lead, turn %d", g.Turn)
	}
	play(t, g, 3, "445566")
	if g.Phase != PhaseFinished {
		t.Fatal("game should be over")
	}
	r := g.Result
	if r.LandlordWon || r.Winner != 3 || r.Spring || !r.AntiSpring || r.Multiplier != 6 {
		t.Errorf("result %+v", r)
	}
	if r.Scores != [4]int{-18, 6, 6, 6} {
		t.Errorf("scores %v", r.Scores)
	}
	if g.PlayCounts != [4]int{1, 2, 1, 2} || len(g.Played[3]) != 7 {
		t.Errorf("play counts %v", g.PlayCounts)
	}
	if err := g.Play(0, nil); !errors.Is(err, ErrNotPlaying) {
		t.Errorf("play after end: %v", err)
	}
	passes := 0
	for _, a := range g.History {
		if a.Kind == ActPass {
			passes++
		}
	}
	if passes != 6 {
		t.Errorf("%d passes recorded", passes)
	}
}

func TestSpringWithBombs(t *testing.T) {
	hands, bottom := specDeal(t, [4]string{"333345678", "89TJQ", "KA2xX", "44556"}, "9999A")
	g := NewGameFromDeal(hands, bottom, 0, Config{BaseScore: 5})
	if err := g.Bid(0, 2); err != nil {
		t.Fatal(err)
	}
	for _, s := range []int{1, 2, 3} {
		if err := g.Bid(s, 0); err != nil {
			t.Fatal(err)
		}
	}
	steps := []string{"3333", "9999", "45678", "A"}
	for _, st := range steps {
		play(t, g, 0, st)
		if g.Phase == PhaseFinished {
			break
		}
		play(t, g, 1, "")
		play(t, g, 2, "")
		play(t, g, 3, "")
	}
	r := g.Result
	if r == nil {
		t.Fatalf("not finished, hand %s", CardsString(g.Hands[0]))
	}
	// bid 2 × 2^2 bombs × 2 spring = 16; unit 80.
	if !r.LandlordWon || !r.Spring || r.AntiSpring || r.Bombs != 2 || r.Multiplier != 16 || r.Bid != 2 {
		t.Errorf("result %+v", r)
	}
	if r.Scores != [4]int{240, -80, -80, -80} {
		t.Errorf("scores %v", r.Scores)
	}
}

func TestMultiplierCap(t *testing.T) {
	for _, tc := range []struct {
		cap, mult int
		capped    bool
	}{{0, 16, false}, {32, 16, false}, {16, 16, false}, {8, 8, true}} {
		hands, bottom := specDeal(t, [4]string{"333345678", "89TJQ", "KA2xX", "44556"}, "9999A")
		g := NewGameFromDeal(hands, bottom, 0, Config{BaseScore: 5, MaxMultiplier: tc.cap})
		g.Bid(0, 2)
		for _, s := range []int{1, 2, 3} {
			g.Bid(s, 0)
		}
		for _, st := range []string{"3333", "9999", "45678", "A"} {
			play(t, g, 0, st)
			if g.Phase == PhaseFinished {
				break
			}
			play(t, g, 1, "")
			play(t, g, 2, "")
			play(t, g, 3, "")
		}
		r := g.Result
		unit := 5 * tc.mult
		if r == nil || r.Multiplier != tc.mult || r.Capped != tc.capped || r.Scores != [4]int{3 * unit, -unit, -unit, -unit} {
			t.Errorf("cap %d: result %+v", tc.cap, r)
		}
	}
}

func TestNoSpringAndJokerBomb(t *testing.T) {
	hands, bottom := specDeal(t, [4]string{"344", "xxXX67", "789", "TJQ"}, "K")
	g := NewGameFromDeal(hands, bottom, 0, Config{})
	if err := g.Bid(0, 1); err != nil {
		t.Fatal(err)
	}
	for _, s := range []int{1, 2, 3} {
		if err := g.Bid(s, 0); err != nil {
			t.Fatal(err)
		}
	}
	script := []struct {
		seat int
		spec string
	}{
		{0, "3"}, {1, "xxXX"}, {2, ""}, {3, ""}, {0, ""},
		{1, "6"}, {2, ""}, {3, ""}, {0, "K"}, {1, ""}, {2, ""}, {3, ""},
		{0, "44"},
	}
	for _, st := range script {
		play(t, g, st.seat, st.spec)
	}
	r := g.Result
	if r == nil || !r.LandlordWon || r.Spring || r.AntiSpring || r.Bombs != 1 || r.Multiplier != 2 {
		t.Fatalf("result %+v", r)
	}
	if r.Scores != [4]int{6, -2, -2, -2} {
		t.Errorf("scores %v", r.Scores)
	}
	if g.Bombs != 1 || g.PlayCounts[1] != 2 {
		t.Errorf("bombs %d, plays %v", g.Bombs, g.PlayCounts)
	}
}

func TestViewAndClone(t *testing.T) {
	g := NewGame(rand.New(rand.NewPCG(4, 4)), 0, Config{})
	if err := g.Bid(0, 3); err != nil {
		t.Fatal(err)
	}
	lead := g.LegalMoves(0)[0]
	if err := g.Play(0, lead.Cards); err != nil {
		t.Fatal(err)
	}
	v := g.View(1)
	if len(v.Hand) != 25 || v.HandCounts != [4]int{32, 25, 25, 25} || v.LastCombo == nil || v.LastSeat != 0 {
		t.Fatalf("view %+v", v)
	}
	if n := len(v.Unseen()); n != 108-25-1 {
		t.Errorf("farmer unseen = %d", n)
	}
	known := v.KnownInHand(0)
	for _, c := range known {
		if !slices.Contains(g.Hands[0], c) {
			t.Errorf("known card %v not in landlord hand", c)
		}
	}
	if len(known) != 8 && len(known) != 7 {
		t.Errorf("known = %d", len(known))
	}
	if v.KnownInHand(2) != nil {
		t.Error("KnownInHand for a farmer seat")
	}
	if n := len(g.View(0).Unseen()); n != 108-32-1 {
		t.Errorf("landlord unseen = %d", n)
	}
	// The view is a deep copy.
	h0, l0, hist0 := g.Hands[1][0], g.LastCombo.Cards[0], g.History[len(g.History)-1].Combo.Cards[0]
	const bad = Card(255)
	v.Hand[0] = bad
	v.LastCombo.Cards[0] = bad
	v.History[len(v.History)-1].Combo.Cards[0] = bad
	v.Played[0][0] = bad
	if g.Hands[1][0] != h0 || g.LastCombo.Cards[0] != l0 || g.History[len(g.History)-1].Combo.Cards[0] != hist0 || g.Played[0][0] == bad {
		t.Error("view aliases the game")
	}
	c := g.Clone()
	if err := c.Play(1, nil); err != nil {
		t.Fatal(err)
	}
	if g.Turn != 1 || len(g.History) == len(c.History) {
		t.Error("clone aliases the game")
	}
	c.Hands[0][0] = bad
	c.LastCombo.Cards[0] = bad
	c.Played[0][0] = bad
	if g.Hands[0][0] == bad || g.LastCombo.Cards[0] == bad || g.Played[0][0] == bad {
		t.Error("clone shares slices")
	}
}

// TestRandomGames plays random legal moves to the end many times and checks
// invariants.
func TestRandomGames(t *testing.T) {
	rng := rand.New(rand.NewPCG(5, 5))
	for i := 0; i < 300; i++ {
		g := NewGame(rng, i%4, Config{BaseScore: 1 + i%3})
		for g.Phase == PhaseBidding {
			ch := g.BidChoices()
			if err := g.Bid(g.Turn, ch[rng.IntN(len(ch))]); err != nil {
				t.Fatal(err)
			}
		}
		if g.Phase == PhaseRedeal {
			continue
		}
		for steps := 0; g.Phase == PhasePlaying; steps++ {
			if steps > 2000 {
				t.Fatal("game does not terminate")
			}
			s := g.Turn
			moves := g.LegalMoves(s)
			if g.CanPass(s) && (len(moves) == 0 || rng.IntN(3) == 0) {
				if err := g.Play(s, nil); err != nil {
					t.Fatal(err)
				}
				continue
			}
			if len(moves) == 0 {
				t.Fatal("leader without moves")
			}
			m := moves[rng.IntN(len(moves))]
			if err := g.Play(s, m.Cards); err != nil {
				t.Fatalf("legal move %v rejected: %v", m.Describe(), err)
			}
		}
		total := 0
		cards := 0
		for s := range 4 {
			total += g.Result.Scores[s]
			cards += len(g.Hands[s]) + len(g.Played[s])
		}
		if total != 0 {
			t.Fatalf("scores %v do not sum to zero", g.Result.Scores)
		}
		if cards != 108 {
			t.Fatalf("%d cards accounted for", cards)
		}
		if len(g.Hands[g.Result.Winner]) != 0 {
			t.Fatal("winner still has cards")
		}
	}
}
