package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	figure "github.com/common-nighthawk/go-figure"
)

// Banner renders the AZRL block-letter banner with a blue gradient, the angel
// art, and the tagline.
func Banner() string {
	fig := figure.NewFigure("AZRL", "standard", true).String()
	blues := []lipgloss.Color{azureBlue, azureBlue, azureDeep, azureDeep}
	var lines []string
	for i, line := range strings.Split(strings.TrimRight(fig, "\n"), "\n") {
		c := blues[i%len(blues)]
		lines = append(lines, lipgloss.NewStyle().Foreground(c).Render(line))
	}
	logo := strings.Join(lines, "\n")
	angel := lipgloss.NewStyle().Foreground(gold).Render(AngelArt)
	top := lipgloss.JoinHorizontal(lipgloss.Top, angel, "  ", logo)
	tagline := accentStyle.Render("Azure Remote Login")
	return lipgloss.JoinVertical(lipgloss.Left, top, "", tagline)
}
