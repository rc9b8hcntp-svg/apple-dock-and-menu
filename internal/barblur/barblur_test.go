package barblur

import "testing"

func TestBlurUniform(t *testing.T) {
	w, h := 50, 20
	src := make([]uint32, w*h)
	for i := range src {
		src[i] = 0xFF204060
	}
	out := Blur(src, w, h, 4, 2)
	for i, p := range out {
		if p != 0xFF204060 {
			t.Fatalf("pixel %d = %08x", i, p)
		}
	}
}

func TestBlurSmooths(t *testing.T) {
	w, h := 40, 16
	src := make([]uint32, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if x >= w/2 {
				src[y*w+x] = 0xFFFFFFFF
			} else {
				src[y*w+x] = 0xFF000000
			}
		}
	}
	out := Blur(src, w, h, 4, 2)
	mid := out[(h/2)*w+w/2] & 255
	if mid == 0 || mid == 255 {
		t.Fatalf("pas de dégradé au bord : %d", mid)
	}
	if len(out) != w*h {
		t.Fatal("taille")
	}
}

func TestBlurOdd(t *testing.T) {
	if out := Blur(make([]uint32, 7*3), 7, 3, 4, 2); len(out) != 21 {
		t.Fatal("taille")
	}
	if out := Blur(nil, 0, 0, 4, 2); len(out) != 0 {
		t.Fatal("vide")
	}
}

func TestTint(t *testing.T) {
	px := []uint32{0xFF000000, 0xFFFFFFFF}
	Tint(px, 100, 150, 200, 1)
	if px[0] != 0xFF6496C8 || px[1] != 0xFF6496C8 {
		t.Fatalf("%08x %08x", px[0], px[1])
	}
	px = []uint32{0xFF000000}
	Tint(px, 200, 200, 200, 0.5)
	if px[0] != 0xFF646464 {
		t.Fatalf("%08x", px[0])
	}
}

func TestTop(t *testing.T) {
	if len(Top(make([]uint32, 30), 10, 2)) != 20 {
		t.Fatal("top")
	}
	if len(Top(make([]uint32, 30), 10, 9)) != 30 {
		t.Fatal("top trop grand")
	}
}
