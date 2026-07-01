package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// bannerArt is the haloed angel wings spread apart to seat the AZRL shadow
// wordmark in the centre gap below the halo, with no wing art overwritten.
// Wings/halo are braille; the wordmark is box-drawing glyphs.
var bannerArt = []string{
	"⠀⠀⠀⢠⣶⡀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢠⡿⣇⠀⠀⠀⠀",
	"⠀⠀⠀⣾⠈⢷⡄⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣀⣤⣴⣶⣶⣶⣶⣶⣶⣶⣶⣦⣤⣀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣠⡿⠁⣿⠀⠀⠀⠀",
	"⠀⠀⠀⢿⠀⠈⢿⣦⡀⠀⣠⡀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣾⡟⠋⠉⠀⠀⠀⠀⠀⠀⠀⠀⠈⠉⠻⣷⡄⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢠⣤⡄⢀⣴⠟⠁⠀⣿⠀⠀⠀⠀",
	"⠀⠀⠀⢸⡄⠀⠀⠙⢿⣾⠋⢿⣤⡀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠻⣷⣦⣤⣀⣀⣀⣀⣀⣀⣀⣀⣠⣤⣾⠟⠁⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣀⣴⠟⠈⣿⠟⠁⠀⠀⢠⠇⠀⠀⠀⠀",
	"⠀⠀⠀⣴⣷⡀⠀⠀⠀⣿⠀⠀⠈⠛⠷⣦⣤⣀⣀⠀⠀⠀⠀⠀⠀⠀⠀ ⠀⠀⠀⠀⠀⠀⠀⠀⠀⠉⠙⠛⠛⠛⠛⠛⠛⠛⠛⠋⠉⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣀⣠⣤⣴⠶⠟⠋⠀⠀⠀⣿⠀⠀⠀⢀⣾⣶⠀⠀⠀",
	"⠀⠀⠀⣿⡻⢷⣄⠀⠀⢹⡀⠀⠀⠀⠀⠀⠈⠉⠙⠛⠷⢦⣄⡀⠀⠀⠀ ⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣀⡴⠾⠛⠉⠉⠀⠀⠀⠀⠀⠀⠀⢀⡏⠀⠀⣠⠿⢋⣿⠀⠀⠀",
	"⠀⠀⠀⠸⣧⠀⠉⠳⠤⣈⢧⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠈⠙⢦⡀⠀ ⠀⠀█████╗⠀███████╗██████╗⠀██╗⠀⠀⠀⠀⠀⢠⠞⠉⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢀⡼⠡⠖⠋⠁⠀⣼⠇⠀⠀⠀",
	"⠀⠀⠀⠀⠹⣦⠀⠀⠀⠀⠈⣱⣄⠀⠀⠀⣀⠀⠀⠀⠀⠀⠀⠀⠀⠹⡄ ⠀██╔══██╗╚══███╔╝██╔══██╗██║⠀⠀⠀⠀⢠⠏⠀⠀⠀⠀⠀⠀⠀⠀⠀⣀⣀⣀⣤⣮⠀⠀⠀⠀⢀⣼⠋⠀⠀⠀⠀",
	"⠀⠀⠀⠀⠀⢈⡳⣄⠀⠀⠀⠙⢿⣍⠉⠉⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⡇ ⠀███████║⠀⠀███╔╝⠀██████╔╝██║⠀⠀⠀⠀⢸⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⣀⣤⠾⠋⠀⠀⢀⣴⣟⣥⠀⠀⠀⠀⠀",
	"⠀⠀⠀⠀⠀⠹⣿⡛⠿⠶⠶⠖⠒⠉⠛⢒⣶⣶⠶⠂⠀⠀⡄⢠⠄⠀⠀ ⠀██╔══██║⠀███╔╝⠀⠀██╔══██╗██║⠀⠀⠀⠀⠈⠀⠀⣆⢰⣄⠀⠈⠛⢶⣿⡏⠉⠁⠉⠙⠛⠛⠛⣋⡿⠋⠀⠀⠀⠀⠀",
	"⠀⠀⠀⠀⠀⠀⠈⠻⣦⣄⠀⠀⠀⠀⢀⡨⠛⠷⠶⠶⡖⢿⠷⡏⠀⠀⠀ ⠀██║⠀⠀██║███████╗██║⠀⠀██║███████╗⠀⠀⠸⡟⠿⢹⡛⠛⠛⠉⠣⣄⡀⠀⢀⣀⣤⠾⠋⠀⠀⠀⠀⠀⠀⠀",
	"⠀⠀⠀⠀⠀⠀⠀⠀⠀⠉⣿⣷⡶⠚⠉⠀⠀⠀⣀⡾⣁⣤⠞⠀⠀⠀⠀ ⠀╚═╝⠀⠀╚═╝╚══════╝╚═╝⠀⠀╚═╝╚══════╝⠀⠀⠀⠙⢶⣤⣻⣦⣀⣀⠀⠀⣉⣻⣿⡿⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀",
	"⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠉⠛⠛⠛⠚⠛⠛⠿⠛⠋⠁⠀⠀⠀⠀⠀ ⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠈⠉⠛⠉⠉⠙⠛⠉⠉⠁⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀",
}

// haloGrad is the top-bright gold gradient across the halo's four rows (1..4).
var haloGrad = []lipgloss.Color{goldLight, gold, gold, goldDeep}

// wordGrad is a white-hot-to-gold metallic gradient down the wordmark's six
// rows (6..11), giving the AZRL letters their own sheen.
var wordGrad = []lipgloss.Color{
	lipgloss.Color("#f7dc94"),
	lipgloss.Color("#f0cd6f"),
	gold,
	lipgloss.Color("#e3a838"),
	goldDeep,
	lipgloss.Color("#c2851f"),
}

// bannerPad is the number of blank rows above and below the crest.
const bannerPad = 2

// wingBlue is the top-bright blue gradient down the wings by row.
func wingBlue(y int) lipgloss.Color {
	switch {
	case y <= 3:
		return azureSky
	case y <= 8:
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
		idx := y - 6
		if idx < 0 {
			idx = 0
		}
		if idx >= len(wordGrad) {
			idx = len(wordGrad) - 1
		}
		return wordGrad[idx], true
	case y >= 1 && y <= 4 && x >= 34 && x <= 52:
		return haloGrad[y-1], true
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
