// Package dockgfx dessine le Dock (barre d'icônes du bas de l'écran) dans un
// tampon de pixels. Il ne dépend pas de Windows : le code est testable
// partout, et l'application n'a plus qu'à envoyer le tampon à l'écran.
//
// Format des pixels : ARGB 32 bits « prémultiplié » (comme l'attend
// UpdateLayeredWindow) : 0xAARRGGBB, avec R, G, B déjà multipliés par A.
package dockgfx

import "math"

// Sprite est une image prémultipliée.
type Sprite struct {
	W, H int
	Pix  []uint32
}

// Item est une case du Dock : une icône ou un séparateur.
type Item struct {
	Sprite    *Sprite
	Separator bool
	Running   bool    // affiche le petit point sous l'icône
	Lift      float64 // décalage vers le haut en pixels (rebond au lancement)
	Shadow    *Sprite // ombre douce sous l'icône (facultative)
}

// Rect est un rectangle en pixels (flottants pour l'animation).
type Rect struct{ X, Y, W, H float64 }

// Params regroupe toutes les dimensions, déjà multipliées par l'échelle DPI.
type Params struct {
	Base   float64 // taille d'une icône au repos
	Max    float64 // taille maximale d'une icône sous la souris
	Gap    float64 // espace entre deux cases
	PadX   float64 // marge gauche/droite dans la barre
	PadY   float64 // marge haut/bas dans la barre
	DotH   float64 // hauteur réservée au point « application ouverte »
	Bottom float64 // espace sous la barre
	SepW   float64 // largeur d'un séparateur
	Radius float64 // portée du zoom
	Corner float64 // arrondi de la barre
}

// NewParams donne les dimensions pour une échelle DPI (1.0 = 96 dpi).
func NewParams(scale float64) Params {
	b := 48 * scale
	return Params{
		Base:   b,
		Max:    b * 1.65,
		Gap:    6 * scale,
		PadX:   12 * scale,
		PadY:   8 * scale,
		DotH:   9 * scale,
		Bottom: 6 * scale,
		SepW:   14 * scale,
		Radius: b * 2.4,
		Corner: 20 * scale,
	}
}

// PanelH est la hauteur de la barre.
func (p Params) PanelH() float64 { return p.Base + 2*p.PadY + p.DotH }

// WindowH est la hauteur de la fenêtre : la barre plus la place pour que les
// icônes grossissent vers le haut.
func (p Params) WindowH() int {
	return int(math.Ceil(p.Max - p.Base + p.PanelH() + p.Bottom + 4))
}

// RestWidth est la largeur de la barre quand rien n'est agrandi.
func (p Params) RestWidth(items []Item) float64 {
	w := 2 * p.PadX
	for i, it := range items {
		if it.Separator {
			w += p.SepW
		} else {
			w += p.Base
		}
		if i > 0 {
			w += p.Gap
		}
	}
	return w
}

// WindowW est la largeur de la fenêtre : la barre au repos plus la place
// nécessaire quand les icônes s'agrandissent.
func (p Params) WindowW(items []Item) int {
	return int(math.Ceil(p.RestWidth(items) + 3*p.Base))
}

// Layout est la position de chaque élément pour une position de souris.
type Layout struct {
	Panel Rect
	Items []Rect
}

func bump(d, r float64) float64 {
	if d >= r {
		return 0
	}
	return 0.5 * (1 + math.Cos(math.Pi*d/r))
}

// Compute calcule la disposition. mouseX est la position de la souris dans la
// fenêtre ; strength (0 à 1) dose l'effet de zoom (0 = repos).
func Compute(p Params, items []Item, winW, winH int, mouseX, strength float64) Layout {
	n := len(items)
	restLeft := (float64(winW)-p.RestWidth(items))/2 + p.PadX
	centers := make([]float64, n)
	x := restLeft
	for i, it := range items {
		w := p.Base
		if it.Separator {
			w = p.SepW
		}
		centers[i] = x + w/2
		x += w + p.Gap
	}

	sizes := make([]float64, n)
	total := 0.0
	for i, it := range items {
		if it.Separator {
			sizes[i] = p.SepW
		} else {
			d := math.Abs(mouseX - centers[i])
			sizes[i] = p.Base * (1 + (p.Max/p.Base-1)*strength*bump(d, p.Radius))
		}
		total += sizes[i]
		if i > 0 {
			total += p.Gap
		}
	}

	panelW := total + 2*p.PadX
	panelX := (float64(winW) - panelW) / 2
	panelY1 := float64(winH) - p.Bottom
	panelY0 := panelY1 - p.PanelH()
	iconBottom := panelY1 - p.PadY - p.DotH

	l := Layout{Panel: Rect{panelX, panelY0, panelW, p.PanelH()}, Items: make([]Rect, n)}
	x = panelX + p.PadX
	for i, it := range items {
		if it.Separator {
			l.Items[i] = Rect{x, iconBottom - p.Base, sizes[i], p.Base}
		} else {
			l.Items[i] = Rect{x, iconBottom - sizes[i], sizes[i], sizes[i]}
		}
		x += sizes[i] + p.Gap
	}
	return l
}

// HitTest renvoie l'index de la case sous (x, y), ou -1. Les séparateurs ne
// comptent pas. La zone cliquable couvre toute la hauteur de la barre, pour
// qu'on puisse viser entre deux icônes.
func HitTest(p Params, items []Item, l Layout, x, y float64) int {
	if y < l.Panel.Y-(p.Max-p.Base) || y > l.Panel.Y+l.Panel.H {
		return -1
	}
	for i, r := range l.Items {
		if items[i].Separator {
			continue
		}
		if x >= r.X-p.Gap/2 && x < r.X+r.W+p.Gap/2 {
			// au-dessus de la barre, on ne compte que l'icône elle-même
			if y < l.Panel.Y && y < r.Y {
				return -1
			}
			return i
		}
	}
	return -1
}

// ---------------------------------------------------------------------------
// Dessin
// ---------------------------------------------------------------------------

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// over compose src par-dessus dst (tous deux prémultipliés).
func over(dst, src uint32) uint32 {
	sa := src >> 24
	if sa == 255 {
		return src
	}
	if sa == 0 {
		return dst
	}
	inv := 255 - sa
	a := sa + (dst>>24)*inv/255
	r := (src>>16)&255 + ((dst>>16)&255)*inv/255
	g := (src>>8)&255 + ((dst>>8)&255)*inv/255
	b := src&255 + (dst&255)*inv/255
	if r > 255 {
		r = 255
	}
	if g > 255 {
		g = 255
	}
	if b > 255 {
		b = 255
	}
	if a > 255 {
		a = 255
	}
	return a<<24 | r<<16 | g<<8 | b
}

// premul fabrique un pixel prémultiplié à partir d'une couleur et d'une
// opacité (0 à 1).
func premul(r, g, b uint8, alpha float64) uint32 {
	a := clamp01(alpha)
	return uint32(a*255+0.5)<<24 |
		uint32(float64(r)*a+0.5)<<16 |
		uint32(float64(g)*a+0.5)<<8 |
		uint32(float64(b)*a+0.5)
}

func sdRoundRect(px, py float64, r Rect, rad float64) float64 {
	cx, cy := r.X+r.W/2, r.Y+r.H/2
	qx := math.Abs(px-cx) - (r.W/2 - rad)
	qy := math.Abs(py-cy) - (r.H/2 - rad)
	return math.Hypot(math.Max(qx, 0), math.Max(qy, 0)) + math.Min(math.Max(qx, qy), 0) - rad
}

func scaleColor(c uint32, k float64) uint32 {
	k = clamp01(k)
	return uint32(float64(c>>24)*k+0.5)<<24 |
		uint32(float64((c>>16)&255)*k+0.5)<<16 |
		uint32(float64((c>>8)&255)*k+0.5)<<8 |
		uint32(float64(c&255)*k+0.5)
}

// sample lit le sprite en (u, v) avec une interpolation bilinéaire.
func (s *Sprite) sample(u, v float64) uint32 {
	x0, y0 := int(math.Floor(u)), int(math.Floor(v))
	fx, fy := u-float64(x0), v-float64(y0)
	get := func(x, y int) [4]float64 {
		if x < 0 {
			x = 0
		}
		if y < 0 {
			y = 0
		}
		if x >= s.W {
			x = s.W - 1
		}
		if y >= s.H {
			y = s.H - 1
		}
		p := s.Pix[y*s.W+x]
		return [4]float64{float64(p >> 24), float64((p >> 16) & 255), float64((p >> 8) & 255), float64(p & 255)}
	}
	a, b, c, d := get(x0, y0), get(x0+1, y0), get(x0, y0+1), get(x0+1, y0+1)
	var o [4]float64
	for i := 0; i < 4; i++ {
		top := a[i]*(1-fx) + b[i]*fx
		bot := c[i]*(1-fx) + d[i]*fx
		o[i] = top*(1-fy) + bot*fy
	}
	return uint32(o[0]+0.5)<<24 | uint32(o[1]+0.5)<<16 | uint32(o[2]+0.5)<<8 | uint32(o[3]+0.5)
}

func drawSprite(buf []uint32, W, H int, s *Sprite, r Rect) {
	if s == nil || s.W == 0 || s.H == 0 || r.W < 1 || r.H < 1 {
		return
	}
	x0, x1 := int(math.Floor(r.X)), int(math.Ceil(r.X+r.W))
	y0, y1 := int(math.Floor(r.Y)), int(math.Ceil(r.Y+r.H))
	for y := y0; y < y1; y++ {
		if y < 0 || y >= H {
			continue
		}
		v := (float64(y)+0.5-r.Y)/r.H*float64(s.H) - 0.5
		for x := x0; x < x1; x++ {
			if x < 0 || x >= W {
				continue
			}
			u := (float64(x)+0.5-r.X)/r.W*float64(s.W) - 0.5
			px := s.sample(u, v)
			if px>>24 == 0 {
				continue
			}
			buf[y*W+x] = over(buf[y*W+x], px)
		}
	}
}

func drawDisc(buf []uint32, W, H int, cx, cy, rad float64, color uint32) {
	for y := int(cy - rad - 1); y <= int(cy+rad+1); y++ {
		if y < 0 || y >= H {
			continue
		}
		for x := int(cx - rad - 1); x <= int(cx+rad+1); x++ {
			if x < 0 || x >= W {
				continue
			}
			d := math.Hypot(float64(x)+0.5-cx, float64(y)+0.5-cy) - rad
			cov := clamp01(0.5 - d)
			if cov > 0 {
				buf[y*W+x] = over(buf[y*W+x], scaleColor(color, cov))
			}
		}
	}
}

// Render dessine tout le Dock dans buf (W×H pixels) avec l'aspect de base. hitAlpha
// est l'opacité quasi invisible dont on remplit le fond : sans elle, Windows laisse
// passer la souris à travers la fenêtre transparente.
func Render(buf []uint32, W, H int, p Params, items []Item, l Layout, hitAlpha uint8) {
	RenderStyled(buf, W, H, p, items, l, hitAlpha, Style{})
}

// ---------------------------------------------------------------------------
// Images
// ---------------------------------------------------------------------------

// FromBGRA crée un sprite depuis des octets B,G,R,A (ordre Windows). Si
// premultiplied est faux, les couleurs sont multipliées par l'opacité.
func FromBGRA(w, h int, b []byte, premultiplied bool) *Sprite {
	s := &Sprite{W: w, H: h, Pix: make([]uint32, w*h)}
	for i := 0; i < w*h && i*4+3 < len(b); i++ {
		bb, g, r, a := uint32(b[i*4]), uint32(b[i*4+1]), uint32(b[i*4+2]), uint32(b[i*4+3])
		if !premultiplied {
			bb = (bb*a + 127) / 255
			g = (g*a + 127) / 255
			r = (r*a + 127) / 255
		}
		s.Pix[i] = a<<24 | r<<16 | g<<8 | bb
	}
	return s
}

// ResizeArea réduit (ou agrandit) un sprite en faisant la moyenne des pixels
// couverts. C'est le bon choix pour réduire une icône de 256 px.
func ResizeArea(src *Sprite, dw, dh int) *Sprite {
	if src == nil || dw <= 0 || dh <= 0 {
		return nil
	}
	if src.W == dw && src.H == dh {
		return src
	}
	out := &Sprite{W: dw, H: dh, Pix: make([]uint32, dw*dh)}
	sx := float64(src.W) / float64(dw)
	sy := float64(src.H) / float64(dh)
	for y := 0; y < dh; y++ {
		y0, y1 := float64(y)*sy, float64(y+1)*sy
		for x := 0; x < dw; x++ {
			x0, x1 := float64(x)*sx, float64(x+1)*sx
			var acc [4]float64
			wsum := 0.0
			for yy := int(y0); yy < int(math.Ceil(y1)) && yy < src.H; yy++ {
				wy := math.Min(y1, float64(yy+1)) - math.Max(y0, float64(yy))
				for xx := int(x0); xx < int(math.Ceil(x1)) && xx < src.W; xx++ {
					wx := math.Min(x1, float64(xx+1)) - math.Max(x0, float64(xx))
					w := wx * wy
					p := src.Pix[yy*src.W+xx]
					acc[0] += float64(p>>24) * w
					acc[1] += float64((p>>16)&255) * w
					acc[2] += float64((p>>8)&255) * w
					acc[3] += float64(p&255) * w
					wsum += w
				}
			}
			if wsum > 0 {
				out.Pix[y*dw+x] = uint32(acc[0]/wsum+0.5)<<24 | uint32(acc[1]/wsum+0.5)<<16 |
					uint32(acc[2]/wsum+0.5)<<8 | uint32(acc[3]/wsum+0.5)
			}
		}
	}
	return out
}
