package main

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"

	"deepdizhu/ai"
	"deepdizhu/game"
)

// theme is the widgets' look on the lacquer table: the dark theme, warmed
// to the palette, with vermilion for the primary action.
var theme = func() *ui.Theme {
	t := *ui.DarkTheme()
	t.Background = lacquerDeep
	t.Surface = ui.Hex("#24364f")
	t.SurfaceHover = ui.Hex("#2c4262")
	t.SurfacePressed = ui.Hex("#1c2b40")
	t.Border = ui.Hex("#3a5070")
	t.Text = inkText
	t.TextMuted = inkMuted
	t.Accent = vermilion
	t.AccentHover = vermilionHi
	t.AccentPressed = ui.Hex("#a83223")
	t.AccentText = ivory
	t.Focus = brass
	t.Radius = 8
	t.Inverse, t.InverseText = ivory, cardBlack
	return &t
}()

// Where cards come from as they land on the table, from each seat.
var landFrom = [4]ui.Motion{
	{Y: 60, Scale: 0.9},
	{X: 80, Scale: 0.9},
	{Y: -60, Scale: 0.9},
	{X: -80, Scale: 0.9},
}

// cardMatch is the identity a card in your hand shares with the same card
// landing on the table, so that it flies there as you play it.
type cardMatch struct{ card game.Card }

func faceOf(card game.Card) face {
	r := card.Rank()
	if r == game.RSmallJoker || r == game.RBigJoker {
		return face{rank: r.String(), red: card.IsRedJoker(), joker: true}
	}
	return face{rank: r.String(), suit: card.Suit().Symbol(), red: card.Suit().IsRed()}
}

func (t *table) view(c *ui.Context) {
	c.SetTheme(theme)
	c.Root().Background(lacquerDeep)
	ui.Column(c).Fill().Children(func() {
		t.topBar(c)
		t.tableTop(c)
	})
	t.resultDialog(c)
	t.settingsDialog(c)
	t.rulesDialog(c)
	t.save()
}

// topBar runs along the top of the window, under its controls: the name of
// the game, the deal's stakes, and the settings.
func (t *table) topBar(c *ui.Context) {
	bar := c.TitleBar()
	ui.Row(c).Height(max(bar.Height, 50)).PaddingStart(bar.Left + 18).PaddingEnd(bar.Right + 18).
		Gap(14).DragWindow().Children(func() {
		ui.Text(c, "斗地主").Font(kaiFont).FontSize(22).Bold().TextColor(brass)
		ui.Text(c, "DeepDizhu 四人场").FontSize(12).TextColor(inkMuted)
		ui.Spacer(c)
		if t.g != nil {
			stat(c, "第", fmt.Sprint(t.dealNumber()), "局")
			stat(c, "底分", fmt.Sprint(baseScores[t.set.base]), "")
			stat(c, "倍数", fmt.Sprintf("×%d", t.multiplier()), "")
		}
		if ui.Button(c, "规则").Clicked() {
			t.rulesOpen = true
		}
		if ui.Button(c, "设置").Clicked() {
			t.settingsOpen = true
		}
	})
}

// stat shows a figure of the deal with what it counts; the figure rolls
// to its new value as it changes.
func stat(c *ui.Context, before, value, after string) {
	ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
		ui.Text(c, before).FontSize(12).TextColor(inkMuted)
		ui.Text(c, value).Font(serifFont).FontSize(17).Bold().TextColor(inkText).
			FontFeatures("lnum").ContentTransition(ui.ContentTransition{Numeric: true})
		if after != "" {
			ui.Text(c, after).FontSize(12).TextColor(inkMuted)
		}
	})
}

// tableTop is the lacquered top inside its brass rim, where the seats are.
func (t *table) tableTop(c *ui.Context) {
	ui.Box(c).Grow(1).Margin(0, 14, 14, 14).Radius(26).Clip().
		Background(lacquer).Border(2, brassDim).
		InsetShadow(0, 0, 40, 0, ui.RGBA(0, 0, 0, 0.45)).
		Draw(drawLamp).
		Children(func() {
			if !t.started {
				t.lobby(c)
				return
			}
			ui.Column(c).Fill().Padding(12, 18, 10, 18).Children(func() {
				t.upperRow(c)
				t.middleRow(c)
				t.lowerArea(c)
			})
		})
}

// drawLamp lights the middle of the table, as a lamp hanging over it: a
// pool of lighter lacquer whose edge fades out. The pool is the blurred
// shadow of a box too small to see.
func drawLamp(p *ui.Painter, r ui.Rect) {
	cx, cy := r.X+r.W/2, r.Y+r.H*0.45
	dot := ui.Rect{X: cx - r.W*0.2, Y: cy, W: r.W * 0.4}
	p.Shadow(dot, 0, 0, 0, r.H*0.32, r.H*0.22, lacquerLight)
}

// upperRow holds the counter of cards still out, the player across, and
// the bottom cards.
func (t *table) upperRow(c *ui.Context) {
	ui.Row(c).AlignItems(ui.Start).Children(func() {
		ui.Box(c).Width(330).Children(func() {
			if t.set.counter {
				t.cardCounter(c)
			}
		})
		ui.Column(c).Grow(1).AlignItems(ui.Center).Gap(10).Children(func() {
			t.seatPanel(c, 2, true)
			ui.Box(c).Height(tableCard.h + 26).Center().Children(func() { t.shownView(c, 2) })
		})
		ui.Row(c).Width(330).Justify(ui.End).Children(func() { t.bottomCards(c) })
	})
}

// middleRow holds the players on the left and the right, and what they
// played.
func (t *table) middleRow(c *ui.Context) {
	ui.Row(c).Grow(1).Gap(16).Children(func() {
		t.seatPanel(c, 3, false)
		ui.Row(c).Grow(1).Basis(0).Justify(ui.Start).Children(func() { t.shownView(c, 3) })
		ui.Column(c).Width(220).AlignItems(ui.Center).Children(func() { t.centerNotice(c) })
		ui.Row(c).Grow(1).Basis(0).Justify(ui.End).Children(func() { t.shownView(c, 1) })
		t.seatPanel(c, 1, false)
	})
}

// lowerArea is yours: what you played, your actions and your hand.
func (t *table) lowerArea(c *ui.Context) {
	ui.Column(c).AlignItems(ui.Center).Gap(8).Children(func() {
		ui.Box(c).Height(tableCard.h + 26).Center().Children(func() { t.shownView(c, human) })
		ui.Row(c).FillWidth().Height(44).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Width(260).Children(func() { t.myPanel(c) })
			ui.Row(c).Grow(1).Justify(ui.Center).Gap(10).Children(func() { t.actions(c) })
			ui.Box(c).Width(260)
		})
		t.hand(c)
	})
}

// centerNotice says what the table waits for, or what went wrong.
func (t *table) centerNotice(c *ui.Context) {
	g := t.g
	var line string
	switch {
	case t.notice != "":
		line = t.notice
	case g.Phase == game.PhaseBidding && g.HighestBid > 0:
		line = fmt.Sprintf("当前叫分 %d 分", g.HighestBid)
	case g.Phase == game.PhaseBidding:
		line = "叫分抢地主"
	case g.Phase == game.PhaseFinished:
		line = "本局结束"
	}
	if line != "" {
		ui.Text(c.Key(line), line).Font(kaiFont).FontSize(18).TextColor(inkText).TextAlign(ui.Center).
			Transition(ui.ElementTransition{Enter: &ui.Motion{Y: 6}, Exit: &ui.Motion{}})
	}
}

// seatPanel shows a computer player: who they are, how well they play,
// their role and how many cards they hold.
func (t *table) seatPanel(c *ui.Context, seat int, across bool) {
	g := t.g
	var panel ui.Element
	if across {
		panel = ui.Row(c.Key(seat)).Gap(14).AlignItems(ui.Center)
	} else {
		panel = ui.Column(c.Key(seat)).Width(132).AlignItems(ui.Center).AlignSelf(ui.Center).Gap(6)
	}
	panel.Children(func() {
		ui.Column(c).AlignItems(ui.Center).Gap(6).Children(func() {
			avatar(c, seatChars[seat], g.Landlord == seat, t.thinking == seat)
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Text(c, seatNames[seat]).Font(kaiFont).FontSize(17).TextColor(inkText)
				levelBadge(c, t.set.levels[seat])
			})
		})
		ui.Column(c).AlignItems(ui.Center).Gap(6).Children(func() {
			t.roleSeal(c, seat)
			cardsLeft(c, len(g.Hands[seat]))
			score(c, t.scores[seat])
		})
	})
}

// myPanel shows your role and score, beside your actions.
func (t *table) myPanel(c *ui.Context) {
	ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
		ui.Text(c, "我").Font(kaiFont).FontSize(20).TextColor(inkText)
		t.roleSeal(c, human)
		score(c, t.scores[human])
	})
}

// avatar is a lacquer disc with a seat's character, ringed in brass when
// the seat is the landlord's, and pulsing while its player thinks.
func avatar(c *ui.Context, char string, landlord, thinking bool) {
	ring := brassDim
	if landlord {
		ring = brass
	}
	disc := ui.Box(c).Size(58, 58).Radius(29).Center().Background(lacquerDeep).Border(2, ring)
	if thinking {
		// It pulses, unless the desktop asks for less motion.
		pulse := float32(0.6)
		if !c.Preferences().ReduceMotion {
			pulse = disc.Loop("think", 1200*time.Millisecond, ui.Bounce(ui.EaseInOut))
		}
		disc.Shadow(0, 0, 6+10*pulse, 1, brass.Alpha(0.25+0.35*pulse))
	}
	disc.Children(func() {
		ui.Text(c, char).Font(kaiFont).FontSize(24).TextColor(inkText)
	})
}

// levelBadge names how well a computer player plays.
func levelBadge(c *ui.Context, level ai.Level) {
	color := map[ai.Level]ui.Color{ai.Easy: jade, ai.Normal: brass, ai.Hard: vermilionHi}[level]
	ui.Box(c).Padding(1, 6).Radius(4).Border(1, color.Alpha(0.7)).Children(func() {
		ui.Text(c, level.String()).FontSize(11).TextColor(color)
	})
}

// roleSeal stamps a seat's role in the deal: a vermilion seal for the
// landlord, a jade one for the farmers.
func (t *table) roleSeal(c *ui.Context, seat int) {
	role := t.role(seat)
	if role == "" {
		ui.Box(c).Height(24)
		return
	}
	color := jade
	if role == "地主" {
		color = vermilion
	}
	ui.Box(c.Key(role)).Padding(1, 6).Radius(3).Background(color).Rotate(-4).
		Border(1, ivory.Alpha(0.35)).
		Transition(ui.ElementTransition{Animation: &ui.Snappy, Enter: &ui.Motion{Scale: 1.9}}).
		Children(func() {
			ui.Text(c, role).Font(kaiFont).FontSize(16).Bold().TextColor(ivory)
		})
}

// cardsLeft shows how many cards a player holds, warning when one is about
// to go out.
func cardsLeft(c *ui.Context, n int) {
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		cardBack(c, miniCard)
		color := inkText
		if n > 0 && n <= 2 {
			color = vermilionHi
		}
		ui.Text(c, fmt.Sprint(n)).Font(serifFont).FontSize(24).Bold().TextColor(color).
			FontFeatures("lnum", "tnum").ContentTransition(ui.ContentTransition{Numeric: true, Down: true})
	})
	if n > 0 && n <= 2 {
		ui.Text(c, fmt.Sprintf("只剩 %d 张", n)).FontSize(12).TextColor(vermilionHi)
	}
}

func score(c *ui.Context, n int) {
	ui.RichText(c,
		ui.Span{Text: "积分 ", Size: 12, Color: inkMuted},
		ui.Span{Text: signed(n), Font: serifFont, Size: 15, Weight: 700, Color: inkText, Features: "lnum"},
	).NoWrap()
}

func signed(n int) string {
	if n > 0 {
		return fmt.Sprintf("+%d", n)
	}
	if n < 0 {
		return fmt.Sprintf("−%d", -n)
	}
	return "0"
}

// shownView shows what seat did last in this bidding or trick: its bid, a
// pass, or the cards it played.
func (t *table) shownView(c *ui.Context, seat int) {
	s := t.shown[seat]
	switch s.kind {
	case showBid:
		word := "不叫"
		if s.bid > 0 {
			word = fmt.Sprintf("叫 %d 分", s.bid)
		}
		bubble(c, seat, word, s.bid > 0)
	case showPass:
		bubble(c, seat, "不出", false)
	case showPlay:
		t.playedCards(c, seat, s.combo)
	}
}

// bubble is a word said at the table by seat. It paints its word rather
// than holding a text, so that only the buttons read 不出 or 不叫 to tests;
// screen readers hear who said it.
func bubble(c *ui.Context, seat int, word string, strong bool) {
	color := inkMuted
	if strong {
		color = brass
	}
	span := ui.Span{Text: word, Font: kaiFont, Size: 20, Color: color}
	w, h := c.MeasureText(0, span)
	enter := ui.Motion{Scale: 0.8}
	ui.Box(c.Key(word)).Size(w+32, h+12).Radius((h+12)/2).
		Background(lacquerDeep.Alpha(0.7)).Border(1, color.Alpha(0.5)).
		Label(seatNames[seat] + word).
		Transition(ui.ElementTransition{Animation: &ui.Snappy, Enter: &enter}).
		Draw(func(p *ui.Painter, r ui.Rect) {
			p.RichText(r.X+16, r.Y+6, 0, span)
		})
}

// playedCards fans out the cards of a play, with the name of what they
// make under them.
func (t *table) playedCards(c *ui.Context, seat int, combo game.Combo) {
	n := len(combo.Cards)
	step := min(float32(24), (440-tableCard.w)/float32(max(n-1, 1)))
	width := tableCard.w + step*float32(n-1)
	from := landFrom[seat]
	ui.Column(c.Key(fmt.Sprint(combo.Cards))).AlignItems(ui.Center).Gap(6).Children(func() {
		ui.Box(c).Size(width, tableCard.h).Children(func() {
			for i, card := range combo.Cards {
				tr := ui.ElementTransition{Animation: &ui.Snappy, Enter: &from}
				if seat == human {
					tr = ui.ElementTransition{Animation: &ui.Snappy, Match: cardMatch{card}}
				}
				cardFace(c.Key(int(card)), faceOf(card), tableCard).
					Absolute().Left(float32(i) * step).Label(card.String()).Transition(tr)
			}
		})
		label := combo.Type.String()
		if combo.IsBomb() {
			label = combo.Describe()
		}
		color := brass
		if combo.IsBomb() {
			color = vermilionHi
		}
		ui.Text(c, label).Font(kaiFont).FontSize(14).TextColor(color)
	})
}

// actions are your buttons when it is your turn: the bids while bidding,
// and pass, hint and play while playing.
func (t *table) actions(c *ui.Context) {
	g := t.g
	if !t.myTurn() {
		if t.thinking >= 0 {
			ui.Text(c, seatNames[t.thinking]+"思考中…").FontSize(14).TextColor(inkMuted)
		}
		return
	}
	if g.Phase == game.PhaseBidding {
		if ui.Button(c, "不叫").Width(84).Clicked() {
			t.bid(human, 0)
		}
		for points := 1; points <= 3; points++ {
			label := fmt.Sprintf("%d 分", points)
			button := ui.Button
			if points == 3 {
				button = ui.PrimaryButton
			}
			if button(c, label).Width(84).Disabled(points <= g.HighestBid).Clicked() {
				t.bid(human, points)
			}
		}
		return
	}
	combo, ok := t.selection()
	if ui.Button(c, "不出").Width(92).Disabled(!g.CanPass(human)).Clicked() {
		t.pass()
	}
	if ui.Button(c, "提示").Width(92).Clicked() {
		t.hint()
	}
	if ui.PrimaryButton(c, "出牌").Width(108).Disabled(!ok).Clicked() {
		t.playSelected()
	}
	if ok {
		ui.Text(c, combo.Describe()).Font(kaiFont).FontSize(15).TextColor(brass).Width(120)
	} else {
		ui.Box(c).Width(120)
	}
}

// hand fans out your cards. A click selects or unselects a card, raising
// it; dragging across cards does so to each; a right click unselects them
// all.
func (t *table) hand(c *ui.Context) {
	cards := t.g.Hands[human]
	n := len(cards)
	w, _ := c.Size()
	avail := w - 120
	step := float32(44)
	if n > 1 {
		step = min(step, (avail-handCard.w)/float32(n-1))
	}
	const lift = 22
	width := handCard.w + step*float32(max(n-1, 0))
	// at returns the card under x in the fan: the one on top where cards
	// overlap, or -1.
	at := func(x float32) int {
		if n == 0 || x < 0 || x >= width {
			return -1
		}
		return min(int(x/step), n-1)
	}
	fan := ui.Box(c).Size(width, handCard.h+lift).Margin(0, 0, 6, 0).
		Cursor(ui.CursorPointer).Label("手牌").
		HandleInput(func(ev ui.InputEvent) bool {
			switch ev.Kind {
			case ui.InputPointerDown:
				if ev.Button == 1 {
					t.clearSelection()
					return true
				}
				i := at(ev.X)
				if ev.Button != 0 || i < 0 {
					return false
				}
				t.sweep = sweep{on: true, from: i, to: i}
				return true
			case ui.InputPointerMove:
				if !t.sweep.on {
					return false
				}
				if i := at(ev.X); i >= 0 {
					t.sweep.to = i
				}
				return true
			case ui.InputPointerUp:
				if !t.sweep.on {
					return false
				}
				lo, hi := t.sweep.span()
				t.sweep = sweep{}
				for i := lo; i <= hi && i < n; i++ {
					t.toggle(cards[i])
				}
				return true
			}
			return false
		})
	fan.Children(func() {
		lo, hi := t.sweep.span()
		for i, card := range cards {
			top := float32(lift)
			if t.selected[card] {
				top = 0
			}
			el := cardFace(c.Key(int(card)), faceOf(card), handCard).PassThrough().
				Absolute().Left(float32(i) * step).Top(top).Label(card.String()).
				Transition(ui.ElementTransition{Animation: &ui.Snappy, Position: true, Match: cardMatch{card}})
			if t.fresh[card] {
				el.Border(2, brass)
			}
			if t.sweep.on && i >= lo && i <= hi {
				el.DrawOver(func(p *ui.Painter, r ui.Rect) {
					p.Fill(r, brass.Alpha(0.28), handCard.w*0.09)
				})
			}
		}
	})
}

// sweep is a drag across the cards of your hand, from the card it started
// on to the one under the pointer.
type sweep struct {
	on       bool
	from, to int
}

func (s sweep) span() (lo, hi int) {
	return min(s.from, s.to), max(s.from, s.to)
}

// cardCounter counts the cards still out, by rank, from the highest.
func (t *table) cardCounter(c *ui.Context) {
	if t.g.Phase == game.PhaseBidding {
		return
	}
	counts := t.unseen()
	ui.Row(c).Gap(2).Padding(4).Radius(8).Background(lacquerDeep.Alpha(0.6)).Children(func() {
		for r := game.RBigJoker; r >= game.R3; r-- {
			label := r.String()
			switch r {
			case game.RBigJoker:
				label = "大"
			case game.RSmallJoker:
				label = "小"
			}
			n := counts[r]
			color := inkText
			if n == 0 {
				color = inkMuted.Alpha(0.5)
			}
			ui.Column(c).Width(19).AlignItems(ui.Center).Children(func() {
				ui.Text(c, label).FontSize(11).TextColor(inkMuted)
				ui.Text(c, fmt.Sprint(n)).Font(serifFont).FontSize(14).Bold().TextColor(color).FontFeatures("lnum")
			})
		}
	})
}

// bottomCards shows the eight bottom cards: face down while the players
// bid, then face up.
func (t *table) bottomCards(c *ui.Context) {
	g := t.g
	ui.Column(c).AlignItems(ui.End).Gap(4).Children(func() {
		ui.Row(c).Gap(3).Children(func() {
			for i, card := range g.Bottom {
				if g.Landlord < 0 {
					cardBack(c.Key(i), miniCard)
				} else {
					cardFace(c.Key(i), faceOf(card), miniCard).Label(card.String())
				}
			}
		})
		ui.Text(c, "底牌").FontSize(11).TextColor(inkMuted)
	})
}
