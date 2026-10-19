//go:build windows

package main

import (
	"path/filepath"
	"strings"
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
	hitMenu    = iota + 1 // symbole du menu système
	hitAppName            // nom de l'application active
	hitApps               // menu Applications
	hitWindow             // menu Fenêtre
	hitFile
	hitEdit
	hitInput
	hitBattery
	hitWifi
	hitBluetooth
	hitSound
	hitSearch
	hitControl
	hitClock
)

type hit struct {
	r    rect
	kind int
	hwnd uintptr
}

var (
	scale      = 1.0
	barH       int32
	barHwnd    uintptr
	appBarOK   bool
	ownPID     uintptr
	wins       []winItem
	active     uintptr
	hits       []hit
	hoverIdx   = -1
	tracking   bool
	pressed    int // type de l'élément dont le menu est ouvert
	clockText  string
	activeName string // nom de l'application active (affiché en gras, comme sur macOS)

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
	barH = sc(float64(cfg.BarHeight)) // 24 par défaut : la hauteur de la barre des menus de macOS
	pid, _, _ := pGetCurrentPID.Call()
	ownPID = pid
	initGDIPlus()

	mk := func(weight int) uintptr {
		h := -sc(13)
		f, _, _ := pCreateFont.Call(uintptr(h), 0, 0, 0, uintptr(weight), 0, 0, 0,
			1 /*DEFAULT_CHARSET*/, 0, 0, 5 /*CLEARTYPE*/, 0, ptr(utf16(uiFace())))
		return f
	}
	fontReg = mk(400)
	fontBold = mk(600)
	fontIcon = makeIconFont()
	updateStatus()
}

func freeBarResources() {
	for _, o := range []uintptr{brushBar, brushHover, brushAccent, brushSep, brushGlyph, brushWin, brushSide, fontReg, fontBold, fontIcon} {
		pDeleteObject.Call(o)
	}
	shutdownGDIPlus()
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
	if name := appNameFor(active); name != activeName {
		activeName = name
		changed = true
	}
	return changed
}

// appNameFor donne le nom à afficher en gras à gauche de la barre : le nom
// lisible du programme (« Bloc-notes »), comme le nom de l'app sur macOS.
func appNameFor(h uintptr) string {
	if h == 0 {
		return t.desktop
	}
	title := ""
	for _, w := range wins {
		if w.hwnd == h {
			title = w.title
			break
		}
	}
	p := cachedProcPath(h)
	// Les applications du Store passent par ApplicationFrameHost : le titre est plus parlant.
	if p == "" || strings.EqualFold(filepath.Base(p), "ApplicationFrameHost.exe") {
		return title
	}
	return appLabel(p)
}

func updateClock() bool {
	var d, t [64]uint16
	// Heure : le format du système (sans les secondes), ou 24 h / 12 h imposé.
	flags, picture := uintptr(2 /*TIME_NOSECONDS*/), uintptr(0)
	switch {
	case cfg.Clock == "24" && cfg.ShowSeconds:
		flags, picture = 0, ptr(utf16("HH:mm:ss"))
	case cfg.Clock == "24":
		flags, picture = 0, ptr(utf16("HH:mm"))
	case cfg.Clock == "12" && cfg.ShowSeconds:
		flags, picture = 0, ptr(utf16("h:mm:ss tt"))
	case cfg.Clock == "12":
		flags, picture = 0, ptr(utf16("h:mm tt"))
	case cfg.ShowSeconds:
		flags = 0
	}
	pGetTimeFormat.Call(0x0400, flags, 0, picture, uintptr(unsafe.Pointer(&t[0])), 64)
	s := syscall.UTF16ToString(t[:])
	if cfg.ShowDate {
		pGetDateFormat.Call(0x0400, 0, 0, ptr(utf16("ddd d MMM")), uintptr(unsafe.Pointer(&d[0])), 64)
		s = syscall.UTF16ToString(d[:]) + " " + s
	}
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
	if !barBlurPaint(hdc, w, h) {
		pFillRect.Call(hdc, uintptr(unsafe.Pointer(&full)), brushBar)
	}
	pSetBkMode.Call(hdc, transparent)
	pSetTextColor.Call(hdc, pal.barText)
	hits = hits[:0]

	// Comme sur macOS : un élément ne s'éclaire que lorsque son menu est ouvert
	// (ou pendant le clic), pas au simple survol ; chaque titre a 10 px de marge.
	pad := sc(2)
	margin := sc(10)
	x := sc(8)
	iconW := sc(28)

	add := func(kind int, r rect) {
		hits = append(hits, hit{r: r, kind: kind})
		if pressed == kind || (kind == hitSearch && sp.hwnd != 0) || (kind == hitControl && cc.hwnd != 0) || (kind == hitClock && cal.hwnd != 0) {
			fillRR(hdc, r, sc(9), pal.barPress)
		}
	}

	// À droite, en partant du bord : horloge, centre de contrôle, recherche, son, Wi-Fi, batterie.
	right := w - sc(6)
	item := func(kind int, wd int32) rect {
		r := rect{right - wd, pad, right, h - pad}
		right = r.Left
		add(kind, r)
		return r
	}
	pSelectObject.Call(hdc, fontReg)
	r := item(hitClock, textWidth(hdc, clockText)+2*sc(8))
	drawText(hdc, clockText, r, dtCenter)
	if cfg.ShowControl {
		drawControlIcon(hdc, item(hitControl, iconW))
	}
	if cfg.ShowSearch {
		drawIconGlyph(hdc, "\uE721", item(hitSearch, iconW), pal.barText)
	}
	if cfg.ShowSound {
		drawIconGlyph(hdc, "\uE767", item(hitSound, iconW), pal.barText)
	}
	if cfg.ShowWifi && wifiState != wifiNone {
		col := pal.barText
		if wifiState == wifiOff {
			col = pal.menuOff
		}
		drawIconGlyph(hdc, "\uE701", item(hitWifi, iconW), col)
	}
	if inputLabel != "" {
		it := item(hitInput, textWidth(hdc, inputLabel)+2*sc(8))
		drawText(hdc, inputLabel, it, dtCenter)
	}
	if cfg.ShowBluetooth {
		if present, on, _ := btState(); present {
			col := pal.barText
			if !on {
				col = pal.menuOff
			}
			drawIconGlyph(hdc, "\uE702", item(hitBluetooth, iconW), col)
		}
	}
	if cfg.ShowBattery && battPct >= 0 {
		drawBattery(hdc, item(hitBattery, batteryItemWidth(hdc)))
	}
	limit := right

	// Symbole du menu système (équivalent du menu Apple)
	menuR := rect{x, pad, x + sc(34), h - pad}
	add(hitMenu, menuR)
	drawGlyph(hdc, menuR)
	x = menuR.Right

	// Nom de l'application active, en gras (comme « Finder » sur macOS) : un clic ouvre son menu.
	if cfg.ShowAppName && activeName != "" {
		pSelectObject.Call(hdc, fontBold)
		nameW := textWidth(hdc, activeName)
		maxW := sc(240)
		if room := limit - x - 3*margin - sc(100); room < maxW {
			maxW = room
		}
		if maxW < 0 {
			maxW = 0
		}
		if nameW > maxW {
			nameW = maxW
		}
		itemR := rect{x, pad, x + nameW + 2*margin, h - pad}
		if active != 0 {
			add(hitAppName, itemR)
		}
		drawText(hdc, activeName, rect{x + margin, 0, x + margin + nameW, h}, dtLeft|dtEllipsis)
		x = itemR.Right
	}

	// Menus Applications et Fenêtre, en police normale comme les autres menus de macOS
	pSelectObject.Call(hdc, fontReg)
	menu := func(kind int, label string) {
		r := rect{x, pad, x + textWidth(hdc, label) + 2*margin, h - pad}
		if r.Right > limit {
			return
		}
		add(kind, r)
		drawText(hdc, label, r, dtCenter)
		x = r.Right
	}
	if cfg.ShowFile && active != 0 {
		menu(hitFile, tr("Fichier", "File"))
	}
	if cfg.ShowEdit && active != 0 {
		menu(hitEdit, tr("Édition", "Edit"))
	}
	if cfg.ShowApps {
		menu(hitApps, t.apps)
	}
	if cfg.ShowWindow {
		menu(hitWindow, tr("Fenêtre", "Window"))
	}
}

// hitRect donne le rectangle d'un élément de la barre.
func hitRect(kind int) (rect, bool) {
	for _, h := range hits {
		if h.kind == kind {
			return h.r, true
		}
	}
	return rect{}, false
}

// barMenuKindAt renvoie le type du menu de la barre qui se trouve sous un point
// de l'écran (0 si aucun) : on s'en sert pour passer d'un menu à l'autre en glissant.
func barMenuKindAt(p point) int {
	var wr rect
	pGetWindowRect.Call(barHwnd, uintptr(unsafe.Pointer(&wr)))
	if i := hitAt(p.X-wr.Left, p.Y-wr.Top); i >= 0 {
		return hits[i].kind
	}
	return 0
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
	if cfg.BarAutoHide { // la barre ne réserve pas de place : elle glisse en haut de l'écran
		barPlaceSlid()
		return
	}
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
	if cfg.BarAutoHide {
		barAuto.prevAuto = true
		positionBar(hwnd)
		return
	}
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
