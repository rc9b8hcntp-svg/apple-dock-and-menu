//go:build windows

package main

// Boutons de fenêtre façon macOS sur les autres applications (expérimental, désactivé
// par défaut : Réglages > Apparence).
//
// Windows dessine lui-même la barre de titre des autres programmes : on ne peut pas
// y déplacer les boutons. Lumen pose donc par-dessus une barre de titre à la macOS
// (pastilles rouge / jaune / verte à gauche, titre centré), rattachée à chaque fenêtre.
// Aucune fenêtre n'est modifiée : on ne fait que dessiner au-dessus et envoyer les mêmes
// commandes que les boutons de Windows (fermer, réduire, agrandir, déplacer, redimensionner).
//
// Les programmes qui dessinent eux-mêmes leur barre de titre (navigateurs, Explorateur,
// Terminal, Office, applications du Store…) sont laissés tels quels.

import (
	"strings"
	"syscall"
	"unsafe"
)

const (
	capTimerID = 9
	capClass   = "LumenTitleBar"

	scClose    = 0xF060
	scMinimize = 0xF020
	scMaximize = 0xF030
	scRestore  = 0xF120

	wmSysCommand    = 0x0112
	wmNCLButtonDown = 0x00A1
	wmLButtonDblClk = 0x0203
	wmSetCursor     = 0x0020
	wmMouseActivate = 0x0021
	wmPopupSysMenu  = 0x0313

	htLeft, htRight, htTop, htTopLeft, htTopRight = 10, 11, 12, 13, 14
)

var (
	pGetTitleBarInfo = user32.NewProc("GetTitleBarInfo")
	pGetClassName    = user32.NewProc("GetClassNameW")
	pSetCursor       = user32.NewProc("SetCursor")
)

type titleBarInfo struct {
	Size  uint32
	Rect  rect
	State [6]uint32
}

type capOverlay struct {
	hwnd, target uintptr
	rc           rect
	prev         uintptr
	title        string
	active       bool
	zoomed       bool
	hover        int // 0 rien, 1 à 3 : pastille survolée
	group        bool
	press        int
	tracking     bool
}

var (
	capByTarget = map[uintptr]*capOverlay{}
	capByHwnd   = map[uintptr]*capOverlay{}
	capClassOK  bool
	capTimerOn  bool
)

// Classes de fenêtres qui dessinent leur propre barre de titre (ou qu'il ne faut pas toucher).
var capSkipClasses = []string{
	"Chrome_WidgetWin", "MozillaWindowClass", "ApplicationFrameWindow", "Windows.UI.Core",
	"CabinetWClass", "WorkerW", "Progman", "Shell_", "OpusApp", "XLMAIN", "PPTFrameClass",
	"Qt", "CASCADIA_HOSTING", "WinUIDesktop", "Notepad", "TaskManagerWindow", "SunAwt",
	"ConsoleWindowClass", "Lumen", "MenuWindows",
}

func capSync() {
	if barHwnd == 0 {
		return
	}
	if cfg.WindowButtons && !capTimerOn {
		pSetTimer.Call(barHwnd, capTimerID, 30, 0)
		capTimerOn = true
	} else if !cfg.WindowButtons && capTimerOn {
		pKillTimer.Call(barHwnd, capTimerID)
		capTimerOn = false
		capCloseAll()
	}
}

func capCloseAll() {
	for _, o := range capByTarget {
		pDestroyWindow.Call(o.hwnd)
	}
	capByTarget = map[uintptr]*capOverlay{}
	capByHwnd = map[uintptr]*capOverlay{}
}

func capEligible(h uintptr) (rect, bool) {
	if v, _, _ := pIsWindowVisible.Call(h); v == 0 {
		return rect{}, false
	}
	if ic, _, _ := pIsIconic.Call(h); ic != 0 {
		return rect{}, false
	}
	if getLongPtr(h, -16)&wsCaption != wsCaption {
		return rect{}, false
	}
	var cls [128]uint16
	n, _, _ := pGetClassName.Call(h, uintptr(unsafe.Pointer(&cls[0])), 128)
	name := syscall.UTF16ToString(cls[:n])
	for _, p := range capSkipClasses {
		if strings.HasPrefix(name, p) {
			return rect{}, false
		}
	}
	var pid uint32
	pGetWindowThreadPI.Call(h, uintptr(unsafe.Pointer(&pid)))
	if uintptr(pid) == ownPID {
		return rect{}, false
	}
	var tbi titleBarInfo
	tbi.Size = uint32(unsafe.Sizeof(tbi))
	if r, _, _ := pGetTitleBarInfo.Call(h, uintptr(unsafe.Pointer(&tbi))); r == 0 {
		return rect{}, false
	}
	if tbi.State[0]&0x8000 != 0 { // STATE_SYSTEM_INVISIBLE
		return rect{}, false
	}
	rc := tbi.Rect
	if rc.Right-rc.Left < sc(160) || rc.Bottom-rc.Top < sc(18) || rc.Bottom-rc.Top > sc(60) {
		return rect{}, false
	}
	return rc, true
}

func capEnsureClass() {
	if capClassOK {
		return
	}
	hInst, _, _ := pGetModuleHandle.Call(0)
	cursor, _, _ := pLoadCursor.Call(0, idcArrow)
	wc := wndClassEx{Style: 0x0008 /*CS_DBLCLKS*/, WndProc: syscall.NewCallback(capProc), Instance: hInst, Cursor: cursor, ClassName: utf16(capClass)}
	wc.Size = uint32(unsafe.Sizeof(wc))
	pRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
	capClassOK = true
}

// capTick suit les fenêtres : crée, déplace et supprime les barres de titre.
func capTick() {
	if !cfg.WindowButtons {
		return
	}
	capEnsureClass()
	fg, _, _ := pGetForeground.Call()
	seen := map[uintptr]bool{}
	for _, w := range wins {
		rc, ok := capEligible(w.hwnd)
		if !ok {
			continue
		}
		seen[w.hwnd] = true
		o := capByTarget[w.hwnd]
		if o == nil {
			hInst, _, _ := pGetModuleHandle.Call(0)
			h, _, _ := pCreateWindowEx.Call(wsExToolwin|wsExNoActive, ptr(utf16(capClass)), 0, wsPopup,
				uintptr(rc.Left), uintptr(rc.Top), uintptr(rc.Right-rc.Left), uintptr(rc.Bottom-rc.Top), 0, 0, hInst, 0)
			if h == 0 {
				continue
			}
			o = &capOverlay{hwnd: h, target: w.hwnd}
			capByTarget[w.hwnd] = o
			capByHwnd[h] = o
			o.rc = rect{0, 0, 0, 0}
		}
		changed := false
		active := fg == w.hwnd
		z, _, _ := pIsZoomed.Call(w.hwnd)
		if o.active != active || o.zoomed != (z != 0) || o.title != w.title {
			o.active, o.zoomed, o.title = active, z != 0, w.title
			changed = true
		}
		// Ordre d'empilement : juste au-dessus de la fenêtre, sous celles qui la couvrent.
		above, _, _ := pGetWindow.Call(w.hwnd, 3 /*GW_HWNDPREV*/)
		for above == o.hwnd {
			above, _, _ = pGetWindow.Call(above, 3)
		}
		insert := above
		targetTop := getLongPtr(w.hwnd, -20)&wsExTopmost != 0
		if insert != 0 && !targetTop && getLongPtr(insert, -20)&wsExTopmost != 0 {
			insert = 0 // HWND_TOP : en haut des fenêtres non « toujours au premier plan »
		} else if insert == 0 && targetTop {
			insert = ^uintptr(0) // HWND_TOPMOST
		}
		if rc != o.rc || insert != o.prev || !o.shown() {
			flags := uintptr(0x0010 | 0x0040) // NOACTIVATE | SHOWWINDOW
			pSetWindowPos.Call(o.hwnd, insert, uintptr(rc.Left), uintptr(rc.Top),
				uintptr(rc.Right-rc.Left), uintptr(rc.Bottom-rc.Top), flags)
			o.rc, o.prev = rc, insert
			changed = true
		}
		if changed {
			pInvalidateRect.Call(o.hwnd, 0, 0)
		}
	}
	for t, o := range capByTarget {
		if !seen[t] {
			delete(capByTarget, t)
			delete(capByHwnd, o.hwnd)
			pDestroyWindow.Call(o.hwnd)
		}
	}
}

func (o *capOverlay) shown() bool {
	v, _, _ := pIsWindowVisible.Call(o.hwnd)
	return v != 0
}

// ---------------------------------------------------------------------------
// Dessin
// ---------------------------------------------------------------------------

func capLight(o *capOverlay, i int, h int32) rect {
	d := h * 38 / 100
	if d < sc(10) {
		d = sc(10)
	}
	x := sc(13) + int32(i)*(d+sc(8))
	y := (h - d) / 2
	return rect{x, y, x + d, y + d}
}

func capDraw(o *capOverlay) func(hdc uintptr, w, h int32) {
	return func(hdc uintptr, w, h int32) {
		bg := rgb(232, 232, 236)
		if pal.dark {
			bg = rgb(52, 52, 55)
		}
		if !o.active {
			if pal.dark {
				bg = rgb(44, 44, 47)
			} else {
				bg = rgb(244, 244, 246)
			}
		}
		full := rect{0, 0, w, h}
		b, _, _ := pCreateSolidBrush.Call(bg)
		pFillRect.Call(hdc, uintptr(unsafe.Pointer(&full)), b)
		pDeleteObject.Call(b)
		fillColor(hdc, rect{0, h - 1, w, h}, blendColor(bg, rgb(0, 0, 0), 0.18))
		pSetBkMode.Call(hdc, transparent)

		grey := blendColor(bg, pal.menuKey, 0.45)
		cols := [3]uintptr{rgb(255, 95, 87), rgb(254, 188, 46), rgb(40, 200, 64)}
		glyphs := [3]string{"×", "–", "+"}
		if !o.active && !o.group {
			cols = [3]uintptr{grey, grey, grey}
		}
		var lightsEnd int32
		for i := 0; i < 3; i++ {
			r := capLight(o, i, h)
			lightsEnd = r.Right
			fillEllipse(hdc, rect{r.Left - 1, r.Top - 1, r.Right + 1, r.Bottom + 1}, blendColor(cols[i], rgb(0, 0, 0), 0.14))
			fillEllipse(hdc, r, cols[i])
			if o.group {
				pSelectObject.Call(hdc, fontBold)
				pSetTextColor.Call(hdc, blendColor(cols[i], rgb(0, 0, 0), 0.7))
				drawText(hdc, glyphs[i], r, dtCenter)
			}
		}
		margin := lightsEnd + sc(12)
		pSelectObject.Call(hdc, fontBold)
		if o.active {
			pSetTextColor.Call(hdc, pal.menuText)
		} else {
			pSetTextColor.Call(hdc, pal.menuKey)
		}
		drawText(hdc, o.title, rect{margin, 0, w - margin, h - 1}, dtCenter|dtEllipsis)
	}
}

// ---------------------------------------------------------------------------
// Souris
// ---------------------------------------------------------------------------

func capLightAt(o *capOverlay, x, y, h int32) int {
	for i := 0; i < 3; i++ {
		r := capLight(o, i, h)
		if inRect(rect{r.Left - sc(3), 0, r.Right + sc(3), h}, x, y) {
			return i + 1
		}
	}
	return 0
}

func capHeight(o *capOverlay) int32 { return o.rc.Bottom - o.rc.Top }

// capEdge : la zone de redimensionnement du bord haut (qu'on ne peut pas « laisser passer » à la fenêtre dessous).
func capEdge(o *capOverlay, x, y int32) uintptr {
	if o.zoomed || y >= sc(4) {
		return 0
	}
	w := o.rc.Right - o.rc.Left
	switch {
	case x < sc(8):
		return htTopLeft
	case x >= w-sc(8):
		return htTopRight
	}
	return htTop
}

func capProc(hwnd, message, wparam, lparam uintptr) uintptr {
	o := capByHwnd[hwnd]
	if o != nil {
		h := capHeight(o)
		switch uint32(message) {
		case wmEraseBkgnd:
			return 1
		case wmPaint:
			paintBuffered(hwnd, capDraw(o))
			return 0
		case wmMouseMove:
			x, y := lparamXY(lparam)
			hov := capLightAt(o, x, y, h)
			grp := inRect(rect{0, 0, capLight(o, 2, h).Right + sc(10), h}, x, y)
			if hov != o.hover || grp != o.group {
				o.hover, o.group = hov, grp
				pInvalidateRect.Call(hwnd, 0, 0)
			}
			if !o.tracking {
				tme := trackMouseEvent{Flags: tmeLeave, HwndTrack: hwnd}
				tme.Size = uint32(unsafe.Sizeof(tme))
				pTrackMouseEvent.Call(uintptr(unsafe.Pointer(&tme)))
				o.tracking = true
			}
			return 0
		case wmMouseLeave:
			o.tracking, o.hover, o.group = false, 0, false
			pInvalidateRect.Call(hwnd, 0, 0)
			return 0
		case wmSetCursor:
			var p point
			pGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
			var wr rect
			pGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&wr)))
			id := uintptr(idcArrow)
			switch capEdge(o, p.X-wr.Left, p.Y-wr.Top) {
			case htTop:
				id = 32645 // IDC_SIZENS
			case htTopLeft:
				id = 32642 // IDC_SIZENWSE
			case htTopRight:
				id = 32643 // IDC_SIZENESW
			}
			c, _, _ := pLoadCursor.Call(0, id)
			pSetCursor.Call(c)
			return 1
		case wmLButtonDown:
			x, y := lparamXY(lparam)
			o.press = capLightAt(o, x, y, h)
			if o.press != 0 {
				return 0
			}
			var p point
			pGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
			pos := uintptr(uint16(int16(p.X))) | uintptr(uint16(int16(p.Y)))<<16
			bringToFront(o.target)
			pReleaseCapture.Call()
			if e := capEdge(o, x, y); e != 0 {
				pPostMessage.Call(o.target, wmNCLButtonDown, e, pos)
			} else if o.zoomed {
				// Une fenêtre agrandie ne se déplace pas : un double clic la restaure.
			} else {
				pPostMessage.Call(o.target, wmNCLButtonDown, 2 /*HTCAPTION*/, pos)
			}
			return 0
		case wmLButtonDblClk:
			x, y := lparamXY(lparam)
			if capLightAt(o, x, y, h) == 0 && capEdge(o, x, y) == 0 {
				capZoom(o)
			}
			return 0
		case wmLButtonUp:
			x, y := lparamXY(lparam)
			i := capLightAt(o, x, y, h)
			if i != 0 && i == o.press {
				switch i {
				case 1:
					pPostMessage.Call(o.target, wmSysCommand, scClose, 0)
				case 2:
					pPostMessage.Call(o.target, wmSysCommand, scMinimize, 0)
				case 3:
					capZoom(o)
				}
			}
			o.press = 0
			return 0
		case wmRButtonUp:
			var p point
			pGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
			pos := uintptr(uint16(int16(p.X))) | uintptr(uint16(int16(p.Y)))<<16
			pPostMessage.Call(o.target, wmPopupSysMenu, 0, pos)
			return 0
		case wmMouseActivate:
			return 3 // MA_NOACTIVATE : cliquer ne vole pas le focus
		}
	}
	r, _, _ := pDefWindowProc.Call(hwnd, message, wparam, lparam)
	return r
}

func capZoom(o *capOverlay) {
	if o.zoomed {
		pPostMessage.Call(o.target, wmSysCommand, scRestore, 0)
	} else {
		pPostMessage.Call(o.target, wmSysCommand, scMaximize, 0)
	}
}
