package dockgfx

import "math"

// Style règle l'aspect « verre dépoli » du Dock, comme sur macOS : le fond de la
// barre est le décor flouté de l'écran, teinté de clair ou de sombre, avec un fin
// liseré lumineux. Un Style vide donne l'ancien aspect (fond sombre uni).
type Style struct {
	Glass    bool     // aspect macOS
	Dark     bool     // thème sombre
	Backdrop []uint32 // pixels opaques (W×H) du décor flouté, ou nil
	Orient   Orient   // côté de l'écran où se trouve le Dock
}

// Orient est le bord de l'écran où le Dock est posé.
type Orient int

const (
	Bottom Orient = iota // en bas (par défaut)
	Left                 // à gauche
	Right                // à droite
)

// Le moteur (Compute, HitTest) travaille toujours « Dock en bas » dans un repère moteur
// de largeur We et de hauteur He. Pour un Dock sur le côté, la fenêtre fait He × We et
// ces fonctions passent du repère moteur au repère de la fenêtre, sans tourner les icônes.

// WindowSize donne la taille de la fenêtre pour un repère moteur We × He.
func (o Orient) WindowSize(we, he int) (int, int) {
	if o == Bottom {
		return we, he
	}
	return he, we
}

// Engine donne les dimensions du repère moteur d'après celles de la fenêtre.
func (o Orient) Engine(w, h int) (int, int) {
	if o == Bottom {
		return w, h
	}
	return h, w
}

// Rect passe un rectangle du repère moteur à celui de la fenêtre.
func (o Orient) Rect(r Rect, we, he float64) Rect {
	switch o {
	case Left:
		return Rect{he - (r.Y + r.H), r.X, r.H, r.W}
	case Right:
		return Rect{r.Y, r.X, r.H, r.W}
	}
	return r
}

// ToEngine passe un point de la fenêtre au repère moteur.
func (o Orient) ToEngine(x, y, we, he float64) (float64, float64) {
	switch o {
	case Left:
		return y, he - x
	case Right:
		return y, x
	}
	return x, y
}

// FromEngine passe un point du repère moteur à la fenêtre.
func (o Orient) FromEngine(x, y, we, he float64) (float64, float64) {
	switch o {
	case Left:
		return he - y, x
	case Right:
		return y, x
	}
	return x, y
}

// near donne, pour un pixel de la fenêtre dans la barre pr, la proximité du bord de l'écran (0 loin, 1 contre).
func (o Orient) near(x, y float64, pr Rect) float64 {
	switch o {
	case Left:
		return clamp01(1 - (x-pr.X)/pr.W)
	case Right:
		return clamp01((x - pr.X) / pr.W)
	}
	return clamp01((y - pr.Y) / pr.H)
}

// RenderStyled dessine le Dock avec un style.
func RenderStyled(buf []uint32, W, H int, p Params, items []Item, l Layout, hitAlpha uint8, st Style) {
	fill := uint32(hitAlpha) << 24
	for i := range buf {
		buf[i] = fill
	}
	we, he := st.Orient.Engine(W, H)
	pr := st.Orient.Rect(l.Panel, float64(we), float64(he))
	if st.Glass {
		drawGlassPanel(buf, W, H, p, pr, st)
	} else {
		drawFlatPanel(buf, W, H, p, pr)
	}
	drawItems(buf, W, H, p, items, l, st.Orient, float64(we), float64(he))
}

func drawFlatPanel(buf []uint32, W, H int, p Params, pr Rect) {
	panelFill := premul(30, 30, 34, 0.58)
	border := premul(255, 255, 255, 0.20)
	bw := math.Max(1, p.Corner/16)
	for y := int(pr.Y) - 1; y <= int(pr.Y+pr.H)+1; y++ {
		if y < 0 || y >= H {
			continue
		}
		for x := int(pr.X) - 1; x <= int(pr.X+pr.W)+1; x++ {
			if x < 0 || x >= W {
				continue
			}
			d := sdRoundRect(float64(x)+0.5, float64(y)+0.5, pr, p.Corner)
			cov := clamp01(0.5 - d)
			if cov <= 0 {
				continue
			}
			inner := clamp01(0.5 - (d + bw))
			bcov := cov - inner
			px := buf[y*W+x]
			px = over(px, scaleColor(panelFill, cov))
			if bcov > 0 {
				px = over(px, scaleColor(border, bcov))
			}
			buf[y*W+x] = px
		}
	}
}

func mixByte(a, b uint32, k float64) uint32 {
	v := float64(a)*(1-k) + float64(b)*k + 0.5
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint32(v)
}

func drawGlassPanel(buf []uint32, W, H int, p Params, pr Rect, st Style) {
	// Couleur de base quand on n'a pas de décor : gris moyen.
	baseR, baseG, baseB := uint32(150), uint32(152), uint32(160)
	if st.Dark {
		baseR, baseG, baseB = 70, 72, 80
	}
	// Teinte : blanc translucide en clair, noir-gris translucide en sombre.
	tr, tg, tb := uint32(255), uint32(255), uint32(255)
	tint := 0.34
	borderA := 0.60
	if st.Dark {
		tr, tg, tb = 18, 18, 22
		tint = 0.40
		borderA = 0.26
	}
	bw := math.Max(1, p.Corner/18)
	for y := int(pr.Y) - 1; y <= int(pr.Y+pr.H)+1; y++ {
		if y < 0 || y >= H {
			continue
		}
		for x := int(pr.X) - 1; x <= int(pr.X+pr.W)+1; x++ {
			if x < 0 || x >= W {
				continue
			}
			d := sdRoundRect(float64(x)+0.5, float64(y)+0.5, pr, p.Corner)
			cov := clamp01(0.5 - d)
			if cov <= 0 {
				continue
			}
			t := st.Orient.near(float64(x), float64(y), pr) // 0 loin du bord de l'écran, 1 contre lui
			r, g, b := baseR, baseG, baseB
			if len(st.Backdrop) == W*H {
				q := st.Backdrop[y*W+x]
				r, g, b = (q>>16)&255, (q>>8)&255, q&255
			}
			k := tint + 0.07*(1-t) // un peu plus clair en haut
			r, g, b = mixByte(r, tr, k), mixByte(g, tg, k), mixByte(b, tb, k)
			px := scaleColor(0xFF000000|r<<16|g<<8|b, cov)

			inner := clamp01(0.5 - (d + bw))
			if bcov := cov - inner; bcov > 0 {
				// le liseré est plus lumineux sur le haut de la barre
				a := borderA * (0.55 + 0.45*(1-t))
				px = over(px, scaleColor(premul(255, 255, 255, a), bcov))
			}
			buf[y*W+x] = px
		}
	}
}

func drawItems(buf []uint32, W, H int, p Params, items []Item, l Layout, o Orient, we, he float64) {
	pr := l.Panel
	dotColor := premul(255, 255, 255, 0.92)
	sepColor := premul(255, 255, 255, 0.30)
	for i, it := range items {
		r := l.Items[i]
		if it.Separator {
			ex := r.X + r.W/2
			for ey := r.Y + r.H*0.12; ey < r.Y+r.H*0.88; ey++ {
				wx, wy := o.FromEngine(ex, ey, we, he)
				x, y := int(wx), int(wy)
				if x >= 0 && x < W && y >= 0 && y < H {
					buf[y*W+x] = over(buf[y*W+x], sepColor)
				}
			}
			continue
		}
		sr := r
		sr.Y -= it.Lift
		if it.Shadow != nil {
			sh := sr
			sh.Y += sr.H * 0.045
			drawSprite(buf, W, H, it.Shadow, o.Rect(sh, we, he))
		}
		drawSprite(buf, W, H, it.Sprite, o.Rect(sr, we, he))
		if it.Running {
			cy := pr.Y + pr.H - p.PadY - p.DotH/2 + p.DotH*0.12
			wx, wy := o.FromEngine(r.X+r.W/2, cy, we, he)
			drawDisc(buf, W, H, wx, wy, math.Max(1.8, p.Base/22), dotColor)
		}
	}
}

// MakeShadow fabrique l'ombre douce d'une icône : sa silhouette en noir translucide, floutée.
func MakeShadow(s *Sprite) *Sprite {
	if s == nil || s.W == 0 || s.H == 0 {
		return nil
	}
	w, h := s.W, s.H
	a := make([]float32, w*h)
	for i, p := range s.Pix {
		a[i] = float32(p>>24) / 255 * 0.42
	}
	rad := w / 36
	if rad < 1 {
		rad = 1
	}
	tmp := make([]float32, w*h)
	for pass := 0; pass < 2; pass++ {
		blur1(a, tmp, w, h, rad, true)
		blur1(tmp, a, w, h, rad, false)
	}
	out := &Sprite{W: w, H: h, Pix: make([]uint32, w*h)}
	for i, v := range a {
		out.Pix[i] = uint32(clamp01(float64(v))*255+0.5) << 24
	}
	return out
}

func blur1(src, dst []float32, w, h, rad int, horizontal bool) {
	lines, length := h, w
	if !horizontal {
		lines, length = w, h
	}
	at := func(l, i int) int {
		if horizontal {
			return l*w + i
		}
		return i*w + l
	}
	n := float32(2*rad + 1)
	for l := 0; l < lines; l++ {
		var sum float32
		for i := -rad; i <= rad; i++ {
			j := i
			if j < 0 || j >= length {
				continue
			}
			sum += src[at(l, j)]
		}
		for i := 0; i < length; i++ {
			dst[at(l, i)] = sum / n
			if j := i + rad + 1; j < length {
				sum += src[at(l, j)]
			}
			if j := i - rad; j >= 0 {
				sum -= src[at(l, j)]
			}
		}
	}
}

// Plate pose une icône sur une plaque en « carré arrondi » (superellipse), comme les
// icônes de macOS. Une icône qui remplit déjà son carré est seulement découpée à cette forme.
func Plate(s *Sprite, dark bool) *Sprite {
	if s == nil || s.W == 0 || s.H == 0 {
		return nil
	}
	n := s.W
	if s.H > n {
		n = s.H
	}
	if n < 96 {
		n = 96
	}
	// L'icône occupe-t-elle déjà tout son carré ?
	opaque := 0
	for _, p := range s.Pix {
		if p>>24 > 200 {
			opaque++
		}
	}
	full := float64(opaque)/float64(len(s.Pix)) > 0.78

	out := &Sprite{W: n, H: n, Pix: make([]uint32, n*n)}
	var inner *Sprite
	if full {
		inner = ResizeArea(s, n, n)
	} else {
		d := int(float64(n) * 0.64)
		iw, ih := d, d
		if s.W > s.H {
			ih = d * s.H / s.W
		} else if s.H > s.W {
			iw = d * s.W / s.H
		}
		inner = ResizeArea(s, iw, ih)
	}
	topR, topG, topB := 252.0, 252.0, 254.0
	botR, botG, botB := 222.0, 225.0, 232.0
	if dark {
		topR, topG, topB = 236, 238, 244
		botR, botG, botB = 196, 200, 210
	}
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			u := (float64(x)+0.5)/float64(n)*2 - 1
			v := (float64(y)+0.5)/float64(n)*2 - 1
			const pw = 5.0
			f := math.Pow(math.Pow(math.Abs(u), pw)+math.Pow(math.Abs(v), pw), 1/pw)
			cov := clamp01((1-f)*float64(n)/2 + 0.5)
			if cov <= 0 {
				continue
			}
			var px uint32
			if full {
				px = inner.Pix[y*n+x]
			} else {
				t := float64(y) / float64(n)
				r := uint32(topR + (botR-topR)*t)
				g := uint32(topG + (botG-topG)*t)
				b := uint32(topB + (botB-topB)*t)
				px = 0xFF000000 | r<<16 | g<<8 | b
				ox, oy := x-(n-inner.W)/2, y-(n-inner.H)/2
				if ox >= 0 && oy >= 0 && ox < inner.W && oy < inner.H {
					px = over(px, inner.Pix[oy*inner.W+ox])
				}
			}
			out.Pix[y*n+x] = scaleColor(px, cov)
		}
	}
	return out
}
