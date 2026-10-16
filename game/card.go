// Package game implements the rules of 4-player Dou Dizhu (斗地主) played
// with two decks: cards, combo parsing and comparison, move generation and
// the game state machine (bidding, playing, scoring).
package game

import (
	"errors"
	"slices"
	"strings"
)

// Rank is a card rank. Larger values beat smaller ones; suits never matter.
type Rank uint8

// Ranks from lowest to highest.
const (
	R3 Rank = 3 + iota
	R4
	R5
	R6
	R7
	R8
	R9
	R10
	RJ
	RQ
	RK
	RA
	R2
	RSmallJoker
	RBigJoker
)

// MinRank and MaxRank bound the valid ranks.
const (
	MinRank = R3
	MaxRank = RBigJoker
)

var rankNames = [...]string{
	R3: "3", R4: "4", R5: "5", R6: "6", R7: "7", R8: "8", R9: "9", R10: "10",
	RJ: "J", RQ: "Q", RK: "K", RA: "A", R2: "2", RSmallJoker: "小王", RBigJoker: "大王",
}

// String returns the display name of the rank.
func (r Rank) String() string {
	if r >= MinRank && r <= MaxRank {
		return rankNames[r]
	}
	return "?"
}

// Valid reports whether r is one of the 15 ranks.
func (r Rank) Valid() bool { return r >= MinRank && r <= MaxRank }

// Suit is a card suit. Jokers have the Joker suit.
type Suit uint8

// Suits.
const (
	Spade Suit = iota
	Heart
	Club
	Diamond
	Joker
)

// Symbol returns the suit symbol, or "" for jokers.
func (s Suit) Symbol() string {
	switch s {
	case Spade:
		return "♠"
	case Heart:
		return "♥"
	case Club:
		return "♣"
	case Diamond:
		return "♦"
	}
	return ""
}

// String returns the suit symbol (jokers: "王").
func (s Suit) String() string {
	if s == Joker {
		return "王"
	}
	return s.Symbol()
}

// IsRed reports whether the suit is hearts or diamonds. Jokers are not red
// suits (use Card.IsRedJoker for the joker's colour).
func (s Suit) IsRed() bool { return s == Heart || s == Diamond }

// Card is a physical card with a unique id in 0..107. Each of the two decks
// holds ids d*54+0 .. d*54+53: 52 regular cards ordered by rank then suit,
// then the small joker (52) and the big joker (53).
type Card uint8

// Deck sizes.
const (
	DeckSize = 54
	NumCards = 2 * DeckSize
)

// NewCard returns the card of the given rank and suit from deck copy 0 or 1.
// The suit is ignored for jokers.
func NewCard(r Rank, s Suit, copy int) Card {
	base := (copy & 1) * DeckSize
	switch r {
	case RSmallJoker:
		return Card(base + 52)
	case RBigJoker:
		return Card(base + 53)
	}
	return Card(base + int(r-R3)*4 + int(s&3))
}

// Rank returns the rank of the card.
func (c Card) Rank() Rank {
	i := int(c) % DeckSize
	switch i {
	case 52:
		return RSmallJoker
	case 53:
		return RBigJoker
	}
	return R3 + Rank(i/4)
}

// Suit returns the suit of the card (Joker for jokers).
func (c Card) Suit() Suit {
	i := int(c) % DeckSize
	if i >= 52 {
		return Joker
	}
	return Suit(i % 4)
}

// IsJoker reports whether the card is a small or big joker.
func (c Card) IsJoker() bool { return c.Rank() >= RSmallJoker }

// IsRedJoker reports whether the card is the big joker, which is drawn red
// (the small joker is drawn black).
func (c Card) IsRedJoker() bool { return c.Rank() == RBigJoker }

// IsRed reports whether the card is drawn red: hearts, diamonds and the big
// joker.
func (c Card) IsRed() bool { return c.Suit().IsRed() || c.IsRedJoker() }

// Valid reports whether the id is in range.
func (c Card) Valid() bool { return int(c) < NumCards }

// String returns e.g. "♠A", "♥10", "小王", "大王".
func (c Card) String() string {
	if c.IsJoker() {
		return c.Rank().String()
	}
	return c.Suit().Symbol() + c.Rank().String()
}

// NewDeck returns all 108 cards (two decks) in id order.
func NewDeck() []Card {
	d := make([]Card, NumCards)
	for i := range d {
		d[i] = Card(i)
	}
	return d
}

func cardOrder(a, b Card) int {
	ra, rb := a.Rank(), b.Rank()
	if ra != rb {
		return int(rb) - int(ra)
	}
	sa, sb := a.Suit(), b.Suit()
	if sa != sb {
		return int(sa) - int(sb)
	}
	return int(a) - int(b)
}

// SortHand sorts cards in place in display order: highest rank first
// (大王, 小王, 2, A, ..., 3); equal ranks are grouped and ordered by suit
// (♠ ♥ ♣ ♦), then by id.
func SortHand(cards []Card) { slices.SortFunc(cards, cardOrder) }

// CardsString joins the card names with spaces.
func CardsString(cards []Card) string {
	var b strings.Builder
	for i, c := range cards {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(c.String())
	}
	return b.String()
}

// RankCounts is a rank histogram indexed by Rank (indices 0..2 are unused).
type RankCounts [MaxRank + 1]uint8

// CountRanks returns the rank histogram of the cards.
func CountRanks(cards []Card) RankCounts {
	var c RankCounts
	for _, x := range cards {
		c[x.Rank()]++
	}
	return c
}

// Total returns the number of cards counted.
func (c *RankCounts) Total() int {
	n := 0
	for r := MinRank; r <= MaxRank; r++ {
		n += int(c[r])
	}
	return n
}

// Contains reports whether c holds at least the cards of o.
func (c *RankCounts) Contains(o *RankCounts) bool {
	for r := MinRank; r <= MaxRank; r++ {
		if c[r] < o[r] {
			return false
		}
	}
	return true
}

// Add adds o to c.
func (c *RankCounts) Add(o *RankCounts) {
	for r := MinRank; r <= MaxRank; r++ {
		c[r] += o[r]
	}
}

// Sub subtracts o from c. The caller must ensure c contains o.
func (c *RankCounts) Sub(o *RankCounts) {
	for r := MinRank; r <= MaxRank; r++ {
		c[r] -= o[r]
	}
}

// String lists the ranks from high to low, e.g. "大王 2 2 9".
func (c RankCounts) String() string {
	var b strings.Builder
	for r := MaxRank; r >= MinRank; r-- {
		for i := 0; i < int(c[r]); i++ {
			if b.Len() > 0 {
				b.WriteByte(' ')
			}
			b.WriteString(r.String())
		}
	}
	return b.String()
}

// CardsFromSpec builds distinct cards from a compact rank spec, mainly for
// tests and debugging. Each character is a rank: '3'..'9', 'T' or '0' for 10,
// 'J', 'Q', 'K', 'A', '2', 'x' for the small joker and 'X' for the big joker.
// Spaces and commas are ignored. Copies of a rank get different suits/decks.
func CardsFromSpec(spec string) ([]Card, error) {
	var used RankCounts
	var out []Card
	for _, ch := range spec {
		var r Rank
		switch {
		case ch == ' ' || ch == ',':
			continue
		case ch >= '3' && ch <= '9':
			r = R3 + Rank(ch-'3')
		case ch == 'T' || ch == 't' || ch == '0':
			r = R10
		case ch == 'J' || ch == 'j':
			r = RJ
		case ch == 'Q' || ch == 'q':
			r = RQ
		case ch == 'K' || ch == 'k':
			r = RK
		case ch == 'A' || ch == 'a':
			r = RA
		case ch == '2':
			r = R2
		case ch == 'x':
			r = RSmallJoker
		case ch == 'X':
			r = RBigJoker
		default:
			return nil, errors.New("unknown rank character " + string(ch))
		}
		i := int(used[r])
		limit := 8
		if r >= RSmallJoker {
			limit = 2
		}
		if i >= limit {
			return nil, errors.New("too many copies of " + r.String())
		}
		used[r]++
		if r >= RSmallJoker {
			out = append(out, NewCard(r, Joker, i))
		} else {
			out = append(out, NewCard(r, Suit(i%4), i/4))
		}
	}
	return out, nil
}
