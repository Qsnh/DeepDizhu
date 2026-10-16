package main

import (
	"github.com/egoist/mygo/ui"
)

// face is what a card shows: its rank and suit as printed, and its ink.
type face struct {
	rank  string // "3"…"10", "J", "Q", "K", "A", "2", or "大王"/"小王"
	suit  string // "♠", "♥", "♣", "♦", or "" on a joker
	red   bool
	joker bool
}

// cardSize is the size a card is drawn at, in DIPs; everything on the card
// scales with its width.
type cardSize struct{ w, h float32 }

var (
	handCard  = cardSize{78, 108}
	tableCard = cardSize{56, 78}
	miniCard  = cardSize{34, 47}
)

func (f face) ink() ui.Color {
	if f.red {
		return cardRed
	}
	return cardBlack
}

// drawFace paints a card's face into r: the index in the top left corner,
// which shows while cards overlap, and a larger mark in the lower right.
func drawFace(p *ui.Painter, r ui.Rect, f face) {
	s := r.W / handCard.w
	ink := f.ink()
	if f.joker {
		// The corner reads 大王 or 小王 down the card, in brush strokes.
		size := 17 * s
		for i, ch := range []rune(f.rank) {
			p.RichText(r.X+6*s, r.Y+5*s+float32(i)*size*1.05, 0,
				ui.Span{Text: string(ch), Font: kaiFont, Size: size, Weight: 700, Color: ink})
		}
		big := ui.Span{Text: "王", Font: kaiFont, Size: 44 * s, Weight: 700, Color: ink.Alpha(0.9)}
		w, h := p.MeasureText(0, big)
		p.RichText(r.X+r.W-w-6*s, r.Y+r.H-h-4*s, 0, big)
		return
	}
	rank := ui.Span{Text: f.rank, Font: serifFont, Size: 21 * s, Weight: 700, Color: ink, Features: "lnum"}
	if f.rank == "10" {
		rank.LetterSpacing = -1.5 * s
	}
	rw, rh := p.MeasureText(0, rank)
	p.RichText(r.X+5*s, r.Y+3*s, 0, rank)
	suit := ui.Span{Text: f.suit, Size: 15 * s, Color: ink}
	sw, _ := p.MeasureText(0, suit)
	p.RichText(r.X+5*s+(rw-sw)/2, r.Y+3*s+rh-2*s, 0, suit)

	// The mark: the suit, large, or the letter of a court card over it.
	switch f.rank {
	case "J", "Q", "K":
		letter := ui.Span{Text: f.rank, Font: serifFont, Size: 40 * s, Weight: 700, Italic: true, Color: ink}
		w, h := p.MeasureText(0, letter)
		p.RichText(r.X+r.W-w-8*s, r.Y+r.H-h-6*s, 0, letter)
	default:
		mark := ui.Span{Text: f.suit, Size: 38 * s, Color: ink}
		w, h := p.MeasureText(0, mark)
		p.RichText(r.X+r.W-w-7*s, r.Y+r.H-h-4*s, 0, mark)
	}
}

// cardFace builds a card face up. The caller places and labels it.
func cardFace(c *ui.Context, f face, sz cardSize) ui.Element {
	return ui.Box(c).Size(sz.w, sz.h).Radius(sz.w*0.09).
		Background(ivory).Border(1, ivoryEdge).
		Shadow(0, 1, 3, 0, ui.RGBA(0, 0, 0, 0.45)).
		Draw(func(p *ui.Painter, r ui.Rect) { drawFace(p, r, f) })
}

// cardBack builds a card face down: red lacquer under a brass lattice.
func cardBack(c *ui.Context, sz cardSize) ui.Element {
	return ui.Box(c).Size(sz.w, sz.h).Radius(sz.w*0.09).Padding(sz.w*0.08).
		Background(ivory).Border(1, ivoryEdge).
		Shadow(0, 1, 3, 0, ui.RGBA(0, 0, 0, 0.45)).
		Children(func() {
			ui.Box(c).Fill().Radius(sz.w*0.05).Background(backRed).Border(1, brassDim).
				Draw(func(p *ui.Painter, r ui.Rect) { drawLattice(p, r, sz.w*0.14, sz.w*0.05) })
		})
}

// drawLattice crosses brass lines both ways over r, step DIPs apart.
func drawLattice(p *ui.Painter, r ui.Rect, step, radius float32) {
	line := brass.Alpha(0.3)
	var lattice ui.Path
	for x := r.X - r.H; x < r.X+r.W; x += step {
		lattice.MoveTo(x, r.Y).LineTo(x+r.H, r.Y+r.H)
		lattice.MoveTo(x, r.Y+r.H).LineTo(x+r.H, r.Y)
	}
	p.Clip(r, radius, func() { p.StrokePath(&lattice, 1, line) })
}
