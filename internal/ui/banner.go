package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// bannerArt is the haloed angel wings spread apart to seat the AZRL shadow
// wordmark in the centre gap below the halo. Wings/halo are braille; the
// wordmark is ANSI Shadow box-drawing glyphs. The crest is resampled from the
// original wing dot-bitmap at 0.80x; the wings are nudged down to flank the
// wordmark with a gap either side, the halo shifted left (71x10 cells).
var bannerArt = []string{
	"⢠⣶⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢠⣤⡶⠿⠿⠿⠿⠿⠿⠿⠷⢦⣤⡀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢀⣼⣇⠀",
	"⣿⠘⢷⣄⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢸⣧⣄⡀⠀⠀⠀⠀⠀⠀⢀⣠⣿⠇⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢠⡾⠃⣿⠀",
	"⢹⡀⠈⠻⣶⣴⢳⣆⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠉⠛⠛⠛⠛⠛⠛⠛⠛⠛⠋⠁⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣰⠟⢻⡶⠛⠀⢀⡟⠀",
	"⣼⣷⠀⠀⠈⣿⠀⠙⠓⢶⣤⣄⣀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣀⣠⣤⡶⠟⠛⠀⠈⣶⠀⠀⢈⣶⣶",
	"⢿⡝⢷⣄⠀⢹⠀⠀⠀⠀⠀⠉⠉⠙⠛⠶⣄⠀⠀⠀█████╗⠀⠀███████╗⠀██████╗⠀⠀██╗⠀⠀⠀⠀⠀⠀⡴⠛⠋⠉⠉⠀⠀⠀⠀⠀⢠⡏⣀⣠⠞⢩⡿",
	"⠈⢿⡀⠈⠉⠚⠧⣀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀██╔══██╗⠀╚══███╔╝⠀██╔══██╗⠀██║⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢀⣀⡞⠈⠁⠀⢠⡿⠁",
	"⠀⠈⣳⢤⠀⠀⠘⢿⣖⠒⠊⠁⠀⠀⠀⠀⠀⠀⠀███████║⠀⠀⠀███╔╝⠀⠀██████╔╝⠀██║⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠈⣉⣭⠿⠃⠀⢠⣴⣿⠀⠀",
	"⠀⠈⠻⣿⠛⠒⠒⠒⠉⠛⣶⣶⠒⠂⢀⣰⣰⠀⠀██╔══██║⠀⠀███╔╝⠀⠀⠀██╔══██╗⠀██║⠀⠀⠀⠀⠀⠀⣆⣶⣄⣈⣛⣾⣿⠉⠉⠉⠛⠛⢻⡽⠛⠀⠀",
	"⠀⠀⠀⠘⠓⢶⣤⣀⣠⠴⠊⠙⠛⣻⠙⢛⡇⠀⠀██║⠀⠀██║⠀███████╗⠀██║⠀⠀██║⠀███████╗⠀⠻⣝⡛⣯⡉⠁⠘⠒⢦⣤⣶⠒⠛⠀⠀⠀⠀",
	"⠀⠀⠀⠀⠀⠈⠙⠻⠶⠶⠶⠶⣿⠿⠛⠉⠀⠀⠀╚═╝⠀⠀╚═╝⠀╚══════╝⠀╚═╝⠀⠀╚═╝⠀╚══════╝⠀⠀⠈⠙⠛⠿⠛⠳⠶⠛⠛⠉⠀⠀⠀⠀⠀⠀",
}

// haloGrad is the top-bright gold gradient across the halo's rows (0..2).
var haloGrad = []lipgloss.Color{goldLight, gold, goldDeep}

// wordGrad is a white-hot-to-gold metallic gradient down the wordmark's six
// rows (4..9), giving the AZRL letters their own sheen.
var wordGrad = []lipgloss.Color{
	lipgloss.Color("#f7dc94"),
	lipgloss.Color("#f0cd6f"),
	gold,
	lipgloss.Color("#e3a838"),
	goldDeep,
	lipgloss.Color("#c2851f"),
}

// Grid regions used to colour the crest: the halo is a braille oval boxed by
// these coords; the wordmark is the block-glyph rows starting at wordTop.
const (
	haloTop   = 0
	haloBot   = 2
	haloLeft  = 28
	haloRight = 41
	wordTop   = 4
)

// bannerPad is the number of blank rows above and below the crest.
const bannerPad = 1

// wingBlue is the top-bright blue gradient down the wings by row.
func wingBlue(y int) lipgloss.Color {
	switch {
	case y <= 3:
		return azureSky
	case y <= 7:
		return azureBlue
	default:
		return azureDeep
	}
}

// cellColor colours one art cell: the shadow wordmark (any non-braille glyph)
// gold, a gradient halo above it, and gradient-blue wings elsewhere.
func cellColor(y, x int, r rune) (col lipgloss.Color, bold bool) {
	braille := r >= 0x2800 && r <= 0x28FF
	switch {
	case !braille: // shadow wordmark
		idx := y - wordTop
		if idx < 0 {
			idx = 0
		}
		if idx >= len(wordGrad) {
			idx = len(wordGrad) - 1
		}
		return wordGrad[idx], true
	case y >= haloTop && y <= haloBot && x >= haloLeft && x <= haloRight:
		return haloGrad[y-haloTop], true
	default:
		return wingBlue(y), false
	}
}

// Banner renders the winged AZRL crest: gold shadow wordmark, a gradient halo,
// and gradient-blue wings, padded top and bottom. No tagline.
func Banner() string {
	var lines []string
	for i := 0; i < bannerPad; i++ {
		lines = append(lines, "")
	}
	for y, row := range bannerArt {
		rs := []rune(row)
		var sb strings.Builder
		for j := 0; j < len(rs); {
			col, bold := cellColor(y, j, rs[j])
			k := j
			for k < len(rs) {
				c, b := cellColor(y, k, rs[k])
				if c != col || b != bold {
					break
				}
				k++
			}
			sb.WriteString(lipgloss.NewStyle().Foreground(col).Bold(bold).Render(string(rs[j:k])))
			j = k
		}
		lines = append(lines, sb.String())
	}
	for i := 0; i < bannerPad; i++ {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}
