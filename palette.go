package main

import "github.com/egoist/mygo/ui"

// The table is lacquerware: a deep indigo top inside a brass rim, with
// ivory cards, and the landlord marked by a vermilion seal. The palette is
// fixed rather than following the system's light or dark appearance, as a
// card table looks the same whatever the desktop.
var (
	lacquer      = ui.Hex("#16263a") // the table top
	lacquerLight = ui.Hex("#22385a") // where the lamp falls, in the middle
	lacquerDeep  = ui.Hex("#0d1724") // the edges, and panels over the table
	brass        = ui.Hex("#c9a45c") // the rim, chosen states and the bid
	brassDim     = ui.Hex("#8a7345")
	vermilion    = ui.Hex("#c23b2a") // the landlord's seal, and actions
	vermilionHi  = ui.Hex("#d9503d")
	jade         = ui.Hex("#4f9a80") // the farmers' seal
	ivory        = ui.Hex("#fbf7ee") // card paper
	ivoryEdge    = ui.Hex("#d8cdb6")
	cardRed      = ui.Hex("#c0262d")
	cardBlack    = ui.Hex("#1f1f26")
	inkText      = ui.Hex("#ece5d6") // text on the table
	inkMuted     = ui.Hex("#93a2b8")
	backRed      = ui.Hex("#7d2219") // the backs of cards
)

// Typefaces: ranks in a book serif, as printed cards have them; seals,
// jokers and the title in a brush-written kai script.
const (
	serifFont = "Palatino, Palatino Linotype, Book Antiqua, serif"
	kaiFont   = "Kaiti SC, STKaiti, KaiTi, BiauKai, serif"
)
