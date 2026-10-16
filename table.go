package main

import (
	"math/rand/v2"
	"slices"
	"time"

	"deepdizhu/ai"
	"deepdizhu/game"
)

// The seats go round the table in the order of play: you at the bottom,
// then the player on your right, the one across, and the one on your left.
const human = 0

var (
	seatNames = [4]string{"我", "下家", "对家", "上家"}
	seatChars = [4]string{"我", "下", "对", "上"} // on their avatars
)

// showKind is what a seat shows on the table in front of it.
type showKind int

const (
	showNone showKind = iota
	showBid
	showPass
	showPlay
)

// seatShow is the last thing a seat did in the current bidding or trick.
type seatShow struct {
	kind  showKind
	bid   int
	combo game.Combo
}

// Paces of the computer players: how long each takes to act at least, so
// that you can follow what they do.
var paces = []struct {
	label string
	delay time.Duration
}{
	{"慢", 1300 * time.Millisecond},
	{"适中", 800 * time.Millisecond},
	{"快", 350 * time.Millisecond},
}

var baseScores = []int{1, 2, 5}

// caps are the multipliers a deal may be capped at (封顶); 0 is no cap.
var caps = []int{0, 32, 64, 128}

const defaultCap = 2 // 64

// settings are what the player chooses in the settings.
type settings struct {
	levels  [4]ai.Level // of seats 1 to 3
	pace    int         // index in paces
	base    int         // index in baseScores
	cap     int         // index in caps
	counter bool        // shows the cards still out
}

// runner runs the computer players' thinking off the UI thread: work runs
// on another goroutine, and the function it returns runs on the UI thread
// no sooner than delay after it started.
type runner interface {
	Go(delay time.Duration, work func() func())
}

// table is the state of the game the window shows: the deal in play, the
// players, the scores and what the player has selected.
type table struct {
	run  runner
	rng  *rand.Rand
	seed uint64
	set  settings

	started     bool
	g           *game.Game
	players     [4]ai.Player
	scores      [4]int
	dealsDone   int // deals played to the end, over every launch
	firstBidder int
	// gen counts the deals, so that a computer player's move arriving for
	// a deal that is over is dropped.
	gen      int
	thinking int // the seat a computer player is thinking for, or -1
	shown    [4]seatShow

	selected map[game.Card]bool
	sweep    sweep
	fresh    map[game.Card]bool // the bottom cards, just taken by you
	hints    []game.Combo
	hintAt   int
	notice   string // what went wrong with your last action, or news

	resultOpen   bool
	settingsOpen bool
	rulesOpen    bool

	// persist saves what the game keeps between launches, when it changed
	// from lastSaved; nil keeps nothing, as in tests.
	persist   func(saved)
	lastSaved saved
}

func newTable(run runner, seed uint64) *table {
	t := &table{
		run:      run,
		seed:     seed,
		rng:      rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15)),
		thinking: -1,
		selected: map[game.Card]bool{},
		set: settings{
			levels:  [4]ai.Level{ai.Normal, ai.Normal, ai.Normal, ai.Normal},
			pace:    1,
			cap:     defaultCap,
			counter: true,
		},
	}
	for seat := 1; seat < 4; seat++ {
		t.setLevel(seat, t.set.levels[seat])
	}
	t.firstBidder = t.rng.IntN(4)
	return t
}

// setLevel makes the computer player at seat play at level, from its next
// move on.
func (t *table) setLevel(seat int, level ai.Level) {
	t.set.levels[seat] = level
	t.players[seat] = ai.New(level, t.rng.Uint64())
}

func (t *table) delay() time.Duration { return paces[t.set.pace].delay }

// start begins the first deal.
func (t *table) start() {
	t.started = true
	t.newDeal()
}

// newDeal shuffles and deals, and starts the bidding.
func (t *table) newDeal() {
	t.gen++
	t.g = game.NewGame(t.rng, t.firstBidder, game.Config{
		BaseScore:     baseScores[t.set.base],
		MaxMultiplier: caps[t.set.cap],
	})
	t.firstBidder = (t.firstBidder + 1) % 4
	t.shown = [4]seatShow{}
	t.thinking = -1
	t.resultOpen = false
	t.notice = ""
	t.fresh = nil
	t.clearSelection()
	t.advance()
}

func (t *table) clearSelection() {
	clear(t.selected)
	t.sweep = sweep{}
	t.hints, t.hintAt = nil, 0
}

// advance moves the game on from where it is: a computer player's turn
// starts its thinking, a void deal is dealt again, and a deal won ends.
func (t *table) advance() {
	g := t.g
	switch g.Phase {
	case game.PhaseRedeal:
		t.notice = "无人叫地主，重新发牌"
		gen := t.gen
		t.run.Go(1500*time.Millisecond, func() func() {
			return func() {
				if t.gen == gen {
					t.newDeal()
				}
			}
		})
		return
	case game.PhaseFinished:
		t.finish()
		return
	}
	seat := g.Turn
	if seat == human || t.thinking >= 0 {
		return
	}
	t.thinking = seat
	gen := t.gen
	view := g.View(seat)
	player := t.players[seat]
	bidding := g.Phase == game.PhaseBidding
	t.run.Go(t.delay(), func() func() {
		var bid int
		var cards []game.Card
		if bidding {
			bid = player.Bid(view)
		} else {
			cards = player.Play(view)
		}
		return func() {
			if t.gen != gen {
				return
			}
			t.thinking = -1
			if bidding {
				if t.bid(seat, bid) != nil {
					t.bid(seat, 0)
				}
			} else if t.play(seat, cards) != nil {
				t.playFallback(seat)
			}
		}
	})
}

// playFallback makes a legal move for a computer player whose move was
// not, which should never happen: it passes if it may, or plays the
// weakest move.
func (t *table) playFallback(seat int) {
	if t.g.CanPass(seat) {
		t.play(seat, nil)
		return
	}
	if moves := t.g.LegalMoves(seat); len(moves) > 0 {
		t.play(seat, moves[0].Cards)
	}
}

// bid makes seat's bid, 0 for no bid.
func (t *table) bid(seat, points int) error {
	if err := t.g.Bid(seat, points); err != nil {
		return err
	}
	t.shown[seat] = seatShow{kind: showBid, bid: points}
	if t.g.Phase == game.PhasePlaying {
		// The landlord is known: the bids leave the table, and the bottom
		// cards go to the landlord's hand.
		t.shown = [4]seatShow{}
		if t.g.Landlord == human {
			t.sweep = sweep{} // the hand changes under it
			t.fresh = map[game.Card]bool{}
			for _, card := range t.g.Bottom {
				t.fresh[card] = true
			}
		}
	}
	t.notice = ""
	t.advance()
	return nil
}

// play plays cards from seat's hand, none to pass.
func (t *table) play(seat int, cards []game.Card) error {
	leading := t.g.Leading()
	if err := t.g.Play(seat, cards); err != nil {
		return err
	}
	if leading {
		t.shown = [4]seatShow{}
	}
	if len(cards) == 0 {
		t.shown[seat] = seatShow{kind: showPass}
	} else {
		last := t.g.History[len(t.g.History)-1]
		t.shown[seat] = seatShow{kind: showPlay, combo: last.Combo}
	}
	if seat == human {
		t.fresh = nil
		t.notice = ""
		t.sweep = sweep{}
		clear(t.selected)
	}
	t.hints, t.hintAt = nil, 0
	t.advance()
	return nil
}

// finish adds up the scores of a deal won.
func (t *table) finish() {
	for seat, delta := range t.g.Result.Scores {
		t.scores[seat] += delta
	}
	t.dealsDone++
	t.resultOpen = true
}

// dealNumber is the number of the deal on the table, counting from the
// first ever played.
func (t *table) dealNumber() int {
	if t.g != nil && t.g.Phase == game.PhaseFinished {
		return t.dealsDone
	}
	return t.dealsDone + 1
}

// save hands what the game keeps to persist, if it changed.
func (t *table) save() {
	if t.persist == nil {
		return
	}
	if s := t.snapshot(); s != t.lastSaved {
		t.lastSaved = s
		t.persist(s)
	}
}

// myTurn reports whether the game waits for you to act.
func (t *table) myTurn() bool {
	return t.g != nil && t.g.Turn == human &&
		(t.g.Phase == game.PhaseBidding || t.g.Phase == game.PhasePlaying)
}

// selectedCards returns the cards you have selected, in the order of your
// hand.
func (t *table) selectedCards() []game.Card {
	var cards []game.Card
	for _, card := range t.g.Hands[human] {
		if t.selected[card] {
			cards = append(cards, card)
		}
	}
	return cards
}

// selection returns what the cards you selected would play, and whether
// they make a play you may make now.
func (t *table) selection() (game.Combo, bool) {
	cards := t.selectedCards()
	if len(cards) == 0 || t.g.Phase != game.PhasePlaying {
		return game.Combo{}, false
	}
	if t.g.Turn == human && !t.g.Leading() && t.g.LastCombo != nil {
		return game.ParseFollow(cards, *t.g.LastCombo)
	}
	return game.ParseLead(cards)
}

func (t *table) toggle(card game.Card) {
	if t.selected[card] {
		delete(t.selected, card)
	} else {
		t.selected[card] = true
	}
	t.notice = ""
}

// playSelected plays the cards you selected.
func (t *table) playSelected() {
	cards := t.selectedCards()
	if len(cards) == 0 {
		t.notice = "请先选择要出的牌"
		return
	}
	if err := t.play(human, cards); err != nil {
		t.notice = err.Error()
	}
}

func (t *table) pass() {
	if err := t.play(human, nil); err != nil {
		t.notice = err.Error()
	}
}

// hint selects a suggested play: the first time the computer's advice,
// then, each time again, the next of the plays you could make.
func (t *table) hint() {
	if !t.myTurn() || t.g.Phase != game.PhasePlaying {
		return
	}
	if t.hints == nil {
		t.hints = t.g.LegalMoves(human)
		if t.g.Leading() {
			// Leading, put the computer's advice first.
			advice := ai.Hint(t.g.View(human))
			if combo, ok := game.ParseLead(advice); ok {
				t.hints = slices.Insert(t.hints, 0, combo)
			}
		}
		t.hintAt = 0
	}
	if len(t.hints) == 0 {
		t.notice = "没有大过上家的牌"
		clear(t.selected)
		return
	}
	combo := t.hints[t.hintAt%len(t.hints)]
	t.hintAt++
	clear(t.selected)
	for _, card := range combo.Cards {
		t.selected[card] = true
	}
	t.notice = ""
}

// unseen returns how many cards of each rank are still out: in the other
// players' hands, as far as you can tell.
func (t *table) unseen() map[game.Rank]int {
	counts := map[game.Rank]int{}
	for _, card := range t.g.View(human).Unseen() {
		counts[card.Rank()]++
	}
	return counts
}

// role returns the role of seat in the deal: 地主, 农民, or "" while the
// bidding goes on.
func (t *table) role(seat int) string {
	if t.g == nil || t.g.Landlord < 0 {
		return ""
	}
	if t.g.Landlord == seat {
		return "地主"
	}
	return "农民"
}

// multiplier returns the deal's multiplier so far: the bid, doubled for
// each bomb, up to the deal's cap.
func (t *table) multiplier() int {
	if t.g == nil {
		return 0
	}
	if t.g.Result != nil {
		return t.g.Result.Multiplier
	}
	m := max(t.g.HighestBid, 1)
	for range t.g.Bombs {
		m *= 2
	}
	if c := t.g.Config.MaxMultiplier; c > 0 {
		m = min(m, c)
	}
	return m
}
