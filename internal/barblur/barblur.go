// Package barblur : flou et teinte du fond de la barre des menus (pur Go, sans Windows).
package barblur

// Pixels : 0xAARRGGBB, lignes l'une après l'autre.

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// Blur renvoie une copie floutée de src (w×h) : réduction par blocs de f pixels,
// deux passes de flou en boîte de rayon rad, puis agrandissement bilinéaire.
func Blur(src []uint32, w, h, f, rad int) []uint32 {
	out := make([]uint32, w*h)
	if w <= 0 || h <= 0 || len(src) < w*h || f < 1 {
		return out
	}
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
	for pass := 0; pass < 2; pass++ {
		box(small, tmp, sw, sh, rad, true)
		box(tmp, small, sw, sh, rad, false)
	}
	ff := float64(f)
	for y := 0; y < h; y++ {
		v := (float64(y)+0.5)/ff - 0.5
		y0 := int(v)
		if v < 0 {
			y0 = -1
		}
		fy := float32(v - float64(y0))
		ya, yb := clamp(y0, 0, sh-1), clamp(y0+1, 0, sh-1)
		for x := 0; x < w; x++ {
			u := (float64(x)+0.5)/ff - 0.5
			x0 := int(u)
			if u < 0 {
				x0 = -1
			}
			fx := float32(u - float64(x0))
			xa, xb := clamp(x0, 0, sw-1), clamp(x0+1, 0, sw-1)
			var c [3]uint32
			for ch := 0; ch < 3; ch++ {
				top := small[ya*sw+xa][ch]*(1-fx) + small[ya*sw+xb][ch]*fx
				bot := small[yb*sw+xa][ch]*(1-fx) + small[yb*sw+xb][ch]*fx
				c[ch] = uint32(clamp(int((top*(1-fy)+bot*fy)+0.5), 0, 255))
			}
			out[y*w+x] = 0xFF000000 | c[0]<<16 | c[1]<<8 | c[2]
		}
	}
	return out
}

func box(src, dst [][3]float32, w, h, rad int, horizontal bool) {
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
			p := src[at(l, clamp(i, 0, length-1))]
			sum[0] += p[0]
			sum[1] += p[1]
			sum[2] += p[2]
		}
		for i := 0; i < length; i++ {
			dst[at(l, i)] = [3]float32{sum[0] / n, sum[1] / n, sum[2] / n}
			add := src[at(l, clamp(i+rad+1, 0, length-1))]
			sub := src[at(l, clamp(i-rad, 0, length-1))]
			for c := 0; c < 3; c++ {
				sum[c] += add[c] - sub[c]
			}
		}
	}
}

// Tint mélange chaque pixel avec la couleur (r, g, b) : k = 0 laisse l'image, k = 1 donne la couleur.
func Tint(px []uint32, r, g, b uint8, k float64) {
	if k < 0 {
		k = 0
	}
	if k > 1 {
		k = 1
	}
	mix := func(a uint32, t uint8) uint32 {
		return uint32(float64(a)*(1-k) + float64(t)*k + 0.5)
	}
	for i, p := range px {
		px[i] = 0xFF000000 | mix((p>>16)&255, r)<<16 | mix((p>>8)&255, g)<<8 | mix(p&255, b)
	}
}

// Top renvoie les h premières lignes d'une image de largeur w.
func Top(px []uint32, w, h int) []uint32 {
	if h*w > len(px) {
		h = len(px) / w
	}
	return px[:w*h]
}
