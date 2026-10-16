//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

// Formes arrondies lissées (anti-crénelage) avec GDI+. Seuls des nombres entiers
// sont passés à GDI+ : un rectangle arrondi est donc composé de quatre disques
// et de deux rectangles. Si GDI+ est absent, on retombe sur les régions de GDI
// (même forme, bords crénelés).

var (
	gdiplus               = syscall.NewLazyDLL("gdiplus.dll")
	pGdiplusStartup       = gdiplus.NewProc("GdiplusStartup")
	pGdiplusShutdown      = gdiplus.NewProc("GdiplusShutdown")
	pGdipCreateFromHDC    = gdiplus.NewProc("GdipCreateFromHDC")
	pGdipDeleteGraphics   = gdiplus.NewProc("GdipDeleteGraphics")
	pGdipSetSmoothingMode = gdiplus.NewProc("GdipSetSmoothingMode")
	pGdipCreateSolidFill  = gdiplus.NewProc("GdipCreateSolidFill")
	pGdipDeleteBrush      = gdiplus.NewProc("GdipDeleteBrush")
	pGdipFillEllipseI     = gdiplus.NewProc("GdipFillEllipseI")
	pGdipFillRectangleI   = gdiplus.NewProc("GdipFillRectangleI")
	pCreateEllipticRgn    = gdi32.NewProc("CreateEllipticRgn")

	gdipToken uintptr
	gdipOK    bool
)

type gdiplusStartupInput struct {
	Version                  uint32
	DebugEventCallback       uintptr
	SuppressBackgroundThread int32
	SuppressExternalCodecs   int32
}

func initGDIPlus() {
	in := gdiplusStartupInput{Version: 1}
	r, _, _ := pGdiplusStartup.Call(uintptr(unsafe.Pointer(&gdipToken)), uintptr(unsafe.Pointer(&in)), 0)
	gdipOK = r == 0
}

func shutdownGDIPlus() {
	if gdipOK {
		pGdiplusShutdown.Call(gdipToken)
		gdipOK = false
	}
}

// colorrefToARGB convertit un COLORREF (0x00BBGGRR) en couleur GDI+ opaque.
func colorrefToARGB(c uintptr) uintptr {
	return uintptr(0xFF000000 | (uint32(c)&0xFF)<<16 | (uint32(c)>>8&0xFF)<<8 | uint32(c)>>16&0xFF)
}

// aaShapes ouvre un contexte GDI+ lissé sur hdc avec un pinceau uni et appelle draw.
func aaShapes(hdc, col uintptr, draw func(g, br uintptr)) bool {
	if !gdipOK {
		return false
	}
	var g uintptr
	if r, _, _ := pGdipCreateFromHDC.Call(hdc, uintptr(unsafe.Pointer(&g))); r != 0 || g == 0 {
		return false
	}
	defer pGdipDeleteGraphics.Call(g)
	pGdipSetSmoothingMode.Call(g, 4 /*SmoothingModeAntiAlias*/)
	var br uintptr
	if r, _, _ := pGdipCreateSolidFill.Call(colorrefToARGB(col), uintptr(unsafe.Pointer(&br))); r != 0 || br == 0 {
		return false
	}
	defer pGdipDeleteBrush.Call(br)
	draw(g, br)
	return true
}

// fillEllipse remplit l'ellipse inscrite dans r (droite et bas exclus).
func fillEllipse(hdc uintptr, r rect, col uintptr) {
	w, h := r.Right-r.Left, r.Bottom-r.Top
	if w <= 0 || h <= 0 {
		return
	}
	if aaShapes(hdc, col, func(g, br uintptr) {
		pGdipFillEllipseI.Call(g, br, uintptr(r.Left), uintptr(r.Top), uintptr(w), uintptr(h))
	}) {
		return
	}
	brush, _, _ := pCreateSolidBrush.Call(col)
	rgn, _, _ := pCreateEllipticRgn.Call(uintptr(r.Left), uintptr(r.Top), uintptr(r.Right), uintptr(r.Bottom))
	pFillRgn.Call(hdc, rgn, brush)
	pDeleteObject.Call(rgn)
	pDeleteObject.Call(brush)
}

// fillRR remplit un rectangle arrondi (droite et bas exclus) ; d est le diamètre des coins.
func fillRR(hdc uintptr, r rect, d int32, col uintptr) {
	w, h := r.Right-r.Left, r.Bottom-r.Top
	if w <= 0 || h <= 0 {
		return
	}
	if d > h {
		d = h
	}
	if d > w {
		d = w
	}
	if d < 2 {
		brush, _, _ := pCreateSolidBrush.Call(col)
		pFillRect.Call(hdc, uintptr(unsafe.Pointer(&r)), brush)
		pDeleteObject.Call(brush)
		return
	}
	half := d / 2
	if aaShapes(hdc, col, func(g, br uintptr) {
		fr := func(x, y, rw, rh int32) {
			pGdipFillRectangleI.Call(g, br, uintptr(x), uintptr(y), uintptr(rw), uintptr(rh))
		}
		fe := func(x, y int32) {
			pGdipFillEllipseI.Call(g, br, uintptr(x), uintptr(y), uintptr(d), uintptr(d))
		}
		fr(r.Left+half, r.Top, w-2*half, h)
		fr(r.Left, r.Top+half, w, h-2*half)
		fe(r.Left, r.Top)
		fe(r.Right-d, r.Top)
		fe(r.Left, r.Bottom-d)
		fe(r.Right-d, r.Bottom-d)
	}) {
		return
	}
	brush, _, _ := pCreateSolidBrush.Call(col)
	rgn, _, _ := pCreateRoundRgn.Call(uintptr(r.Left), uintptr(r.Top), uintptr(r.Right), uintptr(r.Bottom), uintptr(d), uintptr(d))
	pFillRgn.Call(hdc, rgn, brush)
	pDeleteObject.Call(rgn)
	pDeleteObject.Call(brush)
}
