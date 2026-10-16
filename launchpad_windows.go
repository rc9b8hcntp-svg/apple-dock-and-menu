//go:build windows

package main

// Le Launchpad : une grille plein écran de toutes les applications.
//
//   - le fond est une capture de l'écran, floutée et assombrie ;
//   - les programmes viennent du menu Démarrer (les mêmes que le menu Applications) ;
//   - un champ de recherche : il suffit de taper ; les pages se changent à la
//     molette, avec les points du bas, les flèches ou Page préc./suiv. ;
//   - clic sur une icône ou Entrée : lance l'application ; clic à côté ou Échap : ferme.
//
// Le dessin (grille, recherche, flou) est dans internal/launchpad ; ici on ne
// fait que la fenêtre, la capture de l'écran et les icônes des programmes.

import (
	"syscall"
	"unsafe"

	"menuwin/internal/dockgfx"
	"menuwin/internal/launchpad"
)

const (
	lpClass   = "MenuWindowsLaunchpad"
	lpTimerID = 9

	captureBlt = 0x40000000 // CAPTUREBLT : inclut aussi les fenêtres transparentes (le Dock)
	lwaAlpha   = 2
	waInactive = 0
	lpFadeStep = 8 // nombre d'images de l'apparition en fondu
)

var pSetLayeredWindowAttributes = user32.NewProc("SetLayeredWindowAttributes")

var lp struct {
	hwnd       uintptr
	prev       uintptr // fenêtre qui avait le focus avant
	x, y, w, h int32

	grid  launchpad.Grid
	apps  []appEntry
	names []string
	order []int // indices dans apps, après la recherche
	query []rune
	page  int
	sel   int // position dans order (-1 : aucune)
	wheel int

	bg      []uint32
	sprites map[int]*dockgfx.Sprite
	fade    int

	frame struct {
		dc, bmp, old uintptr
		bits         []uint32
	}
}

var lpClassOK bool

// ---------------------------------------------------------------------------
// Écran : rectangle du moniteur, capture, tampon de dessin
// ---------------------------------------------------------------------------

func monitorRect(x, y int32) rect {
	r := rect{x, y, x + 1, y + 1}
	if hm, _, _ := pMonitorFromRect.Call(uintptr(unsafe.Pointer(&r)), 2 /*MONITOR_DEFAULTTONEAREST*/); hm != 0 {
		var info monitorInfo
		info.Size = uint32(unsafe.Sizeof(info))
		if ok, _, _ := pGetMonitorInfo.Call(hm, uintptr(unsafe.Pointer(&info))); ok != 0 {
			return info.Monitor
		}
	}
	sw, _, _ := pGetSystemMetrics.Call(smCxScreen)
	sh, _, _ := pGetSystemMetrics.Call(smCyScreen)
	return rect{0, 0, int32(sw), int32(sh)}
}

// newDIB crée un tampon de pixels 32 bits associé à un contexte de dessin.
func newDIB(w, h int32) (dc, bmp, old uintptr, bits []uint32) {
	screen, _, _ := pGetDC.Call(0)
	defer pReleaseDC.Call(0, screen)
	dc, _, _ = pCreateCompatDC.Call(screen)
	bi := bitmapInfoHeader{Width: w, Height: -h, Planes: 1, BitCount: 32}
	bi.Size = uint32(unsafe.Sizeof(bi))
	var p unsafe.Pointer
	bmp, _, _ = pCreateDIBSection.Call(dc, uintptr(unsafe.Pointer(&bi)), 0, uintptr(unsafe.Pointer(&p)), 0, 0)
	if bmp == 0 || p == nil {
		pDeleteDC.Call(dc)
		return 0, 0, 0, nil
	}
	old, _, _ = pSelectObject.Call(dc, bmp)
	return dc, bmp, old, unsafe.Slice((*uint32)(p), int(w)*int(h))
}

func freeDIBParts(dc, bmp, old uintptr) {
	if dc != 0 {
		pSelectObject.Call(dc, old)
		pDeleteObject.Call(bmp)
		pDeleteDC.Call(dc)
	}
}

// captureScreen copie les pixels du moniteur (opaques).
func captureScreen(mr rect) []uint32 {
	w, h := mr.Right-mr.Left, mr.Bottom-mr.Top
	dc, bmp, old, bits := newDIB(w, h)
	if bits == nil {
		return nil
	}
	defer freeDIBParts(dc, bmp, old)
	screen, _, _ := pGetDC.Call(0)
	defer pReleaseDC.Call(0, screen)
	if ok, _, _ := pBitBlt.Call(dc, 0, 0, uintptr(w), uintptr(h), screen, uintptr(mr.Left), uintptr(mr.Top), srcCopy|captureBlt); ok == 0 {
		return nil
	}
	out := make([]uint32, len(bits))
	for i, p := range bits {
		out[i] = p | 0xFF000000
	}
	return out
}

// ---------------------------------------------------------------------------
// Icônes des applications
// ---------------------------------------------------------------------------

// lpSprite donne l'icône d'une application, chargée à la demande : une icône
// personnalisée (dossier icons) d'abord, sinon celle du raccourci.
func lpSprite(idx int) *dockgfx.Sprite {
	if s, ok := lp.sprites[idx]; ok {
		return s
	}
	a := lp.apps[idx]
	s := customTextSprite(a.name)
	if s == nil {
		s = spriteForPath(a.path)
	}
	if s == nil {
		s = defaultSprite()
	}
	size := int(lp.grid.IconSize + 0.5)
	if size > 0 && s.W > size {
		if r := dockgfx.ResizeArea(s, size, size*s.H/s.W); r != nil {
			s = r
		}
	}
	lp.sprites[idx] = s
	return s
}

// ---------------------------------------------------------------------------
// Dessin
// ---------------------------------------------------------------------------

func lpPageBounds() (int, int) {
	start := lp.page * lp.grid.PerPage
	end := start + lp.grid.PerPage
	if end > len(lp.order) {
		end = len(lp.order)
	}
	return start, end
}

func lpRender() {
	if lp.frame.bits == nil {
		return
	}
	g := lp.grid
	start, end := lpPageBounds()
	tiles := make([]launchpad.Tile, 0, end-start)
	for i := start; i < end; i++ {
		tiles = append(tiles, launchpad.Tile{Sprite: lpSprite(lp.order[i]), Selected: i == lp.sel})
	}
	launchpad.Paint(lp.frame.bits, lp.bg, launchpad.View{Grid: g, Tiles: tiles, Page: lp.page, Pages: g.Pages(len(lp.order))})

	// Le texte est écrit par Windows par-dessus l'image (elle est opaque).
	dc := lp.frame.dc
	pSetBkMode.Call(dc, transparent)
	pSelectObject.Call(dc, fontReg)
	pSetTextColor.Call(dc, rgb(255, 255, 255))
	for slot := 0; slot < end-start; slot++ {
		lr := g.LabelRect(slot)
		r := rect{int32(lr.X), int32(lr.Y), int32(lr.X + lr.W), int32(lr.Y + lr.H)}
		drawText(dc, lp.apps[lp.order[start+slot]].name, r, dtCenter|dtEllipsis)
	}

	sr := g.Search
	textR := rect{int32(sr.X + 34*g.Scale), int32(sr.Y), int32(sr.X + sr.W - 12*g.Scale), int32(sr.Y + sr.H)}
	if len(lp.query) == 0 {
		pSetTextColor.Call(dc, rgb(200, 200, 208))
		drawText(dc, t.search, textR, dtLeft|dtEllipsis)
	} else {
		q := string(lp.query)
		drawText(dc, q, textR, dtLeft|dtEllipsis)
		cx := textR.Left + textWidth(dc, q) + sc(2)
		if cx < textR.Right {
			caret := rect{cx, textR.Top + sc(7), cx + sc(1) + 1, textR.Bottom - sc(7)}
			pFillRect.Call(dc, uintptr(unsafe.Pointer(&caret)), brushGlyph)
		}
	}
	if len(lp.order) == 0 {
		pSetTextColor.Call(dc, rgb(200, 200, 208))
		mid := rect{0, int32(g.GridY), int32(g.W), int32(g.GridY + g.CellH*float64(g.Rows))}
		drawText(dc, t.noResult, mid, dtCenter)
	}
	if lp.hwnd != 0 {
		pInvalidateRect.Call(lp.hwnd, 0, 0)
	}
}

// ---------------------------------------------------------------------------
// Actions
// ---------------------------------------------------------------------------

func lpRefilter() {
	lp.order = launchpad.Filter(lp.names, string(lp.query))
	lp.page = 0
	lp.sel = -1
	if len(lp.query) > 0 && len(lp.order) > 0 {
		lp.sel = 0 // Entrée lance le premier résultat
	}
	lpRender()
}

func lpGoPage(p int) {
	pages := lp.grid.Pages(len(lp.order))
	if p < 0 {
		p = 0
	}
	if p >= pages {
		p = pages - 1
	}
	if p == lp.page {
		return
	}
	lp.page, lp.sel = p, -1
	lpRender()
}

func lpMove(dx, dy int) {
	n := len(lp.order)
	if n == 0 {
		return
	}
	if lp.sel < 0 {
		lp.sel = lp.page * lp.grid.PerPage
	} else {
		s := lp.sel + dx + dy*lp.grid.Cols
		if s < 0 {
			s = 0
		}
		if s > n-1 {
			s = n - 1
		}
		lp.sel = s
	}
	lp.page = lp.sel / lp.grid.PerPage
	lpRender()
}

func lpLaunch(pos int) {
	if pos < 0 || pos >= len(lp.order) {
		return
	}
	path := lp.apps[lp.order[pos]].path
	closeLaunchpad(false)
	openURI(path)
}

func lpSlotPos(x, y int32) int {
	slot := lp.grid.Slot(float64(x), float64(y))
	if slot < 0 {
		return -1
	}
	pos := lp.page*lp.grid.PerPage + slot
	if pos >= len(lp.order) {
		return -1
	}
	return pos
}

// ---------------------------------------------------------------------------
// Fenêtre
// ---------------------------------------------------------------------------

func lpProc(hwnd, message, wparam, lparam uintptr) uintptr {
	switch uint32(message) {
	case wmEraseBkgnd:
		return 1

	case wmPaint:
		var ps paintStruct
		hdc, _, _ := pBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		if hwnd == lp.hwnd && lp.frame.dc != 0 {
			pBitBlt.Call(hdc, 0, 0, uintptr(lp.w), uintptr(lp.h), lp.frame.dc, 0, 0, srcCopy)
		}
		pEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		return 0

	case wmTimer:
		if hwnd == lp.hwnd && wparam == lpTimerID {
			lp.fade++
			alpha := lp.fade * 255 / lpFadeStep
			if alpha >= 255 {
				alpha = 255
				pKillTimer.Call(hwnd, lpTimerID)
			}
			pSetLayeredWindowAttributes.Call(hwnd, 0, uintptr(alpha), lwaAlpha)
		}
		return 0

	case wmActivate:
		// Un autre programme prend le focus (Alt+Tab, clic ailleurs) : on referme.
		if hwnd == lp.hwnd && wparam&0xFFFF == waInactive {
			closeLaunchpad(false)
		}
		return 0

	case wmMouseMove:
		if hwnd != lp.hwnd {
			return 0
		}
		x, y := lparamXY(lparam)
		if pos := lpSlotPos(x, y); pos >= 0 && pos != lp.sel {
			lp.sel = pos
			lpRender()
		}
		return 0

	case wmLButtonUp:
		if hwnd != lp.hwnd {
			return 0
		}
		x, y := lparamXY(lparam)
		if pos := lpSlotPos(x, y); pos >= 0 {
			lpLaunch(pos)
		} else if d := lp.grid.DotAt(float64(x), float64(y), lp.grid.Pages(len(lp.order))); d >= 0 {
			lpGoPage(d)
		} else if !lp.grid.InSearch(float64(x), float64(y)) {
			closeLaunchpad(true)
		}
		return 0

	case wmMouseWheel:
		if hwnd != lp.hwnd {
			return 0
		}
		lp.wheel += int(int16(wparam >> 16))
		for lp.wheel <= -120 {
			lp.wheel += 120
			lpGoPage(lp.page + 1)
		}
		for lp.wheel >= 120 {
			lp.wheel -= 120
			lpGoPage(lp.page - 1)
		}
		return 0

	case wmChar:
		if hwnd != lp.hwnd {
			return 0
		}
		c := rune(wparam)
		switch {
		case c == 8: // Retour arrière
			if n := len(lp.query); n > 0 {
				lp.query = lp.query[:n-1]
				lpRefilter()
			}
		case c >= 32 && c != 127 && !(c >= 0xD800 && c <= 0xDFFF):
			if len(lp.query) < 60 {
				lp.query = append(lp.query, c)
				lpRefilter()
			}
		}
		return 0

	case wmKeyDown:
		if hwnd != lp.hwnd {
			return 0
		}
		switch wparam {
		case vkEscape:
			if len(lp.query) > 0 {
				lp.query = nil
				lpRefilter()
			} else {
				closeLaunchpad(true)
			}
		case vkReturn:
			if lp.sel >= 0 {
				lpLaunch(lp.sel)
			} else if len(lp.query) > 0 && len(lp.order) > 0 {
				lpLaunch(0)
			}
		case vkLeft:
			lpMove(-1, 0)
		case vkRight:
			lpMove(1, 0)
		case vkUp:
			lpMove(0, -1)
		case vkDown:
			lpMove(0, 1)
		case vkPrior:
			lpGoPage(lp.page - 1)
		case vkNext:
			lpGoPage(lp.page + 1)
		case vkHome:
			lp.page, lp.sel = 0, -1
			lpRender()
		}
		return 0

	case wmClose:
		closeLaunchpad(true)
		return 0
	}
	r, _, _ := pDefWindowProc.Call(hwnd, message, wparam, lparam)
	return r
}

func registerLaunchpadClass() {
	if lpClassOK {
		return
	}
	hInst, _, _ := pGetModuleHandle.Call(0)
	cursor, _, _ := pLoadCursor.Call(0, idcArrow)
	wc := wndClassEx{WndProc: syscall.NewCallback(lpProc), Instance: hInst, Cursor: cursor, ClassName: utf16(lpClass)}
	wc.Size = uint32(unsafe.Sizeof(wc))
	pRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
	lpClassOK = true
}

// showLaunchpad ouvre le Launchpad, ou le referme s'il est déjà ouvert.
func showLaunchpad() {
	if lp.hwnd != 0 {
		closeLaunchpad(true)
		return
	}
	if menuOpen || dialogOpen {
		return
	}
	registerLaunchpadClass()
	hideLabel()

	cur := cursorPt()
	mr := monitorRect(cur.X, cur.Y)
	w, h := mr.Right-mr.Left, mr.Bottom-mr.Top
	if w < 320 || h < 240 {
		return
	}

	// Fond : l'écran tel qu'il est, flouté et assombri.
	lp.bg = nil
	if shot := captureScreen(mr); shot != nil {
		lp.bg = launchpad.BlurDim(shot, int(w), int(h), 0.5)
	} else {
		lp.bg = make([]uint32, int(w)*int(h))
		for i := range lp.bg {
			lp.bg[i] = 0xFF1E1E22
		}
	}

	lp.apps = scanApps()
	lp.names = lp.names[:0]
	for _, a := range lp.apps {
		lp.names = append(lp.names, a.name)
	}
	lp.sprites = map[int]*dockgfx.Sprite{}
	lp.query, lp.page, lp.sel, lp.wheel, lp.fade = nil, 0, -1, 0, 0
	lp.x, lp.y, lp.w, lp.h = mr.Left, mr.Top, w, h
	lp.grid = launchpad.NewGrid(int(w), int(h), scale)
	lp.order = launchpad.Filter(lp.names, "")

	dc, bmp, old, bits := newDIB(w, h)
	if bits == nil {
		lp.bg = nil
		return
	}
	lp.frame.dc, lp.frame.bmp, lp.frame.old, lp.frame.bits = dc, bmp, old, bits
	lpRender() // charge aussi les icônes de la première page

	lp.prev, _, _ = pGetForeground.Call()
	hInst, _, _ := pGetModuleHandle.Call(0)
	hwnd, _, _ := pCreateWindowEx.Call(
		wsExLayered|wsExTopmost|wsExToolwin,
		ptr(utf16(lpClass)), ptr(utf16("Launchpad")), wsPopup,
		uintptr(mr.Left), uintptr(mr.Top), uintptr(w), uintptr(h), 0, 0, hInst, 0)
	if hwnd == 0 {
		freeLaunchpadData()
		return
	}
	lp.hwnd = hwnd
	pSetLayeredWindowAttributes.Call(hwnd, 0, 0, lwaAlpha)
	pShowWindow.Call(hwnd, swShow)
	pSetForeground.Call(hwnd)
	pSetFocus.Call(hwnd)
	pSetTimer.Call(hwnd, lpTimerID, 16, 0)
}

func freeLaunchpadData() {
	freeDIBParts(lp.frame.dc, lp.frame.bmp, lp.frame.old)
	lp.frame.dc, lp.frame.bmp, lp.frame.old, lp.frame.bits = 0, 0, 0, nil
	lp.bg, lp.sprites, lp.apps, lp.order = nil, nil, nil, nil
}

// closeLaunchpad ferme la fenêtre. Si restore est vrai, le focus revient à la
// fenêtre qui l'avait avant.
func closeLaunchpad(restore bool) {
	h := lp.hwnd
	if h == 0 {
		return
	}
	lp.hwnd = 0 // d'abord : les messages envoyés pendant la fermeture voient qu'elle est close
	pKillTimer.Call(h, lpTimerID)
	pDestroyWindow.Call(h)
	freeLaunchpadData()
	if restore && lp.prev != 0 && lp.prev != barHwnd {
		if ok, _, _ := pIsWindow.Call(lp.prev); ok != 0 {
			pSetForeground.Call(lp.prev)
		}
	}
}
