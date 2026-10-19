package dockgfx

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"testing"
)

// TestPreview écrit une image du Dock dans le fichier donné par DOCK_PREVIEW (outil de mise au point).
func TestPreview(t *testing.T) {
	out := os.Getenv("DOCK_PREVIEW")
	if out == "" {
		t.Skip("DOCK_PREVIEW non défini")
	}
	for _, cfgv := range []struct {
		dark bool
		o    Orient
		name string
	}{{false, Bottom, "light"}, {true, Bottom, "dark"}, {false, Left, "left"}, {true, Right, "right"}} {
		dark, orient := cfgv.dark, cfgv.o
		scale := 1.5
		p := NewParams(scale)
		mk := func(r, g, b uint8, round bool) *Sprite {
			n := 128
			s := &Sprite{W: n, H: n, Pix: make([]uint32, n*n)}
			for y := 0; y < n; y++ {
				for x := 0; x < n; x++ {
					if round {
						d := math.Hypot(float64(x)-63.5, float64(y)-63.5)
						if d > 40 {
							continue
						}
					}
					k := float64(y) / float64(n)
					s.Pix[y*n+x] = premul(uint8(float64(r)*(1-0.3*k)), uint8(float64(g)*(1-0.3*k)), uint8(float64(b)*(1-0.3*k)), 1)
				}
			}
			return s
		}
		var items []Item
		cols := [][3]uint8{{40, 120, 240}, {240, 90, 60}, {60, 200, 110}, {250, 200, 50}, {150, 90, 230}, {90, 90, 100}}
		for i, c := range cols {
			s := mk(c[0], c[1], c[2], i%2 == 1)
			if i == 1 || i == 3 {
				s = Plate(s, dark)
			}
			items = append(items, Item{Sprite: s, Shadow: MakeShadow(s), Running: i < 3})
		}
		items = append(items, Item{Separator: true})
		items = append(items, Item{Sprite: mk(200, 200, 205, false), Shadow: MakeShadow(mk(200, 200, 205, false))})
		we, he := p.WindowW(items), p.WindowH()
		W, H := orient.WindowSize(we, he)
		l := Compute(p, items, we, he, float64(we)/2-30, 1)
		// décor : dégradé de couleurs, puis flou léger (comme la capture)
		bd := make([]uint32, W*H)
		for y := 0; y < H; y++ {
			for x := 0; x < W; x++ {
				r := uint32(60 + 120*float64(x)/float64(W))
				g := uint32(90 + 80*float64(y)/float64(H))
				b := uint32(190 - 60*float64(x)/float64(W))
				bd[y*W+x] = 0xFF000000 | r<<16 | g<<8 | b
			}
		}
		buf := make([]uint32, W*H)
		RenderStyled(buf, W, H, p, items, l, 2, Style{Glass: true, Dark: dark, Backdrop: bd, Orient: orient})
		img := image.NewRGBA(image.Rect(0, 0, W, H))
		for y := 0; y < H; y++ {
			for x := 0; x < W; x++ {
				b := bd[y*W+x]
				px := buf[y*W+x]
				a := px >> 24
				// le décor réel est plus sombre autour de la barre : on pose le décor brut dessous
				r := (px>>16)&255 + ((b>>16)&255)*(255-a)/255
				g := (px>>8)&255 + ((b>>8)&255)*(255-a)/255
				bl := px&255 + (b&255)*(255-a)/255
				img.Set(x, y, color.RGBA{uint8(r), uint8(g), uint8(bl), 255})
			}
		}
		name := out + "-" + cfgv.name + ".png"
		f, _ := os.Create(name)
		png.Encode(f, img)
		f.Close()
	}
}
