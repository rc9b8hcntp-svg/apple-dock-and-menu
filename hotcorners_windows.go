//go:build windows

package main

// Coins actifs : quand la souris reste une fraction de seconde dans un coin de
// l'écran, l'action choisie dans les réglages se déclenche (Mission Control,
// Launchpad, Spotlight ou afficher le bureau). Elle ne se redéclenche qu'après
// que la souris a quitté le coin.

import "unsafe"

const hotTimerID = 7

var (
	pSHQueryUserNotificationState = shell32.NewProc("SHQueryUserNotificationState")

	hot struct {
		corner int // coin où se trouve la souris (0 = aucun)
		ticks  int // durée passée dans ce coin, en pas de 100 ms
		fired  bool
	}
	hotQuns int32
)

const hotDwellTicks = 3

func hotAction(corner int) string {
	switch corner {
	case 1:
		return cfg.HotTL
	case 2:
		return cfg.HotTR
	case 3:
		return cfg.HotBL
	case 4:
		return cfg.HotBR
	}
	return "none"
}

func hotAnyEnabled() bool {
	for i := 1; i <= 4; i++ {
		if a := hotAction(i); a != "" && a != "none" {
			return true
		}
	}
	return false
}

// hotCornerAt renvoie le coin (1 haut-gauche, 2 haut-droit, 3 bas-gauche, 4 bas-droit) sous le point.
func hotCornerAt(p point) int {
	m := monitorRect(p.X, p.Y)
	const edge = 1
	left, right := p.X <= m.Left+edge-1, p.X >= m.Right-edge
	top, bottom := p.Y <= m.Top+edge-1, p.Y >= m.Bottom-edge
	switch {
	case left && top:
		return 1
	case right && top:
		return 2
	case left && bottom:
		return 3
	case right && bottom:
		return 4
	}
	return 0
}

func hotRunAction(a string) {
	switch a {
	case "mission":
		sendWinKey(0x09) // Win+Tab
	case "launchpad":
		if lp.hwnd == 0 {
			showLaunchpad()
		}
	case "spotlight":
		if sp.hwnd == 0 {
			showSpotlight()
		}
	case "desktop":
		sendWinKey('D')
	}
}

var hotTimerOn bool

// hotTimerSync n'active la minuterie de 100 ms que si au moins un coin a une action.
func hotTimerSync() {
	if barHwnd == 0 {
		return
	}
	want := hotAnyEnabled() || cfg.BarAutoHide
	switch {
	case want && !hotTimerOn:
		pSetTimer.Call(barHwnd, hotTimerID, 100, 0)
	case !want && hotTimerOn:
		pKillTimer.Call(barHwnd, hotTimerID)
		hot.corner, hot.ticks, hot.fired = 0, 0, false
	}
	hotTimerOn = want
}

// hotCornersTick est appelé toutes les 100 ms par la minuterie de la barre.
func hotCornersTick() {
	barAutoTick()
	if !hotAnyEnabled() {
		hot.corner, hot.ticks, hot.fired = 0, 0, false
		return
	}
	c := hotCornerAt(cursorPt())
	if c != hot.corner {
		hot.corner, hot.ticks, hot.fired = c, 0, false
		return
	}
	if c == 0 || hot.fired {
		return
	}
	hot.ticks++
	if hot.ticks < hotDwellTicks {
		return
	}
	a := hotAction(c)
	if a == "" || a == "none" {
		return
	}
	if menuOpen || dialogOpen {
		return
	}
	if l, _, _ := pGetAsyncKeyState.Call(0x01); l&0x8000 != 0 { // glisser-déposer en cours
		return
	}
	// Pas de coin actif pendant un jeu en plein écran ou une présentation.
	if pSHQueryUserNotificationState.Find() == nil {
		hotQuns = 0
		pSHQueryUserNotificationState.Call(uintptr(unsafe.Pointer(&hotQuns)))
		if hotQuns == 3 || hotQuns == 4 {
			return
		}
	}
	hot.fired = true
	hotRunAction(a)
}
