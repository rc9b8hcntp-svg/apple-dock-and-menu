//go:build windows

package main

// Coins d'écran arrondis, comme sur un Mac : quatre petites fenêtres noires en forme
// de coin, au-dessus de tout, qui laissent passer les clics. Les deux du haut se posent
// juste sous la barre des menus.

import (
	"syscall"
	"unsafe"
)

const (
	cornerClass = "MenuWindowsCorner"
	rgnDiff     = 4
)

var (
	cornerWins [4]uintptr
	cornerOK   bool
)

func cornerProc(hwnd, message, wparam, lparam uintptr) uintptr {
	r, _, _ := pDefWindowProc.Call(hwnd, message, wparam, lparam)
	return r
}

func closeCorners() {
	for i, h := range cornerWins {
		if h != 0 {
			pDestroyWindow.Call(h)
			cornerWins[i] = 0
		}
	}
}

// applyCorners crée, replace ou retire les quatre coins selon le réglage.
func applyCorners() {
	if !cfg.ScreenCorners {
		closeCorners()
		return
	}
	if !cornerOK {
		hInst, _, _ := pGetModuleHandle.Call(0)
		black, _, _ := pGetStockObject.Call(4 /*BLACK_BRUSH*/)
		wc := wndClassEx{WndProc: syscall.NewCallback(cornerProc), Instance: hInst, Background: black, ClassName: utf16(cornerClass)}
		wc.Size = uint32(unsafe.Sizeof(wc))
		pRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
		cornerOK = true
	}
	m := monitorRect(0, 0)
	rad := sc(10)
	top := m.Top + barH
	pos := [4][2]int32{
		{m.Left, top},
		{m.Right - rad, top},
		{m.Left, m.Bottom - rad},
		{m.Right - rad, m.Bottom - rad},
	}
	hInst, _, _ := pGetModuleHandle.Call(0)
	for i := range cornerWins {
		if cornerWins[i] == 0 {
			h, _, _ := pCreateWindowEx.Call(wsExTopmost|wsExToolwin|wgExLayered|wgExTransp|wgExNoActive,
				ptr(utf16(cornerClass)), ptr(utf16("Corner")), wsPopup,
				uintptr(pos[i][0]), uintptr(pos[i][1]), uintptr(rad), uintptr(rad), 0, 0, hInst, 0)
			if h == 0 {
				continue
			}
			cornerWins[i] = h
			pSetLayeredWindowAttributes.Call(h, 0, 255, lwaAlpha)
			// Le coin : un carré moins un disque centré sur le coin de l'écran.
			sq, _, _ := pCreateRectRgn.Call(0, 0, uintptr(rad), uintptr(rad))
			cx, cy := int32(0), int32(0)
			if i%2 == 1 {
				cx = rad
			}
			if i >= 2 {
				cy = rad
			}
			disc, _, _ := pCreateRoundRectRgn.Call(uintptr(cx-rad), uintptr(cy-rad), uintptr(cx+rad+1), uintptr(cy+rad+1), uintptr(2*rad), uintptr(2*rad))
			pCombineRgn.Call(sq, sq, disc, rgnDiff)
			pDeleteObject.Call(disc)
			pSetWindowRgn.Call(h, sq, 1)
			pShowWindow.Call(h, swShowNoActivate)
		}
		pSetWindowPos.Call(cornerWins[i], ^uintptr(0) /*HWND_TOPMOST*/, uintptr(pos[i][0]), uintptr(pos[i][1]), uintptr(rad), uintptr(rad), swpNoActivate)
	}
}
