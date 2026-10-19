package dockgfx

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"testing"
)

// fakeIcon dessine une icône de test : un carré arrondi de couleur.
func fakeIcon(c color.NRGBA) *Sprite {
	const n = 128
	pix := make([]byte, n*n*4)
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			r := Rect{4, 4, n - 8, n - 8}
			d := sdRoundRect(float64(x)+0.5, float64(y)+0.5, r, 26)
			a := clamp01(0.5 - d)
			// un reflet clair en haut pour vérifier le dégradé
			k := 1 + 0.35*(1-float64(y)/n)
			i := (y*n + x) * 4
			pix[i] = uint8(math.Min(255, float64(c.B)*k))
			pix[i+1] = uint8(math.Min(255, float64(c.G)*k))
			pix[i+2] = uint8(math.Min(255, float64(c.R)*k))
			pix[i+3] = uint8(a * 255)
		}
	}
	return FromBGRA(n, n, pix, false)
}

func testItems() []Item {
	cols := []color.NRGBA{
		{232, 180, 60, 255}, {59, 130, 214, 255}, {120, 120, 128, 255},
		{217, 96, 76, 255}, {74, 165, 106, 255}, {150, 100, 200, 255},
	}
	var items []Item
	for i, c := range cols {
		items = append(items, Item{Sprite: fakeIcon(c), Running: i == 1 || i == 3})
	}
	items = append(items, Item{Separator: true})
	items = append(items, Item{Sprite: fakeIcon(color.NRGBA{200, 205, 212, 255})})
	return items
}

func TestRestLayoutIsUniform(t *testing.T) {
	p := NewParams(1)
	items := testItems()
	w, h := p.WindowW(items), p.WindowH()
	l := Compute(p, items, w, h, 0, 0)
	for i, r := range l.Items {
		if items[i].Separator {
			continue
		}
		if math.Abs(r.W-p.Base) > 1e-9 {
			t.Fatalf("item %d: taille %v, attendu %v au repos", i, r.W, p.Base)
		}
	}
	// la barre est centrée
	if math.Abs(l.Panel.X+l.Panel.W/2-float64(w)/2) > 0.5 {
		t.Fatalf("barre non centrée : %+v dans une fenêtre de %d", l.Panel, w)
	}
}

func TestMagnificationPeaksUnderMouse(t *testing.T) {
	p := NewParams(1)
	items := testItems()
	w, h := p.WindowW(items), p.WindowH()
	rest := Compute(p, items, w, h, 0, 0)
	target := rest.Items[2]
	mx := target.X + target.W/2
	l := Compute(p, items, w, h, mx, 1)

	if got := l.Items[2].W; math.Abs(got-p.Max) > p.Max*0.03 {
		t.Fatalf("l'icône sous la souris mesure %.1f, attendu ≈ %.1f", got, p.Max)
	}
	if !(l.Items[2].W > l.Items[1].W && l.Items[1].W > l.Items[0].W) {
		t.Fatalf("le zoom doit décroître avec la distance : %.1f %.1f %.1f", l.Items[0].W, l.Items[1].W, l.Items[2].W)
	}
	// les icônes restent posées sur la même ligne de base
	base := l.Items[2].Y + l.Items[2].H
	for i, r := range l.Items {
		if items[i].Separator {
			continue
		}
		if math.Abs(r.Y+r.H-base) > 1e-6 {
			t.Fatalf("item %d ne repose pas sur la ligne de base", i)
		}
	}
	// tout tient dans la fenêtre, même au zoom maximal
	for i, r := range l.Items {
		if r.X < 0 || r.X+r.W > float64(w) || r.Y < 0 {
			t.Fatalf("item %d sort de la fenêtre : %+v (fenêtre %dx%d)", i, r, w, h)
		}
	}
	if l.Panel.X < 0 || l.Panel.X+l.Panel.W > float64(w) {
		t.Fatalf("la barre sort de la fenêtre : %+v (largeur %d)", l.Panel, w)
	}
}

func TestZoomFitsAtEveryPosition(t *testing.T) {
	for _, scale := range []float64{1, 1.5, 2} {
		p := NewParams(scale)
		items := testItems()
		w, h := p.WindowW(items), p.WindowH()
		for mx := 0.0; mx <= float64(w); mx += 3 {
			l := Compute(p, items, w, h, mx, 1)
			if l.Panel.X < -0.5 || l.Panel.X+l.Panel.W > float64(w)+0.5 {
				t.Fatalf("échelle %v, souris %v : barre hors fenêtre %+v (largeur %d)", scale, mx, l.Panel, w)
			}
			for i, r := range l.Items {
				if r.Y < -0.5 {
					t.Fatalf("échelle %v, souris %v : item %d dépasse en haut (%v)", scale, mx, i, r.Y)
				}
			}
		}
	}
}

func TestHitTest(t *testing.T) {
	p := NewParams(1)
	items := testItems()
	w, h := p.WindowW(items), p.WindowH()
	l := Compute(p, items, w, h, 0, 0)
	for i, r := range l.Items {
		got := HitTest(p, items, l, r.X+r.W/2, r.Y+r.H/2)
		want := i
		if items[i].Separator {
			want = -1
		}
		if got != want {
			t.Fatalf("clic au centre de l'item %d : trouvé %d", i, got)
		}
	}
	if HitTest(p, items, l, 2, 2) != -1 {
		t.Fatal("un clic dans le coin de la fenêtre ne doit rien toucher")
	}
}

func TestResizeAreaKeepsColor(t *testing.T) {
	s := &Sprite{W: 4, H: 4, Pix: make([]uint32, 16)}
	for i := range s.Pix {
		s.Pix[i] = 0xFF204080
	}
	o := ResizeArea(s, 3, 3)
	for _, p := range o.Pix {
		if p != 0xFF204080 {
			t.Fatalf("la couleur change au redimensionnement : %08x", p)
		}
	}
}

// DOCK_PNG=/chemin go test ./internal/dockgfx -run TestPNG écrit des images
// pour contrôler le rendu à l'œil.
func TestPNG(t *testing.T) {
	dir := os.Getenv("DOCK_PNG")
	if dir == "" {
		t.Skip("DOCK_PNG non défini")
	}
	for name, spec := range map[string]struct {
		scale, frac, strength float64
	}{
		"dock-repos":  {1.25, 0, 0},
		"dock-zoom":   {1.25, 0.30, 1},
		"dock-zoom-b": {1.25, 0.62, 1},
	} {
		p := NewParams(spec.scale)
		items := testItems()
		w, h := p.WindowW(items), p.WindowH()
		rest := Compute(p, items, w, h, 0, 0)
		mx := rest.Panel.X + rest.Panel.W*spec.frac
		l := Compute(p, items, w, h, mx, spec.strength)
		buf := make([]uint32, w*h)
		Render(buf, w, h, p, items, l, 2)

		img := image.NewNRGBA(image.Rect(0, 0, w, h))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				// fond de bureau factice : dégradé bleu-gris
				bg := [3]float64{70 + 60*float64(x)/float64(w), 100 + 60*float64(y)/float64(h), 150}
				px := buf[y*w+x]
				a := float64(px>>24) / 255
				r := float64((px>>16)&255) + bg[0]*(1-a)
				g := float64((px>>8)&255) + bg[1]*(1-a)
				b := float64(px&255) + bg[2]*(1-a)
				img.SetNRGBA(x, y, color.NRGBA{uint8(math.Min(r, 255)), uint8(math.Min(g, 255)), uint8(math.Min(b, 255)), 255})
			}
		}
		f, err := os.Create(dir + "/" + name + ".png")
		if err != nil {
			t.Fatal(err)
		}
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
		f.Close()
	}
}

func TestOrientRoundTrip(t *testing.T) {
	we, he := 600.0, 90.0
	for _, o := range []Orient{Bottom, Left, Right} {
		for _, pt := range [][2]float64{{10, 20}, {599, 89}, {300, 45}} {
			wx, wy := o.FromEngine(pt[0], pt[1], we, he)
			ex, ey := o.ToEngine(wx, wy, we, he)
			if ex != pt[0] || ey != pt[1] {
				t.Fatalf("orient %d: (%v,%v) -> (%v,%v) -> (%v,%v)", o, pt[0], pt[1], wx, wy, ex, ey)
			}
		}
		w, h := o.WindowSize(int(we), int(he))
		ew, eh := o.Engine(w, h)
		if ew != int(we) || eh != int(he) {
			t.Fatalf("orient %d: tailles %d×%d -> %d×%d", o, ew, eh, w, h)
		}
	}
}

func TestOrientRectInWindow(t *testing.T) {
	we, he := 600.0, 90.0
	r := Rect{100, 20, 48, 48}
	for _, o := range []Orient{Left, Right} {
		w, h := o.WindowSize(int(we), int(he))
		wr := o.Rect(r, we, he)
		if wr.W != r.W || wr.H != r.H {
			t.Fatalf("la taille de l'icône change : %+v", wr)
		}
		if wr.X < 0 || wr.Y < 0 || wr.X+wr.W > float64(w) || wr.Y+wr.H > float64(h) {
			t.Fatalf("orient %d : hors fenêtre %+v (%d×%d)", o, wr, w, h)
		}
	}
	// le bas du moteur est contre le bord : à gauche x petit, à droite x grand
	bottomIcon := Rect{100, he - 10, 48, 8}
	if lr := Left.Rect(bottomIcon, we, he); lr.X > 5 {
		t.Fatalf("à gauche l'icône du bas devrait coller au bord gauche : %+v", lr)
	}
	if rr := Right.Rect(bottomIcon, we, he); rr.X < he-12 {
		t.Fatalf("à droite l'icône du bas devrait coller au bord droit : %+v", rr)
	}
}

func TestRenderSides(t *testing.T) {
	p := NewParams(1)
	items := []Item{{Sprite: &Sprite{W: 2, H: 2, Pix: []uint32{0xFFFF0000, 0xFFFF0000, 0xFFFF0000, 0xFFFF0000}}, Running: true}, {Separator: true},
		{Sprite: &Sprite{W: 2, H: 2, Pix: []uint32{0xFF00FF00, 0xFF00FF00, 0xFF00FF00, 0xFF00FF00}}}}
	we, he := p.WindowW(items), p.WindowH()
	for _, o := range []Orient{Bottom, Left, Right} {
		w, h := o.WindowSize(we, he)
		l := Compute(p, items, we, he, float64(we)/2, 1)
		buf := make([]uint32, w*h)
		RenderStyled(buf, w, h, p, items, l, 2, Style{Glass: true, Orient: o})
		opaque := 0
		for _, px := range buf {
			if px>>24 > 200 {
				opaque++
			}
		}
		if opaque == 0 {
			t.Fatalf("orient %d : rien de dessiné", o)
		}
	}
}
