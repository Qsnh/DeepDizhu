package game

import (
	"errors"
	"math/rand/v2"
	"slices"
)

// Table layout.
const (
	NumSeats    = 4
	HandSize    = 25
	BottomSize  = 8
	MaxBid      = 3
	LandlordMax = HandSize + BottomSize
)

// Phase is the stage of a deal.
type Phase int

// Phases.
const (
	PhaseBidding Phase = iota
	PhasePlaying
	PhaseFinished
	PhaseRedeal // everybody passed during bidding; the deal is void
)

// ActionKind is the kind of a recorded action.
type ActionKind int

// Action kinds.
const (
	ActBid ActionKind = iota
	ActPlay
	ActPass
)

// Action is one entry of the game history. Bids (including 不叫, Bid 0) use
// ActBid; plays carry their Combo; passes during play use ActPass.
type Action struct {
	Seat  int
	Kind  ActionKind
	Bid   int
	Combo Combo
}

// Config holds the table settings.
type Config struct {
	BaseScore int // default 1 when <= 0
	// MaxMultiplier caps the multiplier of a deal (封顶); 0 or less leaves
	// it uncapped. With two decks bombs are common, and each doubles it.
	MaxMultiplier int
}

func (c Config) base() int {
	if c.BaseScore <= 0 {
		return 1
	}
	return c.BaseScore
}

// Result is the outcome of a finished deal.
type Result struct {
	LandlordWon bool
	Winner      int // seat that emptied their hand
	Spring      bool
	AntiSpring  bool
	Bid         int
	Bombs       int
	Multiplier  int           // after the cap
	Capped      bool          // the multiplier reached Config.MaxMultiplier
	Scores      [NumSeats]int // per-seat delta
}

// Errors returned to the player.
var (
	ErrBadSeat        = errors.New("座位号无效")
	ErrNotYourTurn    = errors.New("还没轮到你")
	ErrNotBidding     = errors.New("现在不是叫分阶段")
	ErrNotPlaying     = errors.New("现在不是出牌阶段")
	ErrBidRange       = errors.New("只能叫1分、2分、3分或不叫")
	ErrBidTooLow      = errors.New("叫分必须高于当前最高分")
	ErrNotOwned       = errors.New("你没有这些牌")
	ErrInvalidCombo   = errors.New("出的牌不符合规则")
	ErrMustBeat       = errors.New("必须大过上家")
	ErrLeaderMustPlay = errors.New("首家出牌不能不出")
)

// Game is the full state of one deal. Fields are exported for display and
// for tests; mutate it only through Bid and Play.
type Game struct {
	Phase       Phase
	Hands       [NumSeats][]Card // kept display-sorted
	Bottom      []Card           // the 8 bottom cards (kept for display after the landlord takes them)
	FirstBidder int
	Bids        [NumSeats]int // -1 not acted yet, 0 pass, 1..3
	HighestBid  int
	Landlord    int    // -1 until decided
	Turn        int    // seat to act
	LastCombo   *Combo // last non-pass play in the current trick; nil when Turn leads a new trick
	LastSeat    int    // seat of LastCombo, -1 if none
	Played      [NumSeats][]Card
	PlayCounts  [NumSeats]int // number of non-pass plays per seat
	Bombs       int           // bombs (incl. joker bombs) played this deal
	History     []Action
	Result      *Result
	Config      Config
}

// NewGame shuffles two decks with rng (a random source is used when rng is
// nil), deals 25 cards to each seat and 8 bottom cards, and starts bidding
// with firstBidder.
func NewGame(rng *rand.Rand, firstBidder int, cfg Config) *Game {
	if rng == nil {
		rng = rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))
	}
	deck := NewDeck()
	rng.Shuffle(len(deck), func(i, j int) { deck[i], deck[j] = deck[j], deck[i] })
	var hands [NumSeats][]Card
	for s := range hands {
		hands[s] = deck[s*HandSize : (s+1)*HandSize]
	}
	return NewGameFromDeal(hands, deck[NumSeats*HandSize:], firstBidder, cfg)
}

// NewGameFromDeal starts bidding on a given deal (for tests and replays).
// The slices are copied.
func NewGameFromDeal(hands [NumSeats][]Card, bottom []Card, firstBidder int, cfg Config) *Game {
	g := &Game{
		Phase:       PhaseBidding,
		Bottom:      slices.Clone(bottom),
		FirstBidder: ((firstBidder % NumSeats) + NumSeats) % NumSeats,
		Bids:        [NumSeats]int{-1, -1, -1, -1},
		Landlord:    -1,
		LastSeat:    -1,
		Config:      cfg,
	}
	g.Turn = g.FirstBidder
	for s := range hands {
		g.Hands[s] = slices.Clone(hands[s])
		SortHand(g.Hands[s])
	}
	SortHand(g.Bottom)
	return g
}

func validSeat(s int) bool { return s >= 0 && s < NumSeats }

// BidChoices returns the bids the seat to act may make: 0 (不叫) and every
// value above the current highest bid.
func (g *Game) BidChoices() []int {
	if g.Phase != PhaseBidding {
		return nil
	}
	return bidChoices(g.HighestBid)
}

func bidChoices(highest int) []int {
	out := []int{0}
	for b := highest + 1; b <= MaxBid; b++ {
		out = append(out, b)
	}
	return out
}

// Bid records a bid of points (0 = pass / 不叫, else 1..3 and strictly
// above the current highest bid). A bid of 3 ends bidding at once; after all
// four seats have acted the highest bidder becomes the landlord, or the deal
// becomes void (PhaseRedeal) if everyone passed.
func (g *Game) Bid(seat, points int) error {
	if g.Phase != PhaseBidding {
		return ErrNotBidding
	}
	if !validSeat(seat) {
		return ErrBadSeat
	}
	if seat != g.Turn {
		return ErrNotYourTurn
	}
	if points < 0 || points > MaxBid {
		return ErrBidRange
	}
	if points != 0 && points <= g.HighestBid {
		return ErrBidTooLow
	}
	g.Bids[seat] = points
	g.History = append(g.History, Action{Seat: seat, Kind: ActBid, Bid: points})
	if points > 0 {
		g.HighestBid = points
	}
	if points == MaxBid {
		g.startPlay(seat)
		return nil
	}
	acted := 0
	for _, b := range g.Bids {
		if b >= 0 {
			acted++
		}
	}
	if acted == NumSeats {
		if g.HighestBid == 0 {
			g.Phase = PhaseRedeal
			return nil
		}
		for s, b := range g.Bids {
			if b == g.HighestBid {
				g.startPlay(s)
				break
			}
		}
		return nil
	}
	g.Turn = (seat + 1) % NumSeats
	return nil
}

func (g *Game) startPlay(landlord int) {
	g.Landlord = landlord
	g.Hands[landlord] = append(g.Hands[landlord], g.Bottom...)
	SortHand(g.Hands[landlord])
	g.Phase = PhasePlaying
	g.Turn = landlord
	g.LastCombo = nil
	g.LastSeat = -1
}

// Leading reports whether the seat to act starts a new trick.
func (g *Game) Leading() bool { return g.Phase == PhasePlaying && g.LastCombo == nil }

// CanPass reports whether seat may pass now (it is their turn and they are
// not leading).
func (g *Game) CanPass(seat int) bool {
	return g.Phase == PhasePlaying && seat == g.Turn && g.LastCombo != nil
}

// IsTeammate reports whether a and b are on the same side: the same seat, or
// both farmers once the landlord is known.
func (g *Game) IsTeammate(a, b int) bool {
	return isTeammate(g.Landlord, a, b)
}

func isTeammate(landlord, a, b int) bool {
	if a == b {
		return true
	}
	return landlord >= 0 && a != landlord && b != landlord && validSeat(a) && validSeat(b)
}

// LegalMoves returns the moves seat may make now (leads or follows, passing
// excluded), weakest first; nil when it is not seat's turn to play.
func (g *Game) LegalMoves(seat int) []Combo {
	if g.Phase != PhasePlaying || seat != g.Turn {
		return nil
	}
	if g.LastCombo == nil {
		return LegalLeads(g.Hands[seat])
	}
	return LegalFollows(g.Hands[seat], *g.LastCombo)
}

// ownsAll reports whether hand contains every card of cards (no duplicates).
func ownsAll(hand, cards []Card) bool {
	var in [NumCards]bool
	for _, c := range hand {
		if c.Valid() {
			in[c] = true
		}
	}
	for _, c := range cards {
		if !c.Valid() || !in[c] {
			return false
		}
		in[c] = false
	}
	return true
}

// Play makes seat play cards; an empty slice passes. It validates the turn,
// ownership and legality, then updates the trick, hands, Played, PlayCounts,
// Bombs and History, and finishes the deal when the hand is emptied.
func (g *Game) Play(seat int, cards []Card) error {
	if g.Phase != PhasePlaying {
		return ErrNotPlaying
	}
	if !validSeat(seat) {
		return ErrBadSeat
	}
	if seat != g.Turn {
		return ErrNotYourTurn
	}
	if len(cards) == 0 {
		if g.LastCombo == nil {
			return ErrLeaderMustPlay
		}
		g.History = append(g.History, Action{Seat: seat, Kind: ActPass})
		next := (seat + 1) % NumSeats
		if next == g.LastSeat {
			g.LastCombo = nil
			g.LastSeat = -1
		}
		g.Turn = next
		return nil
	}
	if !ownsAll(g.Hands[seat], cards) {
		if !distinctValid(cards) {
			return ErrInvalidCombo
		}
		return ErrNotOwned
	}
	var combo Combo
	var ok bool
	if g.LastCombo == nil {
		if combo, ok = ParseLead(cards); !ok {
			return ErrInvalidCombo
		}
	} else if combo, ok = ParseFollow(cards, *g.LastCombo); !ok {
		if len(ParseCombos(cards)) == 0 {
			return ErrInvalidCombo
		}
		return ErrMustBeat
	}
	g.Hands[seat] = removeCards(g.Hands[seat], cards)
	g.Played[seat] = append(g.Played[seat], combo.Cards...)
	g.PlayCounts[seat]++
	if combo.IsBomb() {
		g.Bombs++
	}
	g.History = append(g.History, Action{Seat: seat, Kind: ActPlay, Combo: combo})
	last := combo
	g.LastCombo = &last
	g.LastSeat = seat
	if len(g.Hands[seat]) == 0 {
		g.finish(seat)
		return nil
	}
	g.Turn = (seat + 1) % NumSeats
	return nil
}

func removeCards(hand, cards []Card) []Card {
	var rm [NumCards]bool
	for _, c := range cards {
		rm[c] = true
	}
	out := hand[:0:0]
	for _, c := range hand {
		if !rm[c] {
			out = append(out, c)
		}
	}
	return out
}

func (g *Game) finish(winner int) {
	l := g.Landlord
	lw := winner == l
	r := &Result{LandlordWon: lw, Winner: winner, Bid: g.HighestBid, Bombs: g.Bombs}
	if lw {
		r.Spring = true
		for s := range NumSeats {
			if s != l && g.PlayCounts[s] > 0 {
				r.Spring = false
			}
		}
	} else {
		r.AntiSpring = g.PlayCounts[l] == 1
	}
	mult := g.HighestBid
	for range g.Bombs {
		mult *= 2
	}
	if r.Spring || r.AntiSpring {
		mult *= 2
	}
	if m := g.Config.MaxMultiplier; m > 0 && mult > m {
		mult, r.Capped = m, true
	}
	r.Multiplier = mult
	unit := g.Config.base() * mult
	for s := range NumSeats {
		sign := 1
		if (s == l) != lw {
			sign = -1
		}
		if s == l {
			r.Scores[s] = sign * 3 * unit
		} else {
			r.Scores[s] = sign * unit
		}
	}
	g.Result = r
	g.Phase = PhaseFinished
}

// Clone returns a deep copy of the game.
func (g *Game) Clone() *Game {
	c := *g
	for s := range NumSeats {
		c.Hands[s] = slices.Clone(g.Hands[s])
		c.Played[s] = slices.Clone(g.Played[s])
	}
	c.Bottom = slices.Clone(g.Bottom)
	if g.LastCombo != nil {
		lc := g.LastCombo.Clone()
		c.LastCombo = &lc
	}
	c.History = cloneHistory(g.History)
	if g.Result != nil {
		r := *g.Result
		c.Result = &r
	}
	return &c
}

func cloneHistory(h []Action) []Action {
	if h == nil {
		return nil
	}
	out := make([]Action, len(h))
	for i, a := range h {
		a.Combo = a.Combo.Clone()
		out[i] = a
	}
	return out
}
