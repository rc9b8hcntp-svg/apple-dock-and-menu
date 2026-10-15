//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

// ---------------------------------------------------------------------------
// État de la barre
// ---------------------------------------------------------------------------

type winItem struct {
	hwnd  uintptr
	title string
	icon  uintptr
}

const (
	hitMenu = iota + 1
	hitApps
)

type hit struct {
	r    rect
	kind int
	hwnd uintptr
}

var (
	scale     = 1.0
	barH      int32
	barHwnd   uintptr
	appBarOK  bool
	ownPID    uintptr
	wins      []winItem
	active    uintptr
	hits      []hit
	hoverIdx  = -1
	tracking  bool
	pressed   int // hitMenu / hitApps pendant que le menu est ouvert
	clockText string

	fontReg, fontBold                 uintptr
	brushBar, brushHover, brushAccent uintptr
	brushSep, brushGlyph              uintptr
)

func sc(v float64) int32 { return int32(v*scale + 0.5) }

func initBarResources() {
	if pGetDpiForSystem.Find() == nil {
		if dpi, _, _ := pGetDpiForSystem.Call(); dpi >= 96 {
			scale = float64(dpi) / 96
		}
	}
	barH = sc(30)
	pid, _, _ := pGetCurrentPID.Call()
	ownPID = pid

	brushBar, _, _ = pCreateSolidBrush.Call(rgb(28, 28, 30))
	brushHover, _, _ = pCreateSolidBrush.Call(rgb(62, 62, 68))
	brushAccent, _, _ = pCreateSolidBrush.Call(rgb(77, 140, 240))
	brushSep, _, _ = pCreateSolidBrush.Call(rgb(70, 70, 76))
	brushGlyph, _, _ = pCreateSolidBrush.Call(rgb(245, 245, 247))

	mk := func(weight int) uintptr {
		h := -sc(14)
		f, _, _ := pCreateFont.Call(uintptr(h), 0, 0, 0, uintptr(weight), 0, 0, 0,
			1 /*DEFAULT_CHARSET*/, 0, 0, 5 /*CLEARTYPE*/, 0, ptr(utf16("Segoe UI")))
		return f
	}
	fontReg = mk(400)
	fontBold = mk(600)
}

func freeBarResources() {
	for _, o := range []uintptr{brushBar, brushHover, brushAccent, brushSep, brushGlyph, fontReg, fontBold} {
		pDeleteObject.Call(o)
	}
}

// ---------------------------------------------------------------------------
// Liste des fenêtres ouvertes
// ---------------------------------------------------------------------------

func windowIcon(h uintptr) uintptr {
	var res uintptr
	pSendMsgTimeout.Call(h, wmGetIcon, iconSmall2, 0, smtoAbort, 40, uintptr(unsafe.Pointer(&res)))
	if res != 0 {
		return res
	}
	idx := -34 // GCL_HICONSM
	r, _, _ := pGetClassLongPtr.Call(h, uintptr(idx))
	if r != 0 {
		return r
	}
	idx = -14 // GCL_HICON
	r, _, _ = pGetClassLongPtr.Call(h, uintptr(idx))
	return r
}

func listable(h uintptr) (string, bool) {
	if h == barHwnd {
		return "", false
	}
	if v, _, _ := pIsWindowVisible.Call(h); v == 0 {
		return "", false
	}
	ex := getLongPtr(h, -20) // GWL_EXSTYLE
	app := ex&wsExAppwindow != 0
	if owner, _, _ := pGetWindow.Call(h, gwOwner); owner != 0 && !app {
		return "", false
	}
	if ex&wsExToolwin != 0 && !app {
		return "", false
	}
	if ex&wsExNoActive != 0 && !app {
		return "", false
	}
	var cloaked uint32
	pDwmGetWindowAttr.Call(h, dwmaCloaked, uintptr(unsafe.Pointer(&cloaked)), 4)
	if cloaked != 0 {
		return "", false
	}
	n, _, _ := pGetWindowTextLen.Call(h)
	if n == 0 {
		return "", false
	}
	buf := make([]uint16, n+1)
	pGetWindowText.Call(h, uintptr(unsafe.Pointer(&buf[0])), n+1)
	title := syscall.UTF16ToString(buf)
	if title == "" || title == "Program Manager" {
		return "", false
	}
	return title, true
}

var enumProc = syscall.NewCallback(func(h, _ uintptr) uintptr {
	if title, ok := listable(h); ok {
		found = append(found, winItem{hwnd: h, title: title})
	}
	return 1 // continuer
})

var found []winItem

// refresh met à jour la liste des fenêtres. Renvoie true si quelque chose a changé.
func refresh() bool {
	found = found[:0]
	pEnumWindows.Call(enumProc, 0)

	// Garde l'ordre précédent pour que les boutons ne sautent pas.
	var next []winItem
	for _, old := range wins {
		for _, f := range found {
			if f.hwnd == old.hwnd {
				next = append(next, f)
				break
			}
		}
	}
	for _, f := range found {
		seen := false
		for _, n := range next {
			if n.hwnd == f.hwnd {
				seen = true
				break
			}
		}
		if !seen {
			next = append(next, f)
		}
	}

	changed := len(next) != len(wins)
	for i := range next {
		if !changed && next[i].hwnd == wins[i].hwnd && next[i].title == wins[i].title {
			next[i].icon = wins[i].icon
			continue
		}
		changed = true
		next[i].icon = windowIcon(next[i].hwnd)
	}
	wins = next

	fg, _, _ := pGetForeground.Call()
	newActive := active
	if fg != 0 {
		inList := false
		for _, w := range wins {
			if w.hwnd == fg {
				inList = true
				break
			}
		}
		var pid uint32
		pGetWindowThreadPI.Call(fg, uintptr(unsafe.Pointer(&pid)))
		switch {
		case inList:
			newActive = fg
		case uintptr(pid) == ownPID:
			// Notre propre menu est ouvert : on garde l'ancienne sélection.
		default:
			newActive = 0
		}
	}
	if newActive != active {
		active = newActive
		changed = true
	}
	return changed
}

func updateClock() bool {
	var d, t [64]uint16
	pGetDateFormat.Call(0x0400, 0, 0, ptr(utf16("ddd d MMM")), uintptr(unsafe.Pointer(&d[0])), 64)
	pGetTimeFormat.Call(0x0400, 2 /*TIME_NOSECONDS*/, 0, 0, uintptr(unsafe.Pointer(&t[0])), 64)
	s := syscall.UTF16ToString(d[:]) + "   " + syscall.UTF16ToString(t[:])
	if s == clockText {
		return false
	}
	clockText = s
	return true
}

// ---------------------------------------------------------------------------
// Dessin
// ---------------------------------------------------------------------------

func fillRound(hdc uintptr, r rect, radius int32, brush uintptr) {
	rgn, _, _ := pCreateRoundRgn.Call(uintptr(r.Left), uintptr(r.Top), uintptr(r.Right+1), uintptr(r.Bottom+1),
		uintptr(radius), uintptr(radius))
	pFillRgn.Call(hdc, rgn, brush)
	pDeleteObject.Call(rgn)
}

func drawText(hdc uintptr, s string, r rect, flags uintptr) {
	p := utf16(s)
	pDrawText.Call(hdc, ptr(p), ^uintptr(0), uintptr(unsafe.Pointer(&r)),
		flags|dtVCenter|dtSingleLine|dtNoPrefix)
}

func drawGlyph(hdc uintptr, r rect) {
	s, g := sc(6), sc(2)
	total := 2*s + g
	x0 := r.Left + (r.Right-r.Left-total)/2
	y0 := r.Top + (r.Bottom-r.Top-total)/2
	for i := int32(0); i < 2; i++ {
		for j := int32(0); j < 2; j++ {
			q := rect{x0 + j*(s+g), y0 + i*(s+g), x0 + j*(s+g) + s, y0 + i*(s+g) + s}
			pFillRect.Call(hdc, uintptr(unsafe.Pointer(&q)), brushGlyph)
		}
	}
}

func drawBar(hdc uintptr, w, h int32) {
	full := rect{0, 0, w, h}
	pFillRect.Call(hdc, uintptr(unsafe.Pointer(&full)), brushBar)
	pSetBkMode.Call(hdc, transparent)
	pSetTextColor.Call(hdc, rgb(245, 245, 247))
	hits = hits[:0]

	pad := sc(3)
	x := sc(6)

	// Bouton menu (symbole)
	menuR := rect{x, pad, x + sc(44), h - pad}
	hits = append(hits, hit{r: menuR, kind: hitMenu})
	if pressed == hitMenu || (hoverIdx >= 0 && hoverIdx == len(hits)-1) {
		fillRound(hdc, menuR, sc(10), brushHover)
	}
	drawGlyph(hdc, menuR)
	x = menuR.Right + sc(4)

	// Bouton Applications
	appsR := rect{x, pad, x + sc(108), h - pad}
	hits = append(hits, hit{r: appsR, kind: hitApps})
	if pressed == hitApps || (hoverIdx >= 0 && hoverIdx == len(hits)-1) {
		fillRound(hdc, appsR, sc(10), brushHover)
	}
	pSelectObject.Call(hdc, fontBold)
	drawText(hdc, t.apps, appsR, dtCenter)
	x = appsR.Right + sc(8)

	// Séparateur
	sep := rect{x, sc(7), x + 1, h - sc(7)}
	pFillRect.Call(hdc, uintptr(unsafe.Pointer(&sep)), brushSep)
	x += sc(10)

	// Horloge à droite
	pSelectObject.Call(hdc, fontReg)
	clockW := sc(150)
	clockR := rect{w - clockW - sc(10), 0, w - sc(10), h}
	drawText(hdc, clockText, clockR, dtRight)

	// Nom de la fenêtre active (comme le nom de l'application sur macOS)
	title := ""
	for _, wi := range wins {
		if wi.hwnd == active {
			title = wi.title
			break
		}
	}
	if title != "" {
		pSelectObject.Call(hdc, fontBold)
		tr := rect{x, 0, clockR.Left - sc(16), h}
		drawText(hdc, title, tr, dtLeft|dtEllipsis)
	}
}

func paintBar(hwnd uintptr) {
	var ps paintStruct
	hdc, _, _ := pBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	var cr rect
	pGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&cr)))
	w, h := cr.Right, cr.Bottom

	mem, _, _ := pCreateCompatDC.Call(hdc)
	bmp, _, _ := pCreateCompatBmp.Call(hdc, uintptr(w), uintptr(h))
	old, _, _ := pSelectObject.Call(mem, bmp)
	drawBar(mem, w, h)
	pBitBlt.Call(hdc, 0, 0, uintptr(w), uintptr(h), mem, 0, 0, srcCopy)
	pSelectObject.Call(mem, old)
	pDeleteObject.Call(bmp)
	pDeleteDC.Call(mem)
	pEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
}

func invalidate(hwnd uintptr) { pInvalidateRect.Call(hwnd, 0, 0) }

// ---------------------------------------------------------------------------
// Barre d'application (réserve la place en haut de l'écran)
// ---------------------------------------------------------------------------

func positionBar(hwnd uintptr) {
	sw, _, _ := pGetSystemMetrics.Call(smCxScreen)
	abd := appBarData{HWnd: hwnd, CallbackMessage: wmAppBar, Edge: abeTop}
	abd.Size = uint32(unsafe.Sizeof(abd))
	abd.Rc = rect{0, 0, int32(sw), barH}
	if appBarOK {
		pSHAppBarMessage.Call(abmQueryPos, uintptr(unsafe.Pointer(&abd)))
		abd.Rc.Bottom = abd.Rc.Top + barH
		pSHAppBarMessage.Call(abmSetPos, uintptr(unsafe.Pointer(&abd)))
	}
	pSetWindowPos.Call(hwnd, ^uintptr(0), /*HWND_TOPMOST*/
		uintptr(abd.Rc.Left), uintptr(abd.Rc.Top), uintptr(abd.Rc.Right-abd.Rc.Left), uintptr(barH),
		swpNoActivate|swpShowWindow)
}

func registerAppBar(hwnd uintptr) {
	abd := appBarData{HWnd: hwnd, CallbackMessage: wmAppBar}
	abd.Size = uint32(unsafe.Sizeof(abd))
	r, _, _ := pSHAppBarMessage.Call(abmNew, uintptr(unsafe.Pointer(&abd)))
	appBarOK = r != 0
	positionBar(hwnd)
}

func removeAppBar(hwnd uintptr) {
	if !appBarOK {
		return
	}
	abd := appBarData{HWnd: hwnd}
	abd.Size = uint32(unsafe.Sizeof(abd))
	pSHAppBarMessage.Call(abmRemove, uintptr(unsafe.Pointer(&abd)))
	appBarOK = false
}

// ---------------------------------------------------------------------------
// Souris
// ---------------------------------------------------------------------------

func hitAt(x, y int32) int {
	for i, h := range hits {
		if x >= h.r.Left && x < h.r.Right && y >= h.r.Top && y < h.r.Bottom {
			return i
		}
	}
	return -1
}

func lparamXY(lp uintptr) (int32, int32) {
	return int32(int16(lp & 0xFFFF)), int32(int16((lp >> 16) & 0xFFFF))
}

// screenPos convertit un coin de rectangle de la barre en coordonnées écran.
func screenPos(hwnd uintptr, r rect) (int32, int32) {
	var wr rect
	pGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&wr)))
	return wr.Left + r.Left, wr.Top + r.Bottom + sc(1)
}

func trackLeave(hwnd uintptr) {
	if tracking {
		return
	}
	tme := trackMouseEvent{Flags: tmeLeave, HwndTrack: hwnd}
	tme.Size = uint32(unsafe.Sizeof(tme))
	pTrackMouseEvent.Call(uintptr(unsafe.Pointer(&tme)))
	tracking = true
}
