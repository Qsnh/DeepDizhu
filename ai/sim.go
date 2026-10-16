package ai

import "deepdizhu/game"

// sim is a fast rank-count model of a deal in the playing phase, used for
// Monte Carlo rollouts. Every seat is played by the heuristic policy using
// only its own hand and public information.
type sim struct {
	hands    [4]counts
	n        [4]int
	remain   counts // cards still in some hand
	landlord int
	turn     int
	lastSeat int // -1 when the seat to act leads
	last     game.Move
	bombs    int // bombs played inside the simulation
	plays    [4]int
	dcs      [4]decomp
	dcOK     [4]bool
}

// setHands installs the hands and resets the derived state.
func (s *sim) setHands(h *[4]counts) {
	s.hands = *h
	s.remain = counts{}
	for i := 0; i < 4; i++ {
		s.n[i] = s.hands[i].Total()
		s.remain.Add(&s.hands[i])
		s.dcOK[i] = false
	}
}

func (s *sim) ctxFor(seat int, cx *ctx) {
	cx.me = seat
	cx.landlord = s.landlord
	cx.hand = s.hands[seat]
	for r := rLo; r <= rBJ; r++ {
		cx.outside[r] = s.remain[r] - s.hands[seat][r]
	}
	cx.n = s.n
	cx.lastSeat = s.lastSeat
	cx.last = s.last
}

// apply plays m (ok) or passes (!ok) for the seat to act. It returns true
// when that seat has emptied its hand.
func (s *sim) apply(m *game.Move, ok bool) bool {
	seat := s.turn
	if !ok {
		next := (seat + 1) & 3
		if next == s.lastSeat {
			s.lastSeat = -1
		}
		s.turn = next
		return false
	}
	if debugChecks {
		if !s.hands[seat].Contains(&m.Use) {
			panic("sim: move not in hand")
		}
		if s.lastSeat >= 0 && !m.Beats(s.last) {
			panic("sim: move does not beat " + s.last.Describe())
		}
		if s.lastSeat < 0 {
			if om, ok := game.ClassifyLead(&m.Use); !ok || om.Type != m.Type {
				panic("sim: bad lead")
			}
		}
	}
	h := &s.hands[seat]
	k := 0
	for r := rLo; r <= rBJ; r++ {
		u := m.Use[r]
		if u != 0 {
			h[r] -= u
			s.remain[r] -= u
			k += int(u)
		}
	}
	s.n[seat] -= k
	s.plays[seat]++
	if m.IsBomb() {
		s.bombs++
	}
	s.dcOK[seat] = false
	if s.n[seat] == 0 {
		return true
	}
	s.last = *m
	s.lastSeat = seat
	s.turn = (seat + 1) & 3
	return false
}

// run plays the deal out with p for every seat and returns the winning seat
// (-1 if the step cap was hit, which should not happen).
func (s *sim) run(p *policy) int {
	ps := [4]*policy{p, p, p, p}
	return s.runSeats(&ps)
}

// runSeats plays the deal out with one policy per seat.
func (s *sim) runSeats(ps *[4]*policy) int {
	var cx ctx
	for step := 0; step < 2000; step++ {
		seat := s.turn
		p := ps[seat]
		if !s.dcOK[seat] {
			p.dz.decompose(&s.hands[seat], p.wide, p.limit, &s.dcs[seat])
			s.dcOK[seat] = true
		}
		s.ctxFor(seat, &cx)
		m, ok := p.decide(&cx, &s.dcs[seat])
		if s.apply(&m, ok) {
			return seat
		}
	}
	return -1
}
