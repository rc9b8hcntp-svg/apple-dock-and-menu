//go:build windows

// Menu Windows : un menu système façon « menu Apple » pour Windows.
//
// Une barre fine, façon barre des menus de macOS, reste en haut de l'écran :
//   - à gauche, le bouton du menu système (éteindre, redémarrer, verrouiller…)
//   - le nom de l'application active, en gras
//   - le menu Applications (le menu Démarrer, classé par lettre)
//   - à droite, la date et l'heure
//
// Le menu s'ouvre aussi depuis l'icône de la zone de notification et avec
// le raccourci Ctrl+Alt+M.
//
// Tous les menus et la boîte de confirmation sont dessinés par le programme
// (popup_windows.go) : aucun ancien menu Windows n'est utilisé.
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

// openFromBar ouvre le menu d'un titre de la barre en l'affichant « enfoncé ». Tant
// qu'il est ouvert, glisser la souris sur un autre titre ouvre le menu de celui-ci.
func openFromBar(hwnd uintptr, kind int, r rect) {
	for kind != 0 {
		x, y := screenPos(hwnd, r)
		pressed = kind
		mSwitch = 0
		invalidate(hwnd)
		switch kind {
		case hitMenu:
			showMainMenu(hwnd, x, y)
		case hitAppName:
			showAppMenu(hwnd, x, y)
		case hitApps:
			showAppsMenu(hwnd, x, y)
		case hitWindow:
			showWindowMenu(hwnd, x, y)
		}
		pressed = 0
		invalidate(hwnd)
		kind = mSwitch
		if kind != 0 {
			nr, ok := hitRect(kind)
			if !ok {
				break
			}
			r = nr
		}
	}
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
		if updateStatus() {
			c2 = true
		}
		if c1 || c2 {
			invalidate(hwnd)
		}
		dockRefresh() // le Dock suit les fenêtres ouvertes
		checkTheme()  // thème « Automatique » : suit le mode clair/sombre de Windows
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

	case wmLButtonDown:
		if menuOpen {
			// Clic sur la barre pendant qu'un menu est ouvert : on le ferme, sans rien rouvrir.
			ignoreNextUp = true
			cancelMenu()
			return 0
		}
		ignoreNextUp = false
		if dialogOpen {
			return 0
		}
		x, y := lparamXY(lparam)
		i := hitAt(x, y)
		if i < 0 {
			return 0
		}
		h := hits[i]
		switch h.kind {
		case hitMenu, hitAppName, hitApps, hitWindow:
			// Comme sur macOS, un menu s'ouvre dès que le bouton est enfoncé.
			openFromBar(hwnd, h.kind, h.r)
		default:
			barDown = h.kind // icône d'état : l'action part au relâchement
			pSetCapture.Call(hwnd)
			invalidate(hwnd)
		}
		return 0

	case wmLButtonUp:
		down := barDown
		if down != 0 {
			barDown = 0
			pReleaseCapture.Call()
			invalidate(hwnd)
		}
		if ignoreNextUp {
			ignoreNextUp = false
			return 0
		}
		if menuOpen {
			cancelMenu()
			return 0
		}
		if dialogOpen || down == 0 {
			return 0
		}
		x, y := lparamXY(lparam)
		if i := hitAt(x, y); i >= 0 && hits[i].kind == down {
			barAction(down)
		}
		return 0

	case wmRButtonDown:
		if menuOpen {
			ignoreNextUp = true
			cancelMenu()
		} else {
			ignoreNextUp = false
		}
		return 0

	case wmRButtonUp:
		if ignoreNextUp {
			ignoreNextUp = false
			return 0
		}
		if menuOpen {
			cancelMenu()
			return 0
		}
		if dialogOpen {
			return 0
		}
		return 0

	case wmHotkey:
		if wparam == 2 { // Ctrl+Alt+L : Launchpad
			showLaunchpad()
			return 0
		}
		if lp.hwnd != 0 {
			closeLaunchpad(true)
		}
		if menuOpen { // le raccourci ferme aussi le menu
			cancelMenu()
			return 0
		}
		if dialogOpen {
			return 0
		}
		if r, ok := hitRect(hitMenu); ok {
			openFromBar(hwnd, hitMenu, r)
		}
		return 0

	case wmTray:
		if menuOpen {
			cancelMenu()
			return 0
		}
		if dialogOpen {
			return 0
		}
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
			dockSetHidden(lparam != 0) // le Dock s'efface aussi
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
		shutdownDock()
		closeLaunchpad(false)
		setHotkeys(false)
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
	_, _, mutexErr := pCreateMutex.Call(0, 0, ptr(utf16("Local\\MenuWindowsSingleInstance")))
	if errno, ok := mutexErr.(syscall.Errno); ok && errno == errAlready {
		return
	}

	pSetProcessDPIAw.Call()

	loadConfig()
	applyLanguage()

	initBarResources()
	initMenuResources()
	rebuildTheme()
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
		wsExLayered|wsExTopmost|wsExToolwin|wsExNoActive, // « layered » : pour l'opacité de la barre
		ptr(className),
		ptr(utf16(t.appTitle)),
		wsPopup,
		0, 0, sw, uintptr(barH),
		0, 0, hInst, 0)
	if hwnd == 0 {
		return
	}
	barHwnd = hwnd
	applyBarOpacity()

	pShowWindow.Call(hwnd, swShowNoActivate)
	registerAppBar(hwnd)
	addTrayIcon(hwnd)
	setHotkeys(cfg.Hotkey)
	refresh()
	initDock()
	applied = cfg
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

	freeMenuResources()
	freeBarResources()
}
