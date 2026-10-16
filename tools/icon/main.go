// Command icon draws the app's icon, a vermilion seal reading 斗 on red
// lacquer, into resources/icon.png:
//
//	go run ./tools/icon
package main

import (
	"image/png"
	"log"
	"os"

	"github.com/egoist/mygo/ui"
)

func main() {
	const size = 1024
	view := func(c *ui.Context) {
		c.Root().Background(ui.Transparent)
		// macOS icons leave a margin around a rounded square.
		ui.Box(c).Fill().Padding(100).Children(func() {
			ui.Box(c).Fill().Radius(185).Center().
				Gradient(ui.Hex("#22385a"), ui.Hex("#0d1724"), 180).
				Border(14, ui.Hex("#c9a45c")).
				Children(func() {
					ui.Box(c).Size(520, 520).Radius(36).Center().Rotate(-6).
						Background(ui.Hex("#c23b2a")).Border(18, ui.Hex("#fbf7ee").Alpha(0.85)).
						Shadow(0, 24, 60, 0, ui.RGBA(0, 0, 0, 0.5)).
						Children(func() {
							ui.Text(c, "斗").Font("Kaiti SC, STKaiti, KaiTi, serif").FontSize(400).Bold().
								TextColor(ui.Hex("#fbf7ee"))
						})
				})
		})
	}
	img := ui.Render(view, size, size, 1)
	f, err := os.Create("resources/icon.png")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		log.Fatal(err)
	}
}
