//go:build windows

package main

// Sélecteur d'applications façon Cmd+Tab (réglage « Sélecteur d'applications », désactivé
// par défaut) : Alt+Tab ouvre un panneau de grandes icônes ; Tab (ou ← →) avance,
// Maj+Tab recule, Q quitte l'application choisie, Échap annule, relâcher Alt active
// l'application. Un clic sur une icône l'active aussi.
//
// Il remplace Alt+Tab grâce à un crochet clavier (WH_KEYBOARD_LL) : le crochet ne fait que
// poster des messages à la fenêtre de la barre, pour rester instantané.

import (
	"strings"
	"syscall"
	"unsafe"
)

const (
	swMsg        = wmApp + 20 // message envoyé à la barre ; wparam = action
	swActOpen    = 1
	swActNext    = 2
	swActPrev    = 3
	swActCommit  = 4
	swActCancel  = 5
	swActQuit    = 6
	swClass      = "MenuWindowsSwitcher"
	whKeyboardLL = 13
	swTimerID    = 1
)

var (
	pSetWindowsHookEx    = user32.NewProc("SetWindowsHookExW")
	pUnhookWindowsHookEx = user32.NewProc("UnhookWindowsHookEx")
	pCallNextHookEx      = user32.NewProc("CallNextHookEx")

	swHook    uintptr
	swHookCB  = syscall.NewCallback(swHookProc)
	swActive  bool // le crochet a avalé un Alt+Tab : le panneau est (ou va être) ouvert
	swClassOK bool

	sw struct {
		hwnd    uintptr
		items   []*dockApp
		sel     int
		w, h    int32
		tile    int32
		pad     int32
		prev    uintptr
		hovered int
	}
)

type kbdHook struct {
	VK, Scan, Flags, Time uint32
	Extra                 uintptr
}

func swKeyDown(vk uintptr) bool {
	r, _, _ := pGetAsyncKeyState.Call(vk)
	return r&0x8000 != 0
}

func swHookProc(code, wparam, lparam uintptr) uintptr {
	if int32(code) >= 0 && cfg.AppSwitcher && barHwnd != 0 {
		k := (*kbdHook)(at(lparam))
		if k.Flags&0x10 == 0 { // pas une touche que nous avons simulée
			down := wparam == 0x100 || wparam == 0x104
			post := func(a uintptr) { pPostMessage.Call(barHwnd, swMsg, a, 0) }
			switch {
			case down && k.VK == 0x09 /*TAB*/ && k.Flags&0x20 != 0 && !swKeyDown(0x11 /*CTRL*/) && !menuOpen && !dialogOpen:
				switch {
				case !swActive:
					swActive = true
					post(swActOpen)
					if swKeyDown(0x10) {
						post(swActPrev)
					}
				case swKeyDown(0x10 /*SHIFT*/):
					post(swActPrev)
				default:
					post(swActNext)
				}
				return 1
			case swActive && down && (k.VK == vkRight || k.VK == vkDown):
				post(swActNext)
				return 1
			case swActive && down && (k.VK == vkLeft || k.VK == vkUp):
				post(swActPrev)
				return 1
			case swActive && down && k.VK == vkEscape:
				swActive = false
				post(swActCancel)
				return 1
			case swActive && down && k.VK == 'Q':
				post(swActQuit)
				return 1
			case swActive && !down && (k.VK == 0xA4 || k.VK == 0xA5 || k.VK == 0x12): // Alt relâché
				swActive = false
				post(swActCommit)
				// Un petit appui sur Ctrl empêche Windows d'activer le menu de l'application au relâchement d'Alt.
				pKeybdEvent.Call(0x11, 0, 0, 0)
				pKeybdEvent.Call(0x11, 0, 2, 0)
			}
		}
	}
	r, _, _ := pCallNextHookEx.Call(0, code, wparam, lparam)
	return r
}

// setSwitcherHook installe ou retire le crochet clavier selon le réglage.
func setSwitcherHook(on bool) {
	switch {
	case on && swHook == 0:
		hInst, _, _ := pGetModuleHandle.Call(0)
		swHook, _, _ = pSetWindowsHookEx.Call(whKeyboardLL, swHookCB, hInst, 0)
	case !on && swHook != 0:
		pUnhookWindowsHookEx.Call(swHook)
		swHook = 0
		swActive = false
		swClose(false)
	}
}

// swBuild liste les applications ouvertes, la plus récente d'abord (ordre d'empilement).
func swBuild() []*dockApp {
	refresh()
	var apps []*dockApp
	index := map[string]*dockApp{}
	for _, w := range found {
		path := cachedProcPath(w.hwnd)
		if path == "" {
			continue
		}
		frame := strings.EqualFold(baseName(path), "ApplicationFrameHost.exe")
		key := normKey(path)
		if frame {
			key = "frame:" + itoaHex(w.hwnd)
		}
		if a := index[key]; a != nil {
			a.hwnds = append(a.hwnds, w.hwnd)
			continue
		}
		a := &dockApp{key: key, path: path, launch: path, frame: frame, hwnds: []uintptr{w.hwnd}, spriteKey: key}
		if frame {
			a.label = w.title
		} else {
			a.label = appLabel(path)
		}
		index[key] = a
		apps = append(apps, a)
	}
	return apps
}

func swLayout(n int) {
	m := monitorRect(cursorPt().X, cursorPt().Y)
	sw.tile, sw.pad = sc(92), sc(16)
	maxW := (m.Right - m.Left) * 9 / 10
	for n > 0 && int32(n)*sw.tile+2*sw.pad > maxW && sw.tile > sc(60) {
		sw.tile -= sc(4)
	}
	sw.w = int32(n)*sw.tile + 2*sw.pad
	if sw.w > maxW {
		sw.w = maxW
	}
	sw.h = sw.tile + 2*sw.pad + sc(26)
}

func swTileRect(i int) rect {
	x := sw.pad + int32(i)*sw.tile
	return rect{x, sw.pad, x + sw.tile, sw.pad + sw.tile}
}

func swDraw(hdc uintptr, w, h int32) {
	full := rect{0, 0, w, h}
	pFillRect.Call(hdc, uintptr(unsafe.Pointer(&full)), brushMenuBg)
	pSetBkMode.Call(hdc, transparent)
	icon := sw.tile - sc(20)
	for i, a := range sw.items {
		r := swTileRect(i)
		if r.Right > w-sw.pad/2 {
			break
		}
		if i == sw.sel {
			fillRR(hdc, r, sc(18), blendColor(pal.menuBg, pal.menuText, 0.18))
		} else if i == sw.hovered {
			fillRR(hdc, r, sc(18), blendColor(pal.menuBg, pal.menuText, 0.08))
		}
		blitSprite(hdc, appSprite(a), r.Left+(sw.tile-icon)/2, r.Top+(sw.tile-icon)/2, icon)
	}
	if sw.sel >= 0 && sw.sel < len(sw.items) {
		pSelectObject.Call(hdc, fontBold)
		pSetTextColor.Call(hdc, pal.menuText)
		drawText(hdc, sw.items[sw.sel].label, rect{sw.pad, h - sw.pad - sc(22), w - sw.pad, h - sw.pad + sc(2)}, dtCenter|dtEllipsis)
	}
}

func swProc(hwnd, message, wparam, lparam uintptr) uintptr {
	if hwnd == sw.hwnd && sw.hwnd != 0 {
		switch uint32(message) {
		case wmEraseBkgnd:
			return 1
		case wmPaint:
			paintBuffered(hwnd, swDraw)
			return 0
		case wmTimer:
			// Sécurité : Alt a été relâché sans que le crochet le voie.
			if !swKeyDown(0x12) {
				swActive = false
				swClose(true)
			}
			return 0
		case wmMouseMove:
			x, y := lparamXY(lparam)
			hv := -1
			for i := range sw.items {
				if inRect(swTileRect(i), x, y) {
					hv = i
				}
			}
			if hv != sw.hovered {
				sw.hovered = hv
				pInvalidateRect.Call(hwnd, 0, 0)
			}
			return 0
		case wmLButtonUp:
			x, y := lparamXY(lparam)
			for i := range sw.items {
				if inRect(swTileRect(i), x, y) {
					sw.sel = i
					swActive = false
					swClose(true)
					return 0
				}
			}
			return 0
		}
	}
	r, _, _ := pDefWindowProc.Call(hwnd, message, wparam, lparam)
	return r
}

func swOpen() {
	if sw.hwnd != 0 {
		return
	}
	items := swBuild()
	if len(items) == 0 {
		swActive = false
		return
	}
	initPanelFonts()
	hideLabel()
	if !swClassOK {
		hInst, _, _ := pGetModuleHandle.Call(0)
		cursor, _, _ := pLoadCursor.Call(0, idcArrow)
		wc := wndClassEx{WndProc: syscall.NewCallback(swProc), Instance: hInst, Cursor: cursor, ClassName: utf16(swClass)}
		wc.Size = uint32(unsafe.Sizeof(wc))
		pRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
		swClassOK = true
	}
	sw.items, sw.hovered = items, -1
	sw.sel = 0
	if len(items) > 1 {
		sw.sel = 1 // comme sur macOS : la première frappe va à l'application précédente
	}
	swLayout(len(items))
	m := monitorRect(cursorPt().X, cursorPt().Y)
	x := m.Left + (m.Right-m.Left-sw.w)/2
	y := m.Top + (m.Bottom-m.Top-sw.h)/2
	hInst, _, _ := pGetModuleHandle.Call(0)
	hwnd, _, _ := pCreateWindowEx.Call(wsExTopmost|wsExToolwin|wgExNoActive, ptr(utf16(swClass)), ptr(utf16("Switcher")), wsPopup,
		uintptr(x), uintptr(y), uintptr(sw.w), uintptr(sw.h), 0, 0, hInst, 0)
	if hwnd == 0 {
		swActive = false
		return
	}
	sw.hwnd = hwnd
	styleWindow(hwnd, sw.w, sw.h)
	pShowWindow.Call(hwnd, swShowNoActivate)
	pSetTimer.Call(hwnd, swTimerID, 150, 0)
}

func swStep(d int) {
	if sw.hwnd == 0 || len(sw.items) == 0 {
		return
	}
	sw.sel = (sw.sel + d + len(sw.items)) % len(sw.items)
	pInvalidateRect.Call(sw.hwnd, 0, 0)
}

// swClose ferme le panneau ; avec activate, l'application choisie passe devant.
func swClose(activate bool) {
	h := sw.hwnd
	if h == 0 {
		return
	}
	var target uintptr
	if activate && sw.sel >= 0 && sw.sel < len(sw.items) && len(sw.items[sw.sel].hwnds) > 0 {
		target = sw.items[sw.sel].hwnds[0]
	}
	sw.hwnd = 0
	pKillTimer.Call(h, swTimerID)
	pDestroyWindow.Call(h)
	sw.items = nil
	if target != 0 {
		bringToFront(target)
	}
}

// swQuit demande à l'application choisie de se fermer (comme Cmd+Q) et la retire de la liste.
func swQuit() {
	if sw.hwnd == 0 || sw.sel < 0 || sw.sel >= len(sw.items) {
		return
	}
	a := sw.items[sw.sel]
	for _, h := range a.hwnds {
		pPostMessage.Call(h, wmClose, 0, 0)
	}
	sw.items = append(sw.items[:sw.sel], sw.items[sw.sel+1:]...)
	if len(sw.items) == 0 {
		swActive = false
		swClose(false)
		return
	}
	if sw.sel >= len(sw.items) {
		sw.sel = 0
	}
	swLayout(len(sw.items))
	pSetWindowPos.Call(sw.hwnd, 0, 0, 0, uintptr(sw.w), uintptr(sw.h), wgSwpNoMove|swpNoActivate|swpNoZOrder)
	m := monitorRect(cursorPt().X, cursorPt().Y)
	pSetWindowPos.Call(sw.hwnd, 0, uintptr(m.Left+(m.Right-m.Left-sw.w)/2), uintptr(m.Top+(m.Bottom-m.Top-sw.h)/2), 0, 0, wgSwpNoSize|swpNoActivate|swpNoZOrder)
	styleWindow(sw.hwnd, sw.w, sw.h)
	pInvalidateRect.Call(sw.hwnd, 0, 0)
}

// swHandle traite les messages postés par le crochet.
func swHandle(action uintptr) {
	switch action {
	case swActOpen:
		swOpen()
	case swActNext:
		swStep(1)
	case swActPrev:
		swStep(-1)
	case swActCommit:
		swClose(true)
	case swActCancel:
		swClose(false)
	case swActQuit:
		swQuit()
	}
}

func baseName(p string) string {
	if i := strings.LastIndexAny(p, `\/`); i >= 0 {
		return p[i+1:]
	}
	return p
}

func itoaHex(v uintptr) string {
	const digits = "0123456789abcdef"
	if v == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for v > 0 {
		i--
		b[i] = digits[v&15]
		v >>= 4
	}
	return string(b[i:])
}
