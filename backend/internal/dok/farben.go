// The colors the editor knows, in the forms PDF and Word require.

// In the stored text there is no color value but a name: "yellow", "red",
// "blue". That is the editor's vocabulary and it is intentional — a name
// survives a theme change whereas a color value does not. Translation to the
// target format happens here during rendering and is specific to each target:
// PDF wants three numbers between 0 and 1, Word wants six hex digits and the
// highlighting feature even requires a fixed name from its palette.
package dok

import "fmt"

// rgb ist eine Farbe, wie das PDF sie braucht.
type rgb struct{ r, g, b float64 }

// pdfFarbe schreibt den Operator, der ab hier gilt.
func (c rgb) pdfFarbe() string {
	return fmt.Sprintf("%.3f %.3f %.3f rg", c.r, c.g, c.b)
}

// hex schreibt dieselbe Farbe für Word.
func (c rgb) hex() string {
	n := func(v float64) int {
		x := int(v*255 + 0.5)
		if x < 0 {
			return 0
		}
		if x > 255 {
			return 255
		}
		return x
	}
	return fmt.Sprintf("%02X%02X%02X", n(c.r), n(c.g), n(c.b))
}

// schriftfarben sind die Farben für den Text selbst. Kräftig genug, um auf
// Weiß gelesen zu werden, und nicht kräftiger: eine Seite, auf der jedes zweite
// Wort leuchtet, liest niemand.
var schriftfarben = map[string]rgb{
	"gray":   {0.42, 0.42, 0.40},
	"brown":  {0.51, 0.36, 0.24},
	"red":    {0.79, 0.20, 0.16},
	"orange": {0.84, 0.45, 0.09},
	"yellow": {0.72, 0.55, 0.05},
	"green":  {0.11, 0.52, 0.35},
	"blue":   {0.14, 0.51, 0.89},
	"purple": {0.51, 0.29, 0.75},
	"pink":   {0.79, 0.21, 0.44},
}

// hintergrundfarben sind die Markierungen. Blass, weil darauf gelesen wird:
// derselbe Ton wie in der Oberfläche, nur eine Spur heller, damit auch
// gedruckt noch Text darunter zu erkennen ist.
var hintergrundfarben = map[string]rgb{
	"gray":   {0.90, 0.90, 0.89},
	"brown":  {0.92, 0.86, 0.80},
	"red":    {0.98, 0.85, 0.83},
	"orange": {0.99, 0.90, 0.78},
	"yellow": {1.00, 0.96, 0.70},
	"green":  {0.84, 0.94, 0.88},
	"blue":   {0.85, 0.92, 0.99},
	"purple": {0.92, 0.87, 0.97},
	"pink":   {0.99, 0.87, 0.92},
}

// wordMarker sind die Namen, die Word für seine Markierung erlaubt. Eine feste
// Palette, keine freien Werte -- deshalb hier die Zuordnung und nicht der
// Hexwert von oben.
var wordMarker = map[string]string{
	"gray":   "lightGray",
	"brown":  "darkYellow",
	"red":    "red",
	"orange": "yellow",
	"yellow": "yellow",
	"green":  "green",
	"blue":   "cyan",
	"purple": "magenta",
	"pink":   "magenta",
}

// schriftfarbe und hintergrundfarbe liefern die Farbe zu einem Namen. Ein
// unbekannter Name und "default" ergeben nichts -- dann bleibt es beim
// gewöhnlichen Satz, was richtiger ist, als etwas zu raten.
func schriftfarbe(name string) (rgb, bool) {
	c, ok := schriftfarben[name]
	return c, ok
}

func hintergrundfarbe(name string) (rgb, bool) {
	c, ok := hintergrundfarben[name]
	return c, ok
}
