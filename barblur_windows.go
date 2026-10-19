//go:build windows

package main

// Flou derrière la barre (réglage « Flou derrière la barre ») : on copie les pixels
// de l'écran sous la barre (sans la barre elle-même, qui est une fenêtre
// transparente : BitBlt sans CAPTUREBLT l'ignore), on les floute, on les mélange avec
// la couleur de la barre, et drawBar peint ce résultat au lieu d'un aplat.

import (
	"slices"
	"unsafe"

	"menuwin/internal/barblur"
)

var bb struct {
	dc, bmp, old uintptr
	bits         []uint32
	w, h         int32
	key          [4]uintptr // couleur, opacité, largeur, hauteur de la dernière composition
	last         []uint32
}

func barBlurFree() {
	freeDIBParts(bb.dc, bb.bmp, bb.old)
	bb.dc, bb.bmp, bb.old, bb.bits, bb.last = 0, 0, 0, nil, nil
	bb.key = [4]uintptr{}
}

// barBlurCompose refait l'image floutée ; renvoie true si elle a changé.
func barBlurCompose(w, h int32) bool {
	if barHwnd == 0 || w <= 0 || h <= 0 {
		return false
	}
	var wr rect
	pGetWindowRect.Call(barHwnd, uintptr(unsafe.Pointer(&wr)))
	sh, _, _ := pGetSystemMetrics.Call(smCyScreen)
	ch := h * 4 // on capte plus bas que la barre pour que le flou vertical ait de la matière
	if wr.Top+ch > int32(sh) {
		ch = int32(sh) - wr.Top
	}
	if ch < h {
		ch = h
	}
	cdc, cbmp, cold, cbits := newDIB(w, ch)
	if cbits == nil {
		return false
	}
	defer freeDIBParts(cdc, cbmp, cold)
	screen, _, _ := pGetDC.Call(0)
	defer pReleaseDC.Call(0, screen)
	if ok, _, _ := pBitBlt.Call(cdc, 0, 0, uintptr(w), uintptr(ch), screen, uintptr(wr.Left), uintptr(wr.Top), srcCopy); ok == 0 {
		return false
	}
	for i := range cbits {
		cbits[i] |= 0xFF000000
	}
	blurred := barblur.Blur(cbits, int(w), int(ch), 4, 3)
	px := slices.Clone(barblur.Top(blurred, int(w), int(h)))
	// Teinte : la couleur de la barre ; 80 % au maximum pour que le flou reste visible.
	c := pal.bar
	k := float64(cfg.BarOpacity) / 100 * 0.8
	barblur.Tint(px, uint8(c&0xFF), uint8((c>>8)&0xFF), uint8((c>>16)&0xFF), k)

	if bb.dc == 0 || bb.w != w || bb.h != h {
		freeDIBParts(bb.dc, bb.bmp, bb.old)
		bb.dc, bb.bmp, bb.old, bb.bits = newDIB(w, h)
		bb.w, bb.h = w, h
		bb.last = nil
	}
	if bb.bits == nil {
		bb.dc = 0
		return false
	}
	if slices.Equal(bb.last, px) {
		return false
	}
	copy(bb.bits, px)
	bb.last = px
	bb.key = [4]uintptr{c, uintptr(cfg.BarOpacity), uintptr(w), uintptr(h)}
	return true
}

// barBlurPaint peint le fond flouté ; renvoie false s'il faut se rabattre sur l'aplat.
func barBlurPaint(hdc uintptr, w, h int32) bool {
	if !cfg.BarBlur {
		return false
	}
	key := [4]uintptr{pal.bar, uintptr(cfg.BarOpacity), uintptr(w), uintptr(h)}
	if bb.dc == 0 || bb.key != key {
		bb.last = nil
		barBlurCompose(w, h)
	}
	if bb.dc == 0 || bb.bits == nil {
		return false
	}
	pBitBlt.Call(hdc, 0, 0, uintptr(w), uintptr(h), bb.dc, 0, 0, srcCopy)
	return true
}

// barBlurTick est appelé par la minuterie : le fond a-t-il changé (fond d'écran, fenêtre déplacée) ?
func barBlurTick() bool {
	if !cfg.BarBlur {
		if bb.dc != 0 {
			barBlurFree()
		}
		return false
	}
	var cr rect
	pGetClientRect.Call(barHwnd, uintptr(unsafe.Pointer(&cr)))
	return barBlurCompose(cr.Right, cr.Bottom)
}
