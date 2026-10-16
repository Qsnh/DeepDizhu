package main

import (
	"fmt"

	"github.com/egoist/mygo/ui"

	"deepdizhu/ai"
	"deepdizhu/game"
)

var levelLabels = func() []string {
	var labels []string
	for _, l := range ai.Levels {
		labels = append(labels, l.String())
	}
	return labels
}()

// lobby greets you before the first deal, where you choose how well the
// three computer players play.
func (t *table) lobby(c *ui.Context) {
	ui.Column(c).Fill().Center().Gap(26).Children(func() {
		ui.Row(c).Gap(14).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "斗地主").Font(kaiFont).FontSize(64).Bold().TextColor(ivory)
			ui.Column(c).Padding(4, 6).Radius(4).Background(vermilion).Rotate(-6).Children(func() {
				ui.Text(c, "四人").Font(kaiFont).FontSize(20).Bold().TextColor(ivory)
			})
		})
		ui.Text(c, "你和三位电脑玩家同桌，两副牌一百零八张，一人当地主，三人做农民。").
			FontSize(15).TextColor(inkMuted)
		ui.Column(c).Gap(14).Padding(22, 28).Radius(14).Background(lacquerDeep.Alpha(0.75)).Border(1, brassDim.Alpha(0.6)).
			Children(func() {
				ui.Text(c, "电脑玩家的水平").Font(kaiFont).FontSize(18).TextColor(brass)
				t.levelPickers(c)
			})
		if ui.PrimaryButton(c, "开始游戏").Width(200).Height(44).FontSize(17).Clicked() {
			t.start()
		}
	})
}

// levelPickers choose the level of each computer player.
func (t *table) levelPickers(c *ui.Context) {
	for seat := 1; seat < 4; seat++ {
		ui.Row(c.Key(seat)).Gap(16).AlignItems(ui.Center).Children(func() {
			ui.Text(c, seatNames[seat]).Font(kaiFont).FontSize(17).Width(72)
			chosen := int(t.set.levels[seat])
			if ui.Segmented(c, &chosen, levelLabels...).Label(seatNames[seat] + "水平").Changed() {
				t.setLevel(seat, ai.Level(chosen))
			}
		})
	}
	ui.Text(c, levelHelp).FontSize(12).TextColor(inkMuted).MaxWidth(330)
}

const levelHelp = "简单：随手出牌，常常该压不压。普通：会拆牌组牌、让着队友、拦截快出完的对手。困难：记牌，每一步先推演上万种可能的牌局再出。"

// panel styles a dialog of the table: lacquer under a brass edge.
func panel(backdrop, p ui.Element, width float32) {
	backdrop.Background(ui.RGBA(4, 8, 14, 0.62))
	p.Width(width).Padding(24, 28).Gap(16).Radius(16).Background(lacquerDeep).Border(1, brassDim).
		Shadow(0, 18, 50, 0, ui.RGBA(0, 0, 0, 0.55))
}

// settingsDialog changes the computer players' levels, their pace, the
// stakes and the card counter.
func (t *table) settingsDialog(c *ui.Context) {
	ui.DialogBase(c, &t.settingsOpen, func(backdrop, p ui.Element) {
		panel(backdrop, p, 440)
		ui.Text(c, "设置").Font(kaiFont).FontSize(24).TextColor(brass)
		t.levelPickers(c)
		ui.Divider(c).Background(brassDim.Alpha(0.4))
		ui.Row(c).Gap(16).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "出牌速度").Width(72)
			var labels []string
			for _, p := range paces {
				labels = append(labels, p.label)
			}
			ui.Segmented(c, &t.set.pace, labels...).Label("出牌速度")
		})
		ui.Row(c).Gap(16).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "底分").Width(72)
			var labels []string
			for _, b := range baseScores {
				labels = append(labels, fmt.Sprint(b))
			}
			ui.Segmented(c, &t.set.base, labels...).Label("底分")
		})
		ui.Row(c).Gap(16).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "封顶").Width(72)
			var labels []string
			for _, m := range caps {
				if m == 0 {
					labels = append(labels, "不封顶")
				} else {
					labels = append(labels, fmt.Sprintf("%d 倍", m))
				}
			}
			ui.Segmented(c, &t.set.cap, labels...).Label("封顶")
		})
		ui.Text(c, "两副牌炸弹多，每个炸弹都让倍数翻倍；封顶限制一局的最高倍数。底分和封顶从下一局起生效。").
			FontSize(12).TextColor(inkMuted)
		ui.Row(c).Gap(16).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "记牌器").Width(72)
			ui.Switch(c, &t.set.counter).Label("记牌器")
			ui.Text(c, "显示每种牌还有几张在别人手里").FontSize(12).TextColor(inkMuted)
		})
		ui.Row(c).Justify(ui.End).Gap(10).Children(func() {
			if t.started && ui.Button(c, "重新开始").Clicked() {
				t.scores = [4]int{}
				t.dealsDone = 0
				t.settingsOpen = false
				t.newDeal()
			}
			if ui.PrimaryButton(c, "完成").Width(96).Clicked() {
				t.settingsOpen = false
			}
		})
	})
}

// resultDialog tells who won the deal, how the multiplier came about, what
// each player won or lost, and the cards they were left with.
func (t *table) resultDialog(c *ui.Context) {
	if t.g == nil || t.g.Result == nil {
		return
	}
	r := t.g.Result
	ui.DialogBase(c, &t.resultOpen, func(backdrop, p ui.Element) {
		panel(backdrop, p, 560)
		won := r.Scores[human] > 0
		headline := "农民胜"
		if r.LandlordWon {
			headline = "地主胜"
		}
		ui.Row(c).Gap(14).AlignItems(ui.Center).Children(func() {
			ui.Text(c, headline).Font(kaiFont).FontSize(34).Bold().TextColor(ivory)
			mood, color := "你输了", inkMuted
			if won {
				mood, color = "你赢了", brass
			}
			ui.Text(c, mood).Font(kaiFont).FontSize(20).TextColor(color)
			ui.Spacer(c)
			ui.Text(c, signed(r.Scores[human])).Font(serifFont).FontSize(34).Bold().
				TextColor(color).FontFeatures("lnum")
		})
		ui.Text(c, multiplierLine(r)).FontSize(13).TextColor(inkMuted)
		ui.Column(c).Gap(10).Children(func() {
			for seat := range 4 {
				ui.Row(c.Key(seat)).Gap(12).AlignItems(ui.Center).Children(func() {
					ui.Text(c, seatNames[seat]).Font(kaiFont).FontSize(17).Width(44)
					role := t.role(seat)
					color := jade
					if role == "地主" {
						color = vermilion
					}
					ui.Box(c).Padding(0, 5).Radius(3).Background(color).Shrink(0).Children(func() {
						ui.Text(c, role).Font(kaiFont).FontSize(13).TextColor(ivory).NoWrap()
					})
					ui.Row(c).Grow(1).Basis(0).Height(miniCard.h).Children(func() { leftover(c, t.g.Hands[seat], 250) })
					ui.Text(c, signed(r.Scores[seat])).Font(serifFont).FontSize(17).Bold().Width(56).
						TextAlign(ui.End).FontFeatures("lnum", "tnum")
					ui.Text(c, signed(t.scores[seat])).Font(serifFont).FontSize(13).TextColor(inkMuted).Width(52).
						TextAlign(ui.End).FontFeatures("lnum", "tnum")
				})
			}
		})
		ui.Row(c).Justify(ui.End).Gap(10).Children(func() {
			if ui.Button(c, "看看牌桌").Clicked() {
				t.resultOpen = false
			}
			if ui.PrimaryButton(c, "再来一局").Width(120).AutoFocus().Clicked() {
				t.newDeal()
			}
		})
	})
	if t.g.Phase == game.PhaseFinished && !t.resultOpen && !t.settingsOpen && !t.rulesOpen {
		ui.PrimaryButton(c, "再来一局").Absolute().Bottom(28).Right(36).Width(120).
			OnClick(t.newDeal)
	}
}

// leftover shows the cards a player still held when the deal ended, small
// and overlapping to fit in width.
func leftover(c *ui.Context, cards []game.Card, width float32) {
	if len(cards) == 0 {
		ui.Text(c, "出完").FontSize(12).TextColor(brass)
		return
	}
	step := float32(14)
	if n := len(cards); n > 1 {
		step = min(step, (width-miniCard.w)/float32(n-1))
	}
	ui.Box(c).Size(miniCard.w+step*float32(len(cards)-1), miniCard.h).Children(func() {
		for i, card := range cards {
			cardFace(c.Key(int(card)), faceOf(card), miniCard).Absolute().Left(float32(i) * step)
		}
	})
}

// multiplierLine explains the multiplier: the bid, doubled for each bomb
// and for a spring.
func multiplierLine(r *game.Result) string {
	line := fmt.Sprintf("叫分 %d 分", r.Bid)
	if r.Bombs > 0 {
		line += fmt.Sprintf("，炸弹 %d 个 ×%d", r.Bombs, 1<<r.Bombs)
	}
	if r.Spring {
		line += "，春天 ×2"
	}
	if r.AntiSpring {
		line += "，反春 ×2"
	}
	if r.Capped {
		return line + fmt.Sprintf("，封顶 %d 倍", r.Multiplier)
	}
	return line + fmt.Sprintf("，共 %d 倍", r.Multiplier)
}

// rulesDialog sums up the rules of the four-player game.
func (t *table) rulesDialog(c *ui.Context) {
	ui.DialogBase(c, &t.rulesOpen, func(backdrop, p ui.Element) {
		panel(backdrop, p, 560)
		ui.Text(c, "四人斗地主规则").Font(kaiFont).FontSize(24).TextColor(brass)
		ui.Scroll(c).MaxHeight(440).Children(func() {
			ui.Column(c).Gap(12).PaddingEnd(10).Children(func() {
				for _, section := range rules {
					ui.Column(c).Gap(4).Children(func() {
						ui.Text(c, section[0]).Font(kaiFont).FontSize(17).TextColor(inkText)
						ui.Text(c, section[1]).FontSize(13).TextColor(inkMuted).LineHeight(1.5)
					})
				}
			})
		})
		ui.Row(c).Justify(ui.End).Children(func() {
			if ui.PrimaryButton(c, "知道了").Width(96).Clicked() {
				t.rulesOpen = false
			}
		})
	})
}

var rules = [][2]string{
	{"发牌", "两副牌共 108 张，每人 25 张，留 8 张底牌。"},
	{"叫分", "从首家起依次叫 1、2、3 分或不叫，每次须比之前叫得高；叫到 3 分立即成为地主，否则一轮后叫分最高者当地主。无人叫则重新发牌。地主拿走 8 张底牌并先出牌，其余三人为农民，同为一队。"},
	{"出牌", "按逆时针轮流出牌，须出同牌型且更大的牌，或选择不出；其余三人都不出时，最后出牌者重新领出。先出完手牌的一方获胜。"},
	{"牌型", "单张、对子、三张、三带一、三带一对；顺子（5 张及以上连续单牌，3 到 A）；连对（3 对及以上）；飞机（2 组及以上连续三张，可每组带一张或一对）；四带二（带两张或两对）。"},
	{"大小", "3 最小，然后 4 到 10、J、Q、K、A、2、小王、大王。"},
	{"炸弹", "4 到 8 张同点数的牌为炸弹，张数多的大，张数相同比点数。四张王为天王炸，最大。炸弹可以压任何非炸弹的牌。"},
	{"计分", "倍数 = 叫分 × 2 的炸弹个数次方，春天（农民一张未出）或反春（地主只出过一手）再翻倍，最高不超过设置里的封顶倍数。地主赢，三位农民各输 底分×倍数，地主得三份；地主输则反之。"},
}
