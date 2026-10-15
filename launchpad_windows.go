//go:build windows

package main

import (
	"syscall"
	"unsafe"

	"menuwin/internal/dockgfx"
)

const (
	launchpadClass = "MenuWindowsLaunchpad"
)

type launchpadApp struct {
	label string
	path  string
	sprite *dockgfx.Sprite
}

var (
	launchpadHwnd uintptr
	launchpadOpen bool
	launchpadApps []launchpadApp
	lpHover       = -1
	lpCols        = 6
	lpIconSize    = 64
)

func initLaunchpad() {
	registerLaunchpadClass()
	hInst, _, _ := pGetModuleHandle.Call(0)
	sw, _, _ := pGetSystemMetrics.Call(smCxScreen)
	sh, _, _ := pGetSystemMetrics.Call(smCyScreen)

	launchpadHwnd, _, _ = pCreateWindowEx.Call(
		wsExTopmost|wsExToolwin,
		ptr(utf16(launchpadClass)),
		ptr(utf16("Launchpad")),
		wsPopup,
		0, 0, sw, sh,
		0, 0, hInst, 0,
	)
}

func toggleLaunchpad() {
	if launchpadHwnd == 0 {
		initLaunchpad()
	}
	if launchpadOpen {
		pShowWindow.Call(launchpadHwnd, 0) // SW_HIDE
		launchpadOpen = false
	} else {
		loadLaunchpadApps()
		sw, _, _ := pGetSystemMetrics.Call(smCxScreen)
		sh, _, _ := pGetSystemMetrics.Call(smCyScreen)
		pSetWindowPos.Call(launchpadHwnd, ^uintptr(0), 0, 0, sw, sh, swpShowWindow)
		pShowWindow.Call(launchpadHwnd, swShow)
		pSetForeground.Call(launchpadHwnd)
		pSetFocus.Call(launchpadHwnd)
		launchpadOpen = true
	}
}

func loadLaunchpadApps() {
	if len(launchpadApps) > 0 {
		return
	}
	raw := scanApps()
	for _, a := range raw {
		s := spriteForPath(a.path)
		if s == nil {
			s = defaultSprite()
		}
		launchpadApps = append(launchpadApps, launchpadApp{
			label:  a.name,
			path:   a.path,
			sprite: s,
		})
	}
}

func launchpadProc(hwnd, message, wparam, lparam uintptr) uintptr {
	switch uint32(message) {
	case wmEraseBkgnd:
		return 1

	case wmPaint:
		paintBuffered(hwnd, func(mem uintptr, w, h int32) {
			r := rect{0, 0, w, h}
			// Fond très sombre semi-translucide
			pFillRect.Call(mem, uintptr(unsafe.Pointer(&r)), brushMenuBg)
			pSetBkMode.Call(mem, transparent)
			pSelectObject.Call(mem, fontReg)

			if len(launchpadApps) == 0 {
				return
			}

			// Dessiner la grille d'applications
			paddingX := int32(sc(80))
			paddingY := int32(sc(100))
			cellW := (w - 2*paddingX) / int32(lpCols)
			cellH := int32(sc(110))

			for i, app := range launchpadApps {
				col := int32(i % lpCols)
				row := int32(i / lpCols)

				x := paddingX + col*cellW
				y := paddingY + row*cellH

				if y+cellH > h {
					break // ne pas dépasser l'écran
				}

				iconX := x + (cellW-int32(lpIconSize))/2
				iconY := y + sc(10)

				// Surlignement au survol
				if i == lpHover {
					hoverRect := rect{x + sc(10), y, x + cellW - sc(10), y + cellH - sc(5)}
					fillRound(mem, hoverRect, sc(12), brushHover)
				}

				// Icône
				if app.sprite != nil {
					drawSpriteOnDC(mem, app.sprite, iconX, iconY, int32(lpIconSize), int32(lpIconSize))
				}

				// Texte
				pSetTextColor.Call(mem, rgb(245, 245, 247))
				textRect := rect{x, iconY + int32(lpIconSize) + sc(6), x + cellW, y + cellH}
				drawText(mem, app.label, textRect, dtCenter|dtEllipsis)
			}
		})
		return 0

	case wmMouseMove:
		x, y := lparamXY(lparam)
		sw, _, _ := pGetSystemMetrics.Call(smCxScreen)
		paddingX := int32(sc(80))
		paddingY := int32(sc(100))
		cellW := (int32(sw) - 2*paddingX) / int32(lpCols)
		cellH := int32(sc(110))

		newHover := -1
		if x >= paddingX && x < int32(sw)-paddingX && y >= paddingY {
			col := (x - paddingX) / cellW
			row := (y - paddingY) / cellH
			idx := int(row*int32(lpCols) + col)
			if idx >= 0 && idx < len(launchpadApps) {
				newHover = idx
			}
		}
		if newHover != lpHover {
			lpHover = newHover
			pInvalidateRect.Call(hwnd, 0, 0)
		}
		return 0

	case wmLButtonUp:
		if lpHover >= 0 && lpHover < len(launchpadApps) {
			app := launchpadApps[lpHover]
			toggleLaunchpad()
			openURI(app.path)
		} else {
			toggleLaunchpad()
		}
		return 0

	case wmKeyDown:
		if wparam == vkEscape {
			toggleLaunchpad()
		}
		return 0
	}
	r, _, _ := pDefWindowProc.Call(hwnd, message, wparam, lparam)
	return r
}

func drawSpriteOnDC(hdc uintptr, s *dockgfx.Sprite, x, y, w, h int32) {
	if s == nil || s.W == 0 || s.H == 0 {
		return
	}
	// Création d'un tampon DIB temporaire pour le sprite
	mem, _, _ := pCreateCompatDC.Call(hdc)
	bi := bitmapInfoHeader{Width: int32(s.W), Height: -int32(s.H), Planes: 1, BitCount: 32}
	bi.Size = uint32(unsafe.Sizeof(bi))
	var bits unsafe.Pointer
	bmp, _, _ := pCreateDIBSection.Call(hdc, uintptr(unsafe.Pointer(&bi)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if bmp != 0 && bits != nil {
		old, _, _ := pSelectObject.Call(mem, bmp)
		pix := unsafe.Slice((*uint32)(bits), s.W*s.H)
		copy(pix, s.Pix)
		bf := blendFunction{SourceConstantAlpha: 255, AlphaFormat: 1}
		pUpdateLayeredWindow.Call(0) // dummy check or use AlphaBlend
		// Fallback simple: BitBlt or StretchBlt
		pBitBlt.Call(hdc, uintptr(x), uintptr(y), uintptr(w), uintptr(h), mem, 0, 0, srcCopy)
		pSelectObject.Call(mem, old)
		pDeleteObject.Call(bmp)
	}
	pDeleteDC.Call(mem)
}

func registerLaunchpadClass() {
	hInst, _, _ := pGetModuleHandle.Call(0)
	cursor, _, _ := pLoadCursor.Call(0, idcArrow)
	wc := wndClassEx{
		WndProc:   syscall.NewCallback(launchpadProc),
		Instance:  hInst,
		Cursor:    cursor,
		ClassName: utf16(launchpadClass),
	}
	wc.Size = uint32(unsafe.Sizeof(wc))
	pRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
}
