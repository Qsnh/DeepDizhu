package ai

// handStrength scores a 25-card hand for bidding: the decomposition value
// (number of "hands" needed, high cards, bombs) plus extra credit for the
// cards that win tricks back: jokers, 2s, aces and bombs.
func handStrength(h *counts) int {
	var dz decomposer
	var dc decomp
	dz.decompose(h, true, fullLimit, &dc)
	s := dc.value
	s += 3*int(h[rBJ]) + 2*int(h[rSJ]) + int(h[r2]) + int(h[rA])/2
	for r := rLo; r <= r2; r++ {
		if h[r] >= 4 {
			s += 2 * (int(h[r]) - 3)
		}
	}
	return s
}

// Strength thresholds for bidding 1, 2 and 3 points. In this 4-player
// variant the landlord (33 cards, first lead) is favoured, so these are set
// by hand-strength percentile rather than break-even win rate: about 60% of
// random hands bid at least 1, a third bid 2 and the top 15% bid 3
// (measured with TestTuneBidPercentiles).
var bidThresholds = [3]int{-35, -23, -13}

// bidFromStrength maps a strength to a bid above highest (0 = pass).
func bidFromStrength(s, highest int) int {
	want := 0
	for i, t := range bidThresholds {
		if s >= t {
			want = i + 1
		}
	}
	if want > highest {
		return want
	}
	return 0
}

// Win-rate thresholds for bidding 1, 2 and 3 points, where the rate is
// the simulated landlord win rate under heuristic play (same percentiles as
// bidThresholds; also from TestTuneBidPercentiles).
var bidWinRates = [3]float64{0.62, 0.68, 0.74}

// bidFromWinRate maps an estimated landlord win rate to a bid above
// highest (0 = pass).
func bidFromWinRate(p float64, highest int) int {
	want := 0
	for i, t := range bidWinRates {
		if p >= t {
			want = i + 1
		}
	}
	if want > highest {
		return want
	}
	return 0
}
