//go:build windows

package main

// Verre dépoli du Dock : on copie le décor sous la fenêtre du Dock (sans le Dock lui-même,
// qui est une fenêtre transparente), on le floute, et dockgfx le peint dans la barre.

import (
	"slices"
	"time"

	"menuwin/internal/barblur"
	"menuwin/internal/dockgfx"
)

var dockBD struct {
	bits         []uint32
	x, y, w, h   int32
	at           time.Time
	shadowByName map[string]*dockgfx.Sprite
}

// dockBackdropUpdate recapture le décor (au plus une fois par 700 ms, ou si la fenêtre a bougé).
// Renvoie true si l'image a changé.
func dockBackdropUpdate(force bool) bool {
	if !cfg.DockGlass || dockHwnd == 0 || dockW <= 0 || dockH <= 0 {
		dockBD.bits = nil
		return false
	}
	moved := dockBD.x != dockX || dockBD.y != dockY || dockBD.w != dockW || dockBD.h != dockH
	if !force && !moved && time.Since(dockBD.at) < 700*time.Millisecond {
		return false
	}
	dockBD.at = time.Now()
	dc, bmp, old, bits := newDIB(dockW, dockH)
	if bits == nil {
		return false
	}
	defer freeDIBParts(dc, bmp, old)
	screen, _, _ := pGetDC.Call(0)
	defer pReleaseDC.Call(0, screen)
	if ok, _, _ := pBitBlt.Call(dc, 0, 0, uintptr(dockW), uintptr(dockH), screen, uintptr(dockX), uintptr(dockY), srcCopy); ok == 0 {
		return false
	}
	for i := range bits {
		bits[i] |= 0xFF000000
	}
	blurred := barblur.Blur(bits, int(dockW), int(dockH), 4, 3)
	same := !moved && slices.Equal(blurred, dockBD.bits)
	dockBD.bits, dockBD.x, dockBD.y, dockBD.w, dockBD.h = blurred, dockX, dockY, dockW, dockH
	return !same
}

func dockStyle() dockgfx.Style {
	st := dockgfx.Style{Glass: cfg.DockGlass, Dark: pal.dark, Orient: dockOrient()}
	if cfg.DockGlass && len(dockBD.bits) == int(dockW)*int(dockH) {
		st.Backdrop = dockBD.bits
	}
	return st
}

// dockShadow renvoie (et garde) l'ombre de l'icône.
func dockShadow(key string, s *dockgfx.Sprite) *dockgfx.Sprite {
	if dockBD.shadowByName == nil {
		dockBD.shadowByName = map[string]*dockgfx.Sprite{}
	}
	if sh, ok := dockBD.shadowByName[key]; ok {
		return sh
	}
	sh := dockgfx.MakeShadow(s)
	dockBD.shadowByName[key] = sh
	return sh
}

var dockPlatesApplied bool

// dockOrient donne le bord de l'écran où se trouve le Dock.
func dockOrient() dockgfx.Orient {
	switch cfg.DockPosition {
	case "left":
		return dockgfx.Left
	case "right":
		return dockgfx.Right
	}
	return dockgfx.Bottom
}

// dockEngDims donne les dimensions du repère « Dock en bas » de la fenêtre actuelle.
func dockEngDims() (int, int) { return dockOrient().Engine(int(dockW), int(dockH)) }

// dockWinRect passe un rectangle du moteur dans le repère de la fenêtre.
func dockWinRect(r dockgfx.Rect) dockgfx.Rect {
	we, he := dockEngDims()
	return dockOrient().Rect(r, float64(we), float64(he))
}
