package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Block-letter glyphs for A Z R L, five rows tall, four columns wide.
var (
	glyphA = []string{" ██ ", "█  █", "████", "█  █", "█  █"}
	glyphZ = []string{"████", "  █ ", " █  ", "█   ", "████"}
	glyphR = []string{"███ ", "█  █", "███ ", "█ █ ", "█  █"}
	glyphL = []string{"█   ", "█   ", "█   ", "█   ", "████"}

	azrlGlyphs = [][]string{glyphA, glyphZ, glyphR, glyphL}

	// Braille angel wings, mirrored across the wordmark. Drawn pixel-by-pixel as
	// a fan of tapering feathers and packed into braille (2x4 dots per cell):
	// wrists low at the wordmark, primaries sweeping up and out to the tips.
	leftWing = []string{
		"⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⢀⣀⣒⣲⣺⣒⣒⣒⢒⠐⠀⠀⠀⠀⠀⠀",
		"⠀⠀⠀⠀⠀⠀⠀⢐⢐⣴⣿⣿⣿⣿⣿⣿⣿⣿⣿⣻⣒⢒⠀⠀⠀⠀",
		"⠀⠀⠀⠀⠀⠀⠀⡭⣯⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⠛⠀⠀⠀",
		"⠀⠀⠀⠀⣠⣺⣺⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⡿⡯⠯⠁⠀⠀⠀",
		"⠀⠀⠀⢐⢐⣒⣺⣿⣿⣿⣿⣿⣿⣿⣿⡿⠯⠍⠁⠀⠀⠀⠀⠀⠀⠀",
	}
	rightWing = []string{
		"⠀⠀⠀⠀⠀⠀⢀⣐⣒⣒⣒⣺⣚⣒⠒⠐⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀",
		"⠀⠀⠀⠀⣐⣒⣾⣿⣿⣿⣿⣿⣿⣿⣿⣿⢛⢐⢐⠀⠀⠀⠀⠀⠀⠀",
		"⠀⠀⠀⣤⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⡿⠯⠀⠀⠀⠀⠀⠀⠀",
		"⠀⠀⠀⠄⡭⡯⣯⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣿⣺⣺⠚⠀⠀⠀⠀",
		"⠀⠀⠀⠀⠀⠀⠀⠄⠥⡭⣯⣿⣿⣿⣿⣿⣿⣿⣿⣺⣒⢐⢐⠀⠀⠀",
	}
)

// wordmark builds the five-row AZRL block letters.
func wordmark() []string {
	rows := make([]string, 5)
	for i := 0; i < 5; i++ {
		parts := make([]string, len(azrlGlyphs))
		for j, g := range azrlGlyphs {
			parts[j] = g[i]
		}
		rows[i] = strings.Join(parts, "  ")
	}
	return rows
}

// Banner renders gold AZRL block letters flanked by blue braille angel wings
// in a top-bright gradient, above the tagline.
func Banner() string {
	word := wordmark()
	wingBlues := []lipgloss.Color{azureSky, azureSky, azureBlue, azureBlue, azureDeep}

	var lines []string
	for i := 0; i < 5; i++ {
		wing := lipgloss.NewStyle().Foreground(wingBlues[i])
		row := wing.Render(leftWing[i]) + "  " +
			accentStyle.Render(word[i]) + "  " +
			wing.Render(rightWing[i])
		lines = append(lines, row)
	}
	logo := strings.Join(lines, "\n")
	tagline := accentStyle.Render("Azure Remote Login")
	block := lipgloss.JoinVertical(lipgloss.Center, logo, "", tagline)
	return block
}
