package game

import "slices"

// View is what one seat may see of a game. It is a deep copy, safe to hand
// to another goroutine.
type View struct {
	Seat       int
	Phase      Phase
	Hand       []Card
	HandCounts [NumSeats]int
	Landlord   int
	Bottom     []Card // nil during bidding; public after the landlord is decided
	Bids       [NumSeats]int
	HighestBid int
	Turn       int
	LastCombo  *Combo
	LastSeat   int
	Played     [NumSeats][]Card
	PlayCounts [NumSeats]int
	Bombs      int
	History    []Action
}

// View returns a deep copy of what seat may see.
func (g *Game) View(seat int) View {
	v := View{
		Seat:       seat,
		Phase:      g.Phase,
		Landlord:   g.Landlord,
		Bids:       g.Bids,
		HighestBid: g.HighestBid,
		Turn:       g.Turn,
		LastSeat:   g.LastSeat,
		PlayCounts: g.PlayCounts,
		Bombs:      g.Bombs,
		History:    cloneHistory(g.History),
	}
	if validSeat(seat) {
		v.Hand = slices.Clone(g.Hands[seat])
	}
	for s := range NumSeats {
		v.HandCounts[s] = len(g.Hands[s])
		v.Played[s] = slices.Clone(g.Played[s])
	}
	if g.Landlord >= 0 {
		v.Bottom = slices.Clone(g.Bottom)
	}
	if g.LastCombo != nil {
		lc := g.LastCombo.Clone()
		v.LastCombo = &lc
	}
	return v
}

// Leading reports whether the seat to act starts a new trick.
func (v View) Leading() bool { return v.Phase == PhasePlaying && v.LastCombo == nil }

// MyTurn reports whether the viewer is the seat to act in a bidding or
// playing phase.
func (v View) MyTurn() bool {
	return v.Turn == v.Seat && (v.Phase == PhaseBidding || v.Phase == PhasePlaying)
}

// IsTeammate reports whether a and b are on the same side (see
// Game.IsTeammate).
func (v View) IsTeammate(a, b int) bool { return isTeammate(v.Landlord, a, b) }

// BidChoices returns the legal bids during bidding (0 = 不叫).
func (v View) BidChoices() []int {
	if v.Phase != PhaseBidding {
		return nil
	}
	return bidChoices(v.HighestBid)
}

// LegalMoves returns the viewer's legal plays (passing excluded) when it is
// their turn to play, weakest first.
func (v View) LegalMoves() []Combo {
	if v.Phase != PhasePlaying || v.Turn != v.Seat {
		return nil
	}
	if v.LastCombo == nil {
		return LegalLeads(v.Hand)
	}
	return LegalFollows(v.Hand, *v.LastCombo)
}

// Unseen returns every card whose location the viewer cannot see: the deck
// minus the viewer's hand minus all played cards. For farmers this still
// includes the unplayed bottom cards, which are known to be in the
// landlord's hand (see KnownInHand).
func (v View) Unseen() []Card {
	var seen [NumCards]bool
	for _, c := range v.Hand {
		if c.Valid() {
			seen[c] = true
		}
	}
	for _, p := range v.Played {
		for _, c := range p {
			if c.Valid() {
				seen[c] = true
			}
		}
	}
	out := make([]Card, 0, NumCards)
	for i := range NumCards {
		if !seen[i] {
			out = append(out, Card(i))
		}
	}
	return out
}

// KnownInHand returns the cards the viewer knows to be in seat's hand
// without seeing it: for the landlord seat (when the viewer is not the
// landlord), the bottom cards not yet played. Otherwise nil.
func (v View) KnownInHand(seat int) []Card {
	if v.Landlord < 0 || seat != v.Landlord || v.Seat == v.Landlord || !validSeat(seat) {
		return nil
	}
	var played [NumCards]bool
	for _, c := range v.Played[seat] {
		if c.Valid() {
			played[c] = true
		}
	}
	var out []Card
	for _, c := range v.Bottom {
		if !played[c] {
			out = append(out, c)
		}
	}
	return out
}
