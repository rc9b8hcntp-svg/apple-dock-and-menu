//go:build windows

package main

import "menuwin/internal/config"

// Thème : clair ou sombre, et couleur d'accentuation.
//
// Tous les pinceaux et stylos partagés par la barre, les menus, la boîte de
// confirmation et la fenêtre de réglages sont (re)créés ici à partir d'une
// palette, pour pouvoir changer de thème sans relancer le programme.

// cfg contient les réglages en vigueur (voir settings_windows.go).
var cfg = config.Default()

type palette struct {
	dark                       bool
	bar, barPress, barText     uintptr
	menuBg, menuBorder         uintptr
	menuSep                    uintptr
	menuText, menuKey, menuOff uintptr
	btn, btnHover              uintptr
	winBg, sideBg, card        uintptr
	cardLine, ctrlOff          uintptr
	accent, accentHi           uintptr
}

var (
	pal                 palette
	brushWin, brushSide uintptr
)

func accentRGB(name string) (uint8, uint8, uint8) {
	switch name {
	case "purple":
		return 175, 82, 222
	case "pink":
		return 255, 55, 95
	case "red":
		return 255, 69, 58
	case "orange":
		return 255, 159, 10
	case "yellow":
		return 255, 204, 0
	case "green":
		return 40, 205, 65
	case "graphite":
		return 142, 142, 147
	}
	return 10, 132, 255 // bleu
}

func mixWhite(r, g, b uint8, k float64) (uint8, uint8, uint8) {
	f := func(v uint8) uint8 { return uint8(float64(v) + (255-float64(v))*k + 0.5) }
	return f(r), f(g), f(b)
}

// systemUsesDark lit le réglage « mode sombre » de Windows.
func systemUsesDark() bool {
	v, ok := regDword(`Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`, "SystemUsesLightTheme")
	return !ok || v == 0
}

func effectiveDark() bool {
	switch cfg.Theme {
	case "light":
		return false
	case "auto":
		return systemUsesDark()
	}
	return true
}

func makePalette(dark bool, accent string) palette {
	p := palette{dark: dark}
	if dark {
		p.bar, p.barPress, p.barText = rgb(28, 28, 30), rgb(70, 70, 76), rgb(245, 245, 247)
		p.menuBg, p.menuBorder, p.menuSep = rgb(44, 44, 48), rgb(78, 78, 84), rgb(70, 70, 76)
		p.menuText, p.menuKey, p.menuOff = rgb(238, 238, 242), rgb(150, 150, 158), rgb(110, 110, 116)
		p.btn, p.btnHover = rgb(84, 84, 90), rgb(100, 100, 106)
		p.winBg, p.sideBg, p.card = rgb(30, 30, 33), rgb(40, 40, 44), rgb(46, 46, 50)
		p.cardLine, p.ctrlOff = rgb(64, 64, 70), rgb(92, 92, 98)
	} else {
		p.bar, p.barPress, p.barText = rgb(236, 236, 240), rgb(206, 206, 212), rgb(24, 24, 26)
		p.menuBg, p.menuBorder, p.menuSep = rgb(246, 246, 248), rgb(206, 206, 212), rgb(222, 222, 226)
		p.menuText, p.menuKey, p.menuOff = rgb(28, 28, 30), rgb(120, 120, 128), rgb(176, 176, 182)
		p.btn, p.btnHover = rgb(226, 226, 230), rgb(212, 212, 218)
		p.winBg, p.sideBg, p.card = rgb(240, 240, 243), rgb(229, 229, 234), rgb(255, 255, 255)
		p.cardLine, p.ctrlOff = rgb(226, 226, 230), rgb(200, 200, 206)
	}
	r, g, b := accentRGB(accent)
	p.accent = rgb(r, g, b)
	p.accentHi = rgb(mixWhite(r, g, b, 0.18))
	return p
}

// rebuildTheme recrée la palette et tous les pinceaux et stylos partagés.
func rebuildTheme() {
	pal = makePalette(effectiveDark(), cfg.Accent)
	for _, h := range []*uintptr{&brushBar, &brushHover, &brushAccent, &brushSep, &brushGlyph,
		&brushMenuBg, &brushBtn, &brushBtnHover, &brushAccentHi, &brushWin, &brushSide,
		&penBorder, &penChev, &penChevSel} {
		if *h != 0 {
			pDeleteObject.Call(*h)
			*h = 0
		}
	}
	brush := func(c uintptr) uintptr {
		h, _, _ := pCreateSolidBrush.Call(c)
		return h
	}
	brushBar, brushHover = brush(pal.bar), brush(pal.barPress)
	brushAccent, brushAccentHi = brush(pal.accent), brush(pal.accentHi)
	brushSep, brushGlyph = brush(pal.menuSep), brush(pal.barText)
	brushMenuBg, brushBtn, brushBtnHover = brush(pal.menuBg), brush(pal.btn), brush(pal.btnHover)
	brushWin, brushSide = brush(pal.winBg), brush(pal.sideBg)

	pen := func(w int32, c uintptr) uintptr {
		h, _, _ := pCreatePen.Call(0 /*PS_SOLID*/, uintptr(w), c)
		return h
	}
	cw := int32(1.6*scale + 0.5)
	if cw < 1 {
		cw = 1
	}
	penBorder = pen(1, pal.menuBorder)
	penChev = pen(cw, pal.menuKey)
	chevSel := rgb(255, 255, 255)
	penChevSel = pen(cw, chevSel)
}
