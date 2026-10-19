//go:build windows

package main

// Barre des tâches de Windows : on la bloque pour de bon (réglage « Bloquer la
// barre des tâches Windows », activé par défaut) pour laisser la place au Dock.
// Elle est mise en masquage automatique, ce qui libère sa place à l'écran, puis
// cachée ; et comme l'Explorateur essaie de la faire remonter dès que la souris
// touche le bord bas (ou que la touche Windows est pressée), un minuteur rapide
// (toutes les 20 ms) la recache aussitôt : elle ne devient jamais visible plus
// d'un instant. Elle est rétablie telle qu'elle était à la fermeture du
// programme ou quand on décoche le réglage.

import "unsafe"

const (
	abmGetState = 4
	abmSetState = 10
	absAutoHide = 1
)

// taskbarTimerID : minuteur (sur la fenêtre de la barre) qui recache la barre des tâches.
const (
	taskbarTimerID = 10
	taskbarTickMs  = 20
)

var (
	pFindWindowEx = user32.NewProc("FindWindowExW")

	taskbar struct {
		saved     bool
		origState uintptr
		hidden    bool
	}
)

// taskbarWindows renvoie la barre principale et les barres des autres écrans.
func taskbarWindows() []uintptr {
	var out []uintptr
	for _, class := range []string{"Shell_TrayWnd", "Shell_SecondaryTrayWnd"} {
		var prev uintptr
		for {
			h, _, _ := pFindWindowEx.Call(0, prev, ptr(utf16(class)), 0)
			if h == 0 {
				break
			}
			out = append(out, h)
			prev = h
		}
	}
	return out
}

func taskbarState(set bool, state uintptr) uintptr {
	abd := appBarData{HWnd: barHwnd, LParam: state}
	abd.Size = uint32(unsafe.Sizeof(abd))
	msg := uintptr(abmGetState)
	if set {
		msg = abmSetState
	}
	r, _, _ := pSHAppBarMessage.Call(msg, uintptr(unsafe.Pointer(&abd)))
	return r
}

// applyTaskbar met la barre des tâches dans l'état voulu par les réglages.
func applyTaskbar() {
	if cfg.HideTaskbar {
		if !taskbar.hidden {
			if !taskbar.saved {
				taskbar.origState, taskbar.saved = taskbarState(false, 0), true
			}
			taskbarState(true, taskbar.origState|absAutoHide) // libère la place à l'écran
			taskbar.hidden = true
		}
		if barHwnd != 0 {
			pSetTimer.Call(barHwnd, taskbarTimerID, taskbarTickMs, 0)
		}
		keepTaskbarHidden()
		keepTaskbarAutoHide()
	} else if taskbar.hidden {
		restoreTaskbar()
	}
	if barHwnd != 0 {
		positionBar(barHwnd)
	}
}

// keepTaskbarHidden recache la barre si Windows l'a réaffichée (souris au bord bas,
// touche Windows, redémarrage de l'Explorateur…). Appelé toutes les 20 ms.
func keepTaskbarHidden() {
	if !taskbar.hidden {
		return
	}
	for _, h := range taskbarWindows() {
		if v, _, _ := pIsWindowVisible.Call(h); v != 0 {
			pShowWindow.Call(h, 0 /*SW_HIDE*/)
		}
	}
}

// keepTaskbarAutoHide remet la barre en masquage automatique si l'Explorateur a
// retrouvé ses réglages (il redémarre, ou l'utilisateur a changé l'option dans
// Windows) : sans cela sa place à l'écran serait de nouveau réservée.
func keepTaskbarAutoHide() {
	if !taskbar.hidden {
		return
	}
	if taskbarState(false, 0)&absAutoHide == 0 {
		taskbarState(true, taskbar.origState|absAutoHide)
		if barHwnd != 0 {
			positionBar(barHwnd)
		}
	}
}

// restoreTaskbar rend la barre des tâches comme on l'a trouvée.
func restoreTaskbar() {
	if !taskbar.hidden {
		return
	}
	taskbar.hidden = false
	if barHwnd != 0 {
		pKillTimer.Call(barHwnd, taskbarTimerID)
	}
	taskbarState(true, taskbar.origState)
	for _, h := range taskbarWindows() {
		pShowWindow.Call(h, 5 /*SW_SHOW*/)
	}
}
