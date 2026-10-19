//go:build windows

package main

// Barre des tâches de Windows : on peut la masquer pour de bon (réglage « Masquer
// la barre des tâches Windows »). Elle est mise en masquage automatique, ce qui
// libère sa place à l'écran, puis cachée : elle ne remonte plus quand la souris
// touche le bord bas. Elle est rétablie telle qu'elle était à la fermeture du
// programme ou quand on décoche le réglage.

import "unsafe"

const (
	abmGetState = 4
	abmSetState = 10
	absAutoHide = 1
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
		keepTaskbarHidden()
	} else if taskbar.hidden {
		restoreTaskbar()
	}
	if barHwnd != 0 {
		positionBar(barHwnd)
	}
}

// keepTaskbarHidden recache la barre si Windows l'a réaffichée (touche Windows, redémarrage de l'Explorateur…).
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

// restoreTaskbar rend la barre des tâches comme on l'a trouvée.
func restoreTaskbar() {
	if !taskbar.hidden {
		return
	}
	taskbar.hidden = false
	taskbarState(true, taskbar.origState)
	for _, h := range taskbarWindows() {
		pShowWindow.Call(h, 5 /*SW_SHOW*/)
	}
}
