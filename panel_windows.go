//go:build windows

package main

// Outils communs aux panneaux flottants (Spotlight, Centre de contrôle) :
// polices, icônes en couleur, pastilles à glyphe, presse-papiers.

import (
	"syscall"
	"unsafe"

	"menuwin/internal/dockgfx"
)

var (
	msimg32     = syscall.NewLazyDLL("msimg32.dll")
	pAlphaBlend = msimg32.NewProc("AlphaBlend")

	pOpenClipboard    = user32.NewProc("OpenClipboard")
	pCloseClipboard   = user32.NewProc("CloseClipboard")
	pEmptyClipboard   = user32.NewProc("EmptyClipboard")
	pSetClipboardData = user32.NewProc("SetClipboardData")
	pGlobalAlloc      = kernel32.NewProc("GlobalAlloc")
	pGlobalLock       = kernel32.NewProc("GlobalLock")
	pGlobalUnlock     = kernel32.NewProc("GlobalUnlock")

	panelFont struct {
		big, row, small, glyph, glyphBig uintptr
	}
)

// initPanelFonts crée (une seule fois) les polices des panneaux.
func initPanelFonts() {
	if panelFont.big != 0 {
		return
	}
	face := func(px int32, name string) uintptr {
		f, _, _ := pCreateFont.Call(uintptr(-px), 0, 0, 0, 400, 0, 0, 0,
			1 /*DEFAULT_CHARSET*/, 0, 0, 5 /*CLEARTYPE*/, 0, ptr(utf16(name)))
		return f
	}
	panelFont.big = makeFont(sc(24), 300)
	panelFont.row = makeFont(sc(15), 400)
	panelFont.small = makeFont(sc(12), 400)
	panelFont.glyph = face(sc(16), "Segoe MDL2 Assets")
	panelFont.glyphBig = face(sc(22), "Segoe MDL2 Assets")
}

// drawGlyphIn écrit un glyphe d'icône centré dans r avec la police donnée.
func drawGlyphIn(hdc, font uintptr, glyph string, r rect, col uintptr) {
	pSelectObject.Call(hdc, font)
	pSetTextColor.Call(hdc, col)
	drawText(hdc, glyph, r, dtCenter)
	pSelectObject.Call(hdc, fontReg)
}

// glyphDisc dessine une pastille ronde de couleur bg avec un glyphe blanc.
func glyphDisc(hdc uintptr, r rect, glyph string, bg, fg uintptr) {
	fillEllipse(hdc, r, bg)
	drawGlyphIn(hdc, panelFont.glyph, glyph, r, fg)
}

// blitSprite dessine une icône (pixels prémultipliés) de côté size à l'endroit voulu.
func blitSprite(hdc uintptr, s *dockgfx.Sprite, x, y, size int32) {
	if s == nil || s.W == 0 || s.H == 0 || pAlphaBlend.Find() != nil {
		return
	}
	if int32(s.W) > size {
		if r := dockgfx.ResizeArea(s, int(size), int(size)*s.H/s.W); r != nil {
			s = r
		}
	}
	dc, bmp, old, bits := newDIB(int32(s.W), int32(s.H))
	if bits == nil {
		return
	}
	defer freeDIBParts(dc, bmp, old)
	copy(bits, s.Pix)
	h := size * int32(s.H) / int32(s.W)
	// BLENDFUNCTION : AC_SRC_OVER, 0, 255, AC_SRC_ALPHA
	pAlphaBlend.Call(hdc, uintptr(x), uintptr(y+(size-h)/2), uintptr(size), uintptr(h),
		dc, 0, 0, uintptr(s.W), uintptr(s.H), 0x01FF0000)
}

// setClipboardText copie du texte dans le presse-papiers.
func setClipboardText(s string) {
	u, err := syscall.UTF16FromString(s)
	if err != nil {
		return
	}
	if r, _, _ := pOpenClipboard.Call(barHwnd); r == 0 {
		return
	}
	defer pCloseClipboard.Call()
	pEmptyClipboard.Call()
	mem, _, _ := pGlobalAlloc.Call(2 /*GMEM_MOVEABLE*/, uintptr(len(u)*2))
	if mem == 0 {
		return
	}
	p, _, _ := pGlobalLock.Call(mem)
	if p == 0 {
		return
	}
	copy(unsafe.Slice((*uint16)(at(p)), len(u)), u)
	pGlobalUnlock.Call(mem)
	pSetClipboardData.Call(13 /*CF_UNICODETEXT*/, mem)
}

// cardColor : le fond des « cartes » (tuiles, lignes) des panneaux.
func cardColor() uintptr { return blendColor(pal.menuBg, pal.menuText, 0.08) }
