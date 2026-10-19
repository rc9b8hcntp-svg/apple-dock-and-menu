package dockgfx

import "testing"

func TestLiftMovesIconUp(t *testing.T) {
	p := NewParams(1)
	pix := make([]uint32, 48*48)
	for i := range pix {
		pix[i] = 0xFFFF0000
	}
	sprite := &Sprite{W: 48, H: 48, Pix: pix}
	top := func(lift float64) int {
		items := []Item{{Sprite: sprite, Lift: lift}}
		w, h := p.WindowW(items), p.WindowH()
		buf := make([]uint32, w*h)
		l := Compute(p, items, w, h, float64(w)/2, 0)
		Render(buf, w, h, p, items, l, 2)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				if buf[y*w+x]>>24 == 255 {
					return y
				}
			}
		}
		return -1
	}
	a, b := top(0), top(10)
	if a < 0 || b < 0 || a-b != 10 {
		t.Fatalf("haut de l'icône : %d puis %d, attendu un écart de 10", a, b)
	}
}
