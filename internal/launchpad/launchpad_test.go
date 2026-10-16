package launchpad

import (
	"reflect"
	"testing"
)

func TestGridBigScreen(t *testing.T) {
	g := NewGrid(1920, 1080, 1)
	if g.Cols != 7 || g.Rows != 5 || g.PerPage != 35 {
		t.Fatalf("grille inattendue: %dx%d", g.Cols, g.Rows)
	}
	if g.Pages(0) != 1 || g.Pages(35) != 1 || g.Pages(36) != 2 || g.Pages(100) != 3 {
		t.Error("nombre de pages incorrect")
	}
	if g.IconSize <= 0 || g.IconSize > 128 {
		t.Errorf("taille d'icône %v", g.IconSize)
	}
}

func TestGridSmallScreen(t *testing.T) {
	g := NewGrid(800, 600, 1.5)
	if g.Cols < 3 || g.Rows < 2 || g.IconSize <= 0 {
		t.Fatalf("petite grille invalide: %+v", g)
	}
}

func TestSlotHitTest(t *testing.T) {
	g := NewGrid(1920, 1080, 1)
	for s := 0; s < g.PerPage; s++ {
		ir := g.IconRect(s)
		if got := g.Slot(ir.X+ir.W/2, ir.Y+ir.H/2); got != s {
			t.Fatalf("emplacement %d: obtenu %d", s, got)
		}
	}
	if g.Slot(5, 5) != -1 {
		t.Error("un coin de l'écran ne doit rien toucher")
	}
	// Les blocs ne se chevauchent pas.
	for a := 0; a < g.PerPage; a++ {
		for b := a + 1; b < g.PerPage; b++ {
			ra, rb := g.Block(a), g.Block(b)
			if ra.X < rb.X+rb.W && rb.X < ra.X+ra.W && ra.Y < rb.Y+rb.H && rb.Y < ra.Y+ra.H {
				t.Fatalf("blocs %d et %d se chevauchent", a, b)
			}
		}
	}
	// Le champ de recherche ne touche pas la grille.
	if g.Search.Y+g.Search.H > g.GridY {
		t.Error("le champ de recherche empiète sur la grille")
	}
}

func TestDots(t *testing.T) {
	g := NewGrid(1920, 1080, 1)
	for i := 0; i < 4; i++ {
		x, y := g.DotCenter(i, 4)
		if got := g.DotAt(x, y, 4); got != i {
			t.Errorf("point %d: obtenu %d", i, got)
		}
	}
	if g.DotAt(0, 0, 4) != -1 || g.DotAt(960, g.DotsY, 1) != -1 {
		t.Error("point inattendu")
	}
}

func TestFilter(t *testing.T) {
	names := []string{"Calculatrice", "Bloc-notes", "Google Chrome", "Éditeur de texte", "Paint", "Chromium Notes"}
	cases := map[string][]int{
		"":        {0, 1, 2, 3, 4, 5},
		"calc":    {0},
		"CHROME":  {2},
		"editeur": {3},
		"notes":   {1, 5},
		"chro":    {2, 5},
		"zzz":     {},
		"hrome":   {2},
	}
	for q, want := range cases {
		got := Filter(names, q)
		if len(got) == 0 && len(want) == 0 {
			continue
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("Filter(%q) = %v, attendu %v", q, got, want)
		}
	}
}

func TestBlurDim(t *testing.T) {
	const w, h = 37, 21
	src := make([]uint32, w*h)
	for i := range src {
		src[i] = 0xFF000000 | 200<<16 | 100<<8 | 50
	}
	out := BlurDim(src, w, h, 0.5)
	if len(out) != w*h {
		t.Fatalf("taille %d", len(out))
	}
	for _, p := range out {
		if p>>24 != 255 {
			t.Fatal("le fond doit être opaque")
		}
		r, g, b := (p>>16)&255, (p>>8)&255, p&255
		if abs(int(r)-100) > 1 || abs(int(g)-50) > 1 || abs(int(b)-25) > 1 {
			t.Fatalf("couleur inattendue %d,%d,%d", r, g, b)
		}
	}
	// Un bord net doit être adouci.
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if x < w/2 {
				src[y*w+x] = 0xFF000000
			} else {
				src[y*w+x] = 0xFFFFFFFF
			}
		}
	}
	out = BlurDim(src, w, h, 1)
	mid := out[(h/2)*w+w/2] & 255
	if mid <= 5 || mid >= 250 {
		t.Errorf("le bord n'est pas adouci : %d", mid)
	}
}

func TestPaintAndIcon(t *testing.T) {
	g := NewGrid(640, 400, 1)
	bg := make([]uint32, g.W*g.H)
	for i := range bg {
		bg[i] = 0xFF202020
	}
	buf := make([]uint32, g.W*g.H)
	ic := Icon(96)
	if ic.Pix[0]>>24 != 0 {
		t.Error("le coin de l'icône doit être transparent")
	}
	if ic.Pix[48*96+48]>>24 == 0 {
		t.Error("le centre de l'icône doit être opaque")
	}
	Paint(buf, bg, View{Grid: g, Tiles: []Tile{{Sprite: ic, Selected: true}, {Sprite: ic}}, Page: 0, Pages: 3})
	for _, p := range buf {
		if p>>24 != 255 {
			t.Fatal("l'image finale doit rester opaque")
		}
	}
	ir := g.IconRect(0)
	if buf[int(ir.Y+ir.H/2)*g.W+int(ir.X+ir.W/2)] == bg[0] {
		t.Error("l'icône n'a pas été dessinée")
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
