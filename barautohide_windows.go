//go:build windows

package main

// Masquage automatique de la barre du haut, comme l'option de macOS : la barre ne réserve
// plus de place, glisse hors de l'écran, et redescend quand la souris touche le bord haut.
// Elle reste affichée tant qu'un menu ou un panneau (Spotlight, Centre de contrôle,
// calendrier…) est ouvert.

import (
	"time"
	"unsafe"
)

const barSlideTimerID = 8

var barAuto struct {
	slide    float64 // 0 : visible, 1 : entièrement remontée hors de l'écran
	target   float64
	leaveAt  time.Time
	timerOn  bool
	prevAuto bool
}

func barPanelsOpen() bool {
	return menuOpen || dialogOpen || sp.hwnd != 0 || cc.hwnd != 0 || cal.hwnd != 0 || about.hwnd != 0 || lp.hwnd != 0
}

// barPlaceSlid met la fenêtre de la barre à sa position selon le glissement.
func barPlaceSlid() {
	if barHwnd == 0 {
		return
	}
	sw, _, _ := pGetSystemMetrics.Call(smCxScreen)
	y := -int32(barAuto.slide*float64(barH) + 0.5)
	pSetWindowPos.Call(barHwnd, ^uintptr(0), 0, uintptr(y), sw, uintptr(barH), swpNoActivate|swpShowWindow)
}

func barSlideStart() {
	if barAuto.timerOn || barHwnd == 0 {
		return
	}
	barAuto.timerOn = true
	pSetTimer.Call(barHwnd, barSlideTimerID, 16, 0)
}

// barSlideTick avance l'animation de glissement.
func barSlideTick() {
	d := barAuto.target - barAuto.slide
	if d > -0.02 && d < 0.02 {
		barAuto.slide = barAuto.target
		pKillTimer.Call(barHwnd, barSlideTimerID)
		barAuto.timerOn = false
	} else {
		barAuto.slide += d * 0.35
	}
	barPlaceSlid()
}

// barRevealNow montre la barre tout de suite (raccourci clavier, icône de la zone de notification).
func barRevealNow() {
	if !cfg.BarAutoHide || barAuto.slide == 0 {
		return
	}
	barAuto.slide, barAuto.target = 0, 0
	barAuto.leaveAt = time.Time{}
	barPlaceSlid()
}

// barAutoTick est appelé toutes les 100 ms : la souris est-elle au bord haut ?
func barAutoTick() {
	if !cfg.BarAutoHide || barHwnd == 0 {
		return
	}
	p := cursorPt()
	m := monitorRect(0, 0)
	sw, _, _ := pGetSystemMetrics.Call(smCxScreen)
	atEdge := p.Y <= m.Top && p.X >= 0 && p.X < int32(sw)
	onBar := barAuto.slide < 0.5 && p.Y < m.Top+barH+sc(4)
	want := atEdge || onBar || barPanelsOpen()
	if want && fullscreenBusy() && !barPanelsOpen() {
		want = false // pas de barre par-dessus un jeu ou une présentation en plein écran
	}
	if want {
		barAuto.leaveAt = time.Time{}
		if barAuto.target != 0 {
			barAuto.target = 0
			barSlideStart()
		}
		return
	}
	if barAuto.target == 1 {
		return
	}
	if barAuto.leaveAt.IsZero() {
		barAuto.leaveAt = time.Now()
		return
	}
	if time.Since(barAuto.leaveAt) > 350*time.Millisecond {
		barAuto.target = 1
		barSlideStart()
	}
}

// fullscreenBusy : un jeu en plein écran ou une présentation est en cours.
func fullscreenBusy() bool {
	if pSHQueryUserNotificationState.Find() != nil {
		return false
	}
	hotQuns = 0
	pSHQueryUserNotificationState.Call(uintptr(unsafe.Pointer(&hotQuns)))
	return hotQuns == 3 || hotQuns == 4
}

// applyBarAutoHide réagit au changement du réglage : la barre libère ou reprend sa place.
func applyBarAutoHide() {
	if barHwnd == 0 || cfg.BarAutoHide == barAuto.prevAuto {
		return
	}
	barAuto.prevAuto = cfg.BarAutoHide
	barAuto.slide, barAuto.target, barAuto.leaveAt = 0, 0, time.Time{}
	if cfg.BarAutoHide {
		removeAppBar(barHwnd)
		barPlaceSlid()
	} else {
		if barAuto.timerOn {
			pKillTimer.Call(barHwnd, barSlideTimerID)
			barAuto.timerOn = false
		}
		registerAppBar(barHwnd)
	}
	hotTimerSync()
}
