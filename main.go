//go:build windows

// Menu Windows : un menu système façon « menu Apple » pour Windows.
//
// Une barre fine reste en haut de l'écran :
//   - à gauche, le bouton du menu système (éteindre, redémarrer, verrouiller…)
//   - le menu Applications (le menu Démarrer, classé par lettre)
//   - les fenêtres ouvertes, cliquables, comme une barre des tâches
//   - à droite, la date et l'heure
//
// Le menu s'ouvre aussi depuis l'icône de la zone de notification et avec
// le raccourci Ctrl+Alt+M.
package main

import (
	"runtime"
	"syscall"
	"unsafe"
)

var (
	trayAdded bool
	nid       notifyIconData
)

func addTrayIcon(hwnd uintptr) {
	icon, _, _ := pLoadIcon.Call(0, idiApp)
	nid = notifyIconData{
		HWnd:            hwnd,
		ID:              1,
		Flags:           nifMessage | nifIcon | nifTip,
		CallbackMessage: wmTray,
		Icon:            icon,
	}
	nid.Size = uint32(unsafe.Sizeof(nid))
	tip, _ := syscall.UTF16FromString(t.tip)
	copy(nid.Tip[:], tip)
	r, _, _ := pShellNotifyIcon.Call(nimAdd, uintptr(unsafe.Pointer(&nid)))
	trayAdded = r != 0
}

// openFromBar ouvre un menu sous un bouton de la barre en l'affichant « enfoncé ».
func openFromBar(hwnd uintptr, kind int, r rect) {
	x, y := screenPos(hwnd, r)
	pressed = kind
	invalidate(hwnd)
	switch kind {
	case hitMenu:
		showMainMenu(hwnd, x, y)
	case hitApps:
		showAppsMenu(hwnd, x, y)
	}
	pressed = 0
	invalidate(hwnd)
}

func wndProc(hwnd, message, wparam, lparam uintptr) uintptr {
	switch uint32(message) {
	case wmEraseBkgnd:
		return 1

	case wmPaint:
		paintBar(hwnd)
		return 0

	case wmTimer:
		c1 := refresh()
		c2 := updateClock()
		if c1 || c2 {
			invalidate(hwnd)
		}
		return 0

	case wmMouseMove:
		x, y := lparamXY(lparam)
		if i := hitAt(x, y); i != hoverIdx {
			hoverIdx = i
			invalidate(hwnd)
		}
		trackLeave(hwnd)
		return 0

	case wmMouseLeave:
		hoverIdx, tracking = -1, false
		invalidate(hwnd)
		return 0

	case wmLButtonUp:
		x, y := lparamXY(lparam)
		i := hitAt(x, y)
		if i < 0 {
			return 0
		}
		h := hits[i]
		switch h.kind {
		case hitMenu, hitApps:
			openFromBar(hwnd, h.kind, h.r)
		case hitWin:
			activateWindow(h.hwnd)
			refresh()
			invalidate(hwnd)
		}
		return 0

	case wmRButtonUp:
		x, y := lparamXY(lparam)
		i := hitAt(x, y)
		if i >= 0 && hits[i].kind == hitWin {
			px, py := screenPos(hwnd, hits[i].r)
			showWindowMenu(hwnd, hits[i].hwnd, px, py)
			refresh()
			invalidate(hwnd)
		}
		return 0

	case wmHotkey:
		if len(hits) > 0 {
			openFromBar(hwnd, hitMenu, hits[0].r)
		}
		return 0

	case wmTray:
		switch uint32(lparam) {
		case wmLButtonUp, wmRButtonUp:
			var p point
			pGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
			showMainMenu(hwnd, p.X, p.Y)
		}
		return 0

	case wmAppBar:
		switch wparam {
		case abnPosChg:
			positionBar(hwnd)
		case abnFullScr:
			// Une application passe en plein écran : la barre s'efface derrière.
			if lparam != 0 {
				pSetWindowPos.Call(hwnd, 1 /*HWND_BOTTOM*/, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate)
			} else {
				pSetWindowPos.Call(hwnd, ^uintptr(0) /*HWND_TOPMOST*/, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate)
			}
		}
		return 0

	case wmDisplayChange:
		positionBar(hwnd)
		return 0

	case wmClose:
		pDestroyWindow.Call(hwnd)
		return 0

	case wmDestroy:
		pKillTimer.Call(hwnd, 1)
		pUnregisterHotKey.Call(hwnd, 1)
		removeAppBar(hwnd)
		if trayAdded {
			pShellNotifyIcon.Call(nimDelete, uintptr(unsafe.Pointer(&nid)))
		}
		pPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := pDefWindowProc.Call(hwnd, message, wparam, lparam)
	return r
}

func main() {
	runtime.LockOSThread()

	// Une seule instance à la fois.
	pCreateMutex.Call(0, 0, ptr(utf16("Local\\MenuWindowsSingleInstance")))
	if e, _, _ := pGetLastError.Call(); e == errAlready {
		return
	}

	pSetProcessDPIAw.Call()

	lang, _, _ := pGetUILanguage.Call()
	if lang&0xFF == 0x0C { // LANG_FRENCH
		t = fr
	} else {
		t = en
	}

	initBarResources()
	updateClock()

	hInst, _, _ := pGetModuleHandle.Call(0)
	cursor, _, _ := pLoadCursor.Call(0, idcArrow)
	className := utf16("MenuWindowsBar")

	wc := wndClassEx{
		WndProc:   syscall.NewCallback(wndProc),
		Instance:  hInst,
		Cursor:    cursor,
		ClassName: className,
	}
	wc.Size = uint32(unsafe.Sizeof(wc))
	pRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))

	sw, _, _ := pGetSystemMetrics.Call(smCxScreen)
	hwnd, _, _ := pCreateWindowEx.Call(
		wsExTopmost|wsExToolwin|wsExNoActive,
		ptr(className),
		ptr(utf16(t.appTitle)),
		wsPopup,
		0, 0, sw, uintptr(barH),
		0, 0, hInst, 0)
	if hwnd == 0 {
		return
	}
	barHwnd = hwnd

	pShowWindow.Call(hwnd, swShowNoActivate)
	registerAppBar(hwnd)
	addTrayIcon(hwnd)
	pRegisterHotKey.Call(hwnd, 1, modControl|modAlt|modNoRepeat, 'M')
	refresh()
	pSetTimer.Call(hwnd, 1, 500, 0)

	var m msg
	for {
		r, _, _ := pGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		pDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
	}

	freeBarResources()
}
