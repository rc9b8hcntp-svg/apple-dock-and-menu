// Package launchpad contient la partie du Launchpad qui ne dépend pas de
// Windows : la grille d'icônes, les pages, la recherche, le flou du fond et le
// dessin dans un tampon de pixels prémultipliés (0xAARRGGBB). Le code propre à
// Windows (fenêtre, capture de l'écran, icônes des programmes) est dans
// launchpad_windows.go.
package launchpad

import (
	"math"
	"strings"

	"menuwin/internal/dockgfx"
)

// Rect est un rectangle en pixels.
type Rect = dockgfx.Rect

// ---------------------------------------------------------------------------
// Grille et pages
// ---------------------------------------------------------------------------

// Grid décrit la disposition d'une page : colonnes, lignes, taille des icônes,
// champ de recherche et points de pagination.
type Grid struct {
	W, H             int
	Scale            float64
	Cols, Rows       int
	PerPage          int
	CellW, CellH     float64
	GridX, GridY     float64
	IconSize         float64
	LabelH, LabelGap float64
	Search           Rect
	DotsY, DotR      float64
	DotGap           float64
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// NewGrid calcule la grille pour un écran de w×h pixels et une échelle DPI
// (1.0 = 96 dpi). Sur un grand écran : 7 colonnes × 5 lignes, comme sur macOS.
func NewGrid(w, h int, scale float64) Grid {
	fw, fh := float64(w), float64(h)
	side := fw * 0.12
	top := fh * 0.15
	bottom := fh * 0.11
	gw := fw - 2*side
	gh := fh - top - bottom

	g := Grid{W: w, H: h, Scale: scale}
	g.Cols = clampInt(int(gw/(150*scale)), 3, 7)
	g.Rows = clampInt(int(gh/(140*scale)), 2, 5)
	g.PerPage = g.Cols * g.Rows
	g.CellW = gw / float64(g.Cols)
	g.CellH = gh / float64(g.Rows)
	g.GridX, g.GridY = side, top
	g.LabelH, g.LabelGap = 20*scale, 8*scale
	g.IconSize = math.Min(math.Min(g.CellW*0.62, (g.CellH-g.LabelH-g.LabelGap)*0.88), 128*scale)

	sw, sh := 280*scale, 30*scale
	g.Search = Rect{X: (fw - sw) / 2, Y: fh * 0.045, W: sw, H: sh}
	g.DotR, g.DotGap = 4.5*scale, 22*scale
	g.DotsY = fh - bottom*0.45
	return g
}

// Pages donne le nombre de pages pour n applications (au moins une).
func (g Grid) Pages(n int) int {
	if n <= 0 {
		return 1
	}
	return (n + g.PerPage - 1) / g.PerPage
}

// Cell est la case d'un emplacement (0 à PerPage-1) de la page.
func (g Grid) Cell(slot int) Rect {
	col, row := slot%g.Cols, slot/g.Cols
	return Rect{X: g.GridX + float64(col)*g.CellW, Y: g.GridY + float64(row)*g.CellH, W: g.CellW, H: g.CellH}
}

func (g Grid) blockH() float64 { return g.IconSize + g.LabelGap + g.LabelH }

// IconRect est le carré de l'icône d'un emplacement.
func (g Grid) IconRect(slot int) Rect {
	c := g.Cell(slot)
	y0 := c.Y + (c.H-g.blockH())/2
	return Rect{X: c.X + (c.W-g.IconSize)/2, Y: y0, W: g.IconSize, H: g.IconSize}
}

// LabelRect est la zone du nom, sous l'icône.
func (g Grid) LabelRect(slot int) Rect {
	c := g.Cell(slot)
	ir := g.IconRect(slot)
	return Rect{X: c.X + 4*g.Scale, Y: ir.Y + ir.H + g.LabelGap, W: c.W - 8*g.Scale, H: g.LabelH}
}

// Block est la zone sensible d'un emplacement (icône et nom) : un clic à côté
// ferme le Launchpad.
func (g Grid) Block(slot int) Rect {
	c := g.Cell(slot)
	ir := g.IconRect(slot)
	bw := math.Min(c.W*0.92, g.IconSize*1.55)
	pad := 6 * g.Scale
	return Rect{X: c.X + (c.W-bw)/2, Y: ir.Y - pad, W: bw, H: g.blockH() + 2*pad}
}

func in(r Rect, x, y float64) bool { return x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H }

// Slot renvoie l'emplacement situé sous (x, y), ou -1.
func (g Grid) Slot(x, y float64) int {
	for s := 0; s < g.PerPage; s++ {
		if in(g.Block(s), x, y) {
			return s
		}
	}
	return -1
}

// InSearch indique si le point est dans le champ de recherche.
func (g Grid) InSearch(x, y float64) bool { return in(g.Search, x, y) }

// DotCenter est le centre du point de la page i (pour pages points au total).
func (g Grid) DotCenter(i, pages int) (float64, float64) {
	total := float64(pages-1) * g.DotGap
	return float64(g.W)/2 - total/2 + float64(i)*g.DotGap, g.DotsY
}

// DotAt renvoie la page dont le point est sous (x, y), ou -1.
func (g Grid) DotAt(x, y float64, pages int) int {
	if pages < 2 {
		return -1
	}
	for i := 0; i < pages; i++ {
		cx, cy := g.DotCenter(i, pages)
		if math.Abs(x-cx) <= g.DotGap/2 && math.Abs(y-cy) <= g.DotGap/2 {
			return i
		}
	}
	return -1
}

// ---------------------------------------------------------------------------
// Recherche
// ---------------------------------------------------------------------------

var accentFold = strings.NewReplacer(
	"à", "a", "â", "a", "ä", "a", "á", "a", "ã", "a", "å", "a",
	"ç", "c",
	"é", "e", "è", "e", "ê", "e", "ë", "e",
	"î", "i", "ï", "i", "í", "i", "ì", "i",
	"ñ", "n",
	"ô", "o", "ö", "o", "ó", "o", "ò", "o", "õ", "o", "œ", "oe",
	"ù", "u", "û", "u", "ü", "u", "ú", "u",
	"ÿ", "y", "ý", "y",
)

// Fold met en minuscules et retire les accents.
func Fold(s string) string { return accentFold.Replace(strings.ToLower(s)) }

// Filter renvoie les indices des noms qui correspondent à la recherche : d'abord
// ceux dont un mot commence par le texte, puis ceux qui le contiennent. Une
// recherche vide renvoie tous les indices, dans l'ordre.
func Filter(names []string, query string) []int {
	q := Fold(strings.TrimSpace(query))
	out := make([]int, 0, len(names))
	if q == "" {
		for i := range names {
			out = append(out, i)
		}
		return out
	}
	var sub []int
	for i, n := range names {
		f := Fold(n)
		spaced := strings.Map(func(r rune) rune {
			if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
				return r
			}
			return ' '
		}, f)
		switch {
		case strings.HasPrefix(f, q) || strings.Contains(" "+spaced, " "+q):
			out = append(out, i)
		case strings.Contains(f, q):
			sub = append(sub, i)
		}
	}
	return append(out, sub...)
}

// ---------------------------------------------------------------------------
// Fond : capture de l'écran, floutée et assombrie
// ---------------------------------------------------------------------------

// BlurDim renvoie une copie floutée de src (w×h, opaque) assombrie : chaque
// couleur est multipliée par dim (0 à 1). L'image est réduite par blocs de 8
// pixels, floutée, puis agrandie : c'est rapide et le résultat est très doux.
func BlurDim(src []uint32, w, h int, dim float64) []uint32 {
	out := make([]uint32, w*h)
	if w <= 0 || h <= 0 || len(src) < w*h {
		return out
	}
	const f = 8
	sw, sh := (w+f-1)/f, (h+f-1)/f
	small := make([][3]float32, sw*sh)
	for by := 0; by < sh; by++ {
		for bx := 0; bx < sw; bx++ {
			var r, g, b, n float32
			for y := by * f; y < (by+1)*f && y < h; y++ {
				for x := bx * f; x < (bx+1)*f && x < w; x++ {
					p := src[y*w+x]
					r += float32((p >> 16) & 255)
					g += float32((p >> 8) & 255)
					b += float32(p & 255)
					n++
				}
			}
			small[by*sw+bx] = [3]float32{r / n, g / n, b / n}
		}
	}
	tmp := make([][3]float32, sw*sh)
	const rad = 3
	for pass := 0; pass < 2; pass++ {
		boxBlur(small, tmp, sw, sh, rad, true)
		boxBlur(tmp, small, sw, sh, rad, false)
	}

	k := float32(dim)
	for y := 0; y < h; y++ {
		v := (float64(y)+0.5)/f - 0.5
		y0 := int(math.Floor(v))
		fy := float32(v - float64(y0))
		ya, yb := clampInt(y0, 0, sh-1), clampInt(y0+1, 0, sh-1)
		for x := 0; x < w; x++ {
			u := (float64(x)+0.5)/f - 0.5
			x0 := int(math.Floor(u))
			fx := float32(u - float64(x0))
			xa, xb := clampInt(x0, 0, sw-1), clampInt(x0+1, 0, sw-1)
			var c [3]uint32
			for ch := 0; ch < 3; ch++ {
				top := small[ya*sw+xa][ch]*(1-fx) + small[ya*sw+xb][ch]*fx
				bot := small[yb*sw+xa][ch]*(1-fx) + small[yb*sw+xb][ch]*fx
				val := (top*(1-fy) + bot*fy) * k
				if val < 0 {
					val = 0
				}
				if val > 255 {
					val = 255
				}
				c[ch] = uint32(val + 0.5)
			}
			out[y*w+x] = 0xFF000000 | c[0]<<16 | c[1]<<8 | c[2]
		}
	}
	return out
}

// boxBlur floute src dans dst (horizontalement ou verticalement), les bords étant prolongés.
func boxBlur(src, dst [][3]float32, w, h, rad int, horizontal bool) {
	n := float32(2*rad + 1)
	lines, length := h, w
	if !horizontal {
		lines, length = w, h
	}
	at := func(line, i int) int {
		if horizontal {
			return line*w + i
		}
		return i*w + line
	}
	for l := 0; l < lines; l++ {
		var sum [3]float32
		for i := -rad; i <= rad; i++ {
			p := src[at(l, clampInt(i, 0, length-1))]
			sum[0] += p[0]
			sum[1] += p[1]
			sum[2] += p[2]
		}
		for i := 0; i < length; i++ {
			dst[at(l, i)] = [3]float32{sum[0] / n, sum[1] / n, sum[2] / n}
			add := src[at(l, clampInt(i+rad+1, 0, length-1))]
			sub := src[at(l, clampInt(i-rad, 0, length-1))]
			for c := 0; c < 3; c++ {
				sum[c] += add[c] - sub[c]
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Dessin
// ---------------------------------------------------------------------------

// Tile est une icône de la page courante (les noms sont écrits par Windows).
type Tile struct {
	Sprite   *dockgfx.Sprite
	Selected bool
}

// View regroupe ce qu'il faut dessiner.
type View struct {
	Grid  Grid
	Tiles []Tile // un par emplacement utilisé de la page courante
	Page  int
	Pages int
}

func fillRoundRect(buf []uint32, W, H int, r Rect, rad float64, color uint32) {
	for y := int(r.Y) - 1; y <= int(r.Y+r.H)+1; y++ {
		if y < 0 || y >= H {
			continue
		}
		for x := int(r.X) - 1; x <= int(r.X+r.W)+1; x++ {
			if x < 0 || x >= W {
				continue
			}
			d := dockgfx.SDRoundRect(float64(x)+0.5, float64(y)+0.5, r, rad)
			if cov := dockgfx.Clamp01(0.5 - d); cov > 0 {
				buf[y*W+x] = dockgfx.Over(buf[y*W+x], dockgfx.ScaleColor(color, cov))
			}
		}
	}
}

// strokeCircle dessine un cercle (contour de largeur lw).
func strokeCircle(buf []uint32, W, H int, cx, cy, rad, lw float64, color uint32) {
	for y := int(cy-rad-lw) - 1; y <= int(cy+rad+lw)+1; y++ {
		if y < 0 || y >= H {
			continue
		}
		for x := int(cx-rad-lw) - 1; x <= int(cx+rad+lw)+1; x++ {
			if x < 0 || x >= W {
				continue
			}
			d := math.Abs(math.Hypot(float64(x)+0.5-cx, float64(y)+0.5-cy)-rad) - lw/2
			if cov := dockgfx.Clamp01(0.5 - d); cov > 0 {
				buf[y*W+x] = dockgfx.Over(buf[y*W+x], dockgfx.ScaleColor(color, cov))
			}
		}
	}
}

// strokeLine dessine un segment épais aux extrémités arrondies.
func strokeLine(buf []uint32, W, H int, x0, y0, x1, y1, lw float64, color uint32) {
	minX, maxX := math.Min(x0, x1)-lw-1, math.Max(x0, x1)+lw+1
	minY, maxY := math.Min(y0, y1)-lw-1, math.Max(y0, y1)+lw+1
	dx, dy := x1-x0, y1-y0
	l2 := dx*dx + dy*dy
	for y := int(minY); y <= int(maxY); y++ {
		if y < 0 || y >= H {
			continue
		}
		for x := int(minX); x <= int(maxX); x++ {
			if x < 0 || x >= W {
				continue
			}
			px, py := float64(x)+0.5-x0, float64(y)+0.5-y0
			t := 0.0
			if l2 > 0 {
				t = dockgfx.Clamp01((px*dx + py*dy) / l2)
			}
			d := math.Hypot(px-t*dx, py-t*dy) - lw/2
			if cov := dockgfx.Clamp01(0.5 - d); cov > 0 {
				buf[y*W+x] = dockgfx.Over(buf[y*W+x], dockgfx.ScaleColor(color, cov))
			}
		}
	}
}

// Paint dessine le Launchpad dans buf : fond, champ de recherche, icônes et points.
// buf et bg ont la taille W×H de la grille.
func Paint(buf, bg []uint32, v View) {
	g := v.Grid
	W, H := g.W, g.H
	copy(buf, bg)

	// Champ de recherche : pastille translucide avec une loupe à gauche.
	sr := g.Search
	fillRoundRect(buf, W, H, sr, sr.H/2, dockgfx.Premul(255, 255, 255, 0.16))
	white := dockgfx.Premul(255, 255, 255, 0.75)
	cx, cy := sr.X+16*g.Scale, sr.Y+sr.H/2-1*g.Scale
	strokeCircle(buf, W, H, cx, cy, 5*g.Scale, 1.6*g.Scale, white)
	strokeLine(buf, W, H, cx+3.6*g.Scale, cy+3.6*g.Scale, cx+7.5*g.Scale, cy+7.5*g.Scale, 1.8*g.Scale, white)

	for slot, t := range v.Tiles {
		if t.Selected {
			fillRoundRect(buf, W, H, g.Block(slot), 14*g.Scale, dockgfx.Premul(255, 255, 255, 0.17))
		}
		dockgfx.DrawSprite(buf, W, H, t.Sprite, g.IconRect(slot))
	}

	if v.Pages > 1 {
		for i := 0; i < v.Pages; i++ {
			dx, dy := g.DotCenter(i, v.Pages)
			a := 0.38
			if i == v.Page {
				a = 0.95
			}
			dockgfx.DrawDisc(buf, W, H, dx, dy, g.DotR, dockgfx.Premul(255, 255, 255, a))
		}
	}
}

// ---------------------------------------------------------------------------
// Icône du Launchpad dans le Dock (dessinée ici : aucun fichier image)
// ---------------------------------------------------------------------------

// Icon dessine l'icône du Launchpad : une plaque argentée portant une grille
// de petites applications colorées.
func Icon(size int) *dockgfx.Sprite {
	s := &dockgfx.Sprite{W: size, H: size, Pix: make([]uint32, size*size)}
	fs := float64(size)
	body := Rect{X: fs * 0.04, Y: fs * 0.04, W: fs * 0.92, H: fs * 0.92}
	rad := fs * 0.22
	for y := 0; y < size; y++ {
		t := (float64(y) + 0.5) / fs
		r := uint8(238 - 86*t)
		g := uint8(238 - 84*t)
		b := uint8(244 - 78*t)
		base := dockgfx.Premul(r, g, b, 1)
		for x := 0; x < size; x++ {
			d := dockgfx.SDRoundRect(float64(x)+0.5, float64(y)+0.5, body, rad)
			if cov := dockgfx.Clamp01(0.5 - d); cov > 0 {
				s.Pix[y*size+x] = dockgfx.ScaleColor(base, cov)
			}
		}
	}
	colors := [9][3]uint8{
		{10, 132, 255}, {48, 209, 88}, {255, 159, 10},
		{255, 55, 95}, {175, 82, 222}, {90, 200, 250},
		{255, 204, 0}, {255, 69, 58}, {94, 92, 230},
	}
	cell, gap := fs*0.17, fs*0.07
	x0 := (fs - (3*cell + 2*gap)) / 2
	for i, c := range colors {
		r := Rect{X: x0 + float64(i%3)*(cell+gap), Y: x0 + float64(i/3)*(cell+gap), W: cell, H: cell}
		fillRoundRect(s.Pix, size, size, r, cell*0.28, dockgfx.Premul(c[0], c[1], c[2], 1))
	}
	return s
}
