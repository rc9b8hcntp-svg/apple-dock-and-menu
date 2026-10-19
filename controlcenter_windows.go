//go:build windows

package main

// Le Centre de contrôle : un panneau de tuiles et de curseurs sous l'icône de la
// barre, comme sur macOS. Wi-Fi, mode sombre, Dock et Launchpad s'activent d'un
// clic ; les curseurs règlent l'opacité de la barre et le volume ; une rangée de
// pastilles change la couleur d'accentuation. Tout ce qui touche aux réglages du
// programme est enregistré dans settings.ini et mis à jour dans la fenêtre de réglages.

import (
	"syscall"
	"unsafe"

	"menuwin/internal/config"
)

const ccClass = "MenuWindowsControlCenter"

const (
	ccTile = iota
	ccSlider
	ccSwatches
	ccButton
	ccMedia // lecture précédente / lecture-pause / suivante
)

type ccCtl struct {
	kind       int
	r          rect
	glyph      string
	title, sub string
	on         bool
	plain      bool // tuile sans état (Launchpad)
	closes     bool // referme le panneau avant d'agir
	click      func()
	get        func() int
	set        func(int)
	lo, hi     int
}

var cc struct {
	hwnd, prev uintptr
	x, y, w, h int32
	ctls       []ccCtl
	audio      *audioEP
	hasWifi    bool
	wifiOn     bool
	down       int
	drag       int
	hover      int
	mediaHover int
	compact    bool
}

var ccClassOK bool

// ccBuild reconstruit les contrôles (état et position) ; renvoie la hauteur du panneau.
func ccBuild() int32 {
	pad, gap := sc(12), sc(8)
	w := cc.w
	colW := (w - 2*pad - gap) / 2
	var ctls []ccCtl

	onOff := func(b bool) string {
		if b {
			return tr("Activé", "On")
		}
		return tr("Désactivé", "Off")
	}

	var tiles []ccCtl
	if cc.hasWifi {
		sub := onOff(cc.wifiOn)
		if cc.wifiOn && wifiState == wifiOff {
			sub = tr("Non connecté", "Not Connected")
		}
		tiles = append(tiles, ccCtl{kind: ccTile, glyph: "", title: "Wi-Fi", sub: sub, on: cc.wifiOn, click: func() {
			if toggleWifi(!cc.wifiOn) {
				cc.wifiOn = !cc.wifiOn
			}
		}})
	}
	if present, on, _ := btState(); present {
		tiles = append(tiles, ccCtl{kind: ccTile, glyph: "\uE702", title: "Bluetooth", sub: onOff(on), on: on, click: func() {
			btSetAsync(!on)
			on = !on
		}})
	}
	dark := pal.dark
	themeSub := tr("Clair", "Light")
	if dark {
		themeSub = tr("Sombre", "Dark")
	}
	tiles = append(tiles,
		ccCtl{kind: ccTile, glyph: "", title: tr("Mode sombre", "Dark Mode"), sub: themeSub, on: dark, click: func() {
			if effectiveDark() {
				cfg.Theme = "light"
			} else {
				cfg.Theme = "dark"
			}
			settingChanged(true)
		}},
		ccCtl{kind: ccTile, glyph: "\uE8A9", title: "Dock", sub: onOff(cfg.DockEnabled), on: cfg.DockEnabled, click: func() {
			cfg.DockEnabled = !cfg.DockEnabled
			settingChanged(true)
		}},
		ccCtl{kind: ccTile, glyph: "", title: "Launchpad", sub: "Ctrl+Alt+L", plain: true, closes: true, click: showLaunchpad},
		ccCtl{kind: ccTile, glyph: "\uE7C4", title: "Mission Control", sub: "Win+Tab", plain: true, closes: true, click: func() { sendWinKey(0x09 /*VK_TAB*/) }},
		ccCtl{kind: ccTile, glyph: "\uE708", title: tr("Ne pas déranger", "Do Not Disturb"), sub: onOff(dndOn()), on: dndOn(), click: func() { setDND(!dndOn()) }},
		ccCtl{kind: ccTile, glyph: "\uE722", title: tr("Capture d’écran", "Screenshot"), sub: "Win+Maj+S", plain: true, closes: true, click: func() { sendWinShiftKey('S') }},
	)
	y := pad
	tileH := sc(58)
	for i := range tiles {
		col, row := int32(i%2), int32(i/2)
		x := pad + col*(colW+gap)
		tiles[i].r = rect{x, y + row*(tileH+gap), x + colW, y + row*(tileH+gap) + tileH}
	}
	rows := int32((len(tiles) + 1) / 2)
	y += rows*(tileH+gap) + 0
	ctls = append(ctls, tiles...)

	sliderH := sc(68)
	addSlider := func(glyph, title string, lo, hi int, get func() int, set func(int)) {
		ctls = append(ctls, ccCtl{kind: ccSlider, glyph: glyph, title: title, lo: lo, hi: hi, get: get, set: set,
			r: rect{pad, y, w - pad, y + sliderH}})
		y += sliderH + gap
	}
	addSlider("", tr("Opacité de la barre", "Menu Bar Opacity"), config.BarOpacityMin, config.BarOpacityMax,
		func() int { return cfg.BarOpacity },
		func(v int) {
			cfg.BarOpacity = v
			applyConfig()
			cfgDirty = true
		})
	if brightOK.Load() {
		addSlider("\uE706", tr("Luminosité", "Display"), 1, 100, brightnessGet, brightnessSet)
	}
	if cc.audio != nil {
		a := cc.audio
		glyph := ""
		if a.muted() {
			glyph = ""
		}
		addSlider(glyph, tr("Son", "Sound"), 0, 100, a.percent, a.setPercent)
	}

	if !cc.compact { // écran bas : on retire la rangée multimédia pour que le panneau tienne
		ctls = append(ctls, ccCtl{kind: ccMedia, title: tr("Multimédia", "Now Playing"), r: rect{pad, y, w - pad, y + sc(68)}})
		y += sc(68) + gap
	}

	ctls = append(ctls, ccCtl{kind: ccSwatches, title: tr("Couleur d’accentuation", "Accent Color"),
		r: rect{pad, y, w - pad, y + sc(64)}})
	y += sc(64) + gap

	for _, b := range []struct {
		label string
		fn    func()
	}{
		{t.prefs, showSettings},
		{t.settings, func() { openURI("ms-settings:") }},
	} {
		ctls = append(ctls, ccCtl{kind: ccButton, title: b.label, closes: true, click: b.fn, r: rect{pad, y, w - pad, y + sc(36)}})
		y += sc(36) + gap
	}
	cc.ctls = ctls
	return y - gap + pad
}

func ccRefresh() {
	if cc.hwnd == 0 {
		return
	}
	h := ccBuild()
	if h != cc.h {
		cc.h = h
		pSetWindowPos.Call(cc.hwnd, 0, uintptr(cc.x), uintptr(cc.y), uintptr(cc.w), uintptr(cc.h), swpNoZOrder|swpNoActivate)
		styleWindow(cc.hwnd, cc.w, cc.h)
	}
	pInvalidateRect.Call(cc.hwnd, 0, 0)
}

// ---------------------------------------------------------------------------
// Dessin
// ---------------------------------------------------------------------------

// ccMediaRect : le k-ième bouton (précédent, lecture/pause, suivant) de la rangée multimédia.
func ccMediaRect(c ccCtl, k int) rect {
	w := (c.r.Right - c.r.Left - sc(24)) / 3
	x := c.r.Left + sc(12) + int32(k)*w
	return rect{x, c.r.Top + sc(28), x + w, c.r.Bottom - sc(6)}
}

func ccSwatchRect(c ccCtl, i int) rect {
	d := sc(22)
	n := int32(len(config.Accents))
	span := c.r.Right - c.r.Left - 2*sc(14)
	step := (span - d) / (n - 1)
	x := c.r.Left + sc(14) + int32(i)*step
	y := c.r.Bottom - sc(14) - d
	return rect{x, y, x + d, y + d}
}

func ccTrack(c ccCtl) (x0, x1, top, bot int32) {
	h := sc(26)
	return c.r.Left + sc(12), c.r.Right - sc(12), c.r.Bottom - sc(12) - h, c.r.Bottom - sc(12)
}

func drawControlCenter(hdc uintptr, w, h int32) {
	full := rect{0, 0, w, h}
	pFillRect.Call(hdc, uintptr(unsafe.Pointer(&full)), brushMenuBg)
	pSetBkMode.Call(hdc, transparent)
	card := cardColor()
	off := blendColor(pal.menuBg, pal.menuText, 0.22)

	for i, c := range cc.ctls {
		bg := card
		if i == cc.hover && (c.kind == ccTile || c.kind == ccButton) {
			bg = blendColor(pal.menuBg, pal.menuText, 0.14)
		}
		fillRR(hdc, c.r, sc(14), bg)
		switch c.kind {
		case ccTile:
			d := sc(34)
			disc := rect{c.r.Left + sc(12), c.r.Top + (c.r.Bottom-c.r.Top-d)/2, c.r.Left + sc(12) + d, c.r.Top + (c.r.Bottom-c.r.Top-d)/2 + d}
			fillCol, glyphCol := off, pal.menuText
			if c.on && !c.plain {
				fillCol, glyphCol = pal.accent, rgb(255, 255, 255)
			}
			glyphDisc(hdc, disc, c.glyph, fillCol, glyphCol)
			mid := (c.r.Top + c.r.Bottom) / 2
			pSelectObject.Call(hdc, fontBold)
			pSetTextColor.Call(hdc, pal.menuText)
			drawText(hdc, c.title, rect{disc.Right + sc(10), mid - sc(19), c.r.Right - sc(8), mid + sc(1)}, dtLeft|dtEllipsis)
			pSelectObject.Call(hdc, panelFont.small)
			pSetTextColor.Call(hdc, pal.menuKey)
			drawText(hdc, c.sub, rect{disc.Right + sc(10), mid - sc(1), c.r.Right - sc(8), mid + sc(17)}, dtLeft|dtEllipsis)
		case ccButton:
			pSelectObject.Call(hdc, fontReg)
			pSetTextColor.Call(hdc, pal.menuText)
			drawText(hdc, c.title, rect{c.r.Left + sc(14), c.r.Top, c.r.Right - sc(10), c.r.Bottom}, dtLeft|dtEllipsis)
		case ccSlider:
			pSelectObject.Call(hdc, fontBold)
			pSetTextColor.Call(hdc, pal.menuText)
			drawText(hdc, c.title, rect{c.r.Left + sc(14), c.r.Top + sc(6), c.r.Right - sc(14), c.r.Top + sc(28)}, dtLeft|dtEllipsis)
			x0, x1, top, bot := ccTrack(c)
			th := bot - top
			fillRR(hdc, rect{x0, top, x1, bot}, th, off)
			v := c.get()
			if v < c.lo {
				v = c.lo
			}
			if v > c.hi {
				v = c.hi
			}
			span := c.hi - c.lo
			if span <= 0 {
				span = 1
			}
			kx := x0 + th + int32(float64(v-c.lo)/float64(span)*float64(x1-x0-th))
			fillCol, glyphCol := rgb(255, 255, 255), rgb(70, 70, 76)
			if !pal.dark {
				fillCol, glyphCol = pal.accent, rgb(255, 255, 255)
			}
			fillRR(hdc, rect{x0, top, kx, bot}, th, fillCol)
			drawGlyphIn(hdc, panelFont.glyph, c.glyph, rect{x0, top, x0 + th, bot}, glyphCol)
		case ccMedia:
			pSelectObject.Call(hdc, fontBold)
			pSetTextColor.Call(hdc, pal.menuText)
			drawText(hdc, c.title, rect{c.r.Left + sc(14), c.r.Top + sc(6), c.r.Right - sc(14), c.r.Top + sc(28)}, dtLeft|dtEllipsis)
			for k, g := range []string{"\uE892", "\uE768", "\uE893"} {
				br := ccMediaRect(c, k)
				if i == cc.hover && cc.mediaHover == k {
					fillRR(hdc, br, sc(12), blendColor(pal.menuBg, pal.menuText, 0.14))
				}
				drawGlyphIn(hdc, panelFont.glyphBig, g, br, pal.menuText)
			}
		case ccSwatches:
			pSelectObject.Call(hdc, fontBold)
			pSetTextColor.Call(hdc, pal.menuText)
			drawText(hdc, c.title, rect{c.r.Left + sc(14), c.r.Top + sc(6), c.r.Right - sc(14), c.r.Top + sc(28)}, dtLeft|dtEllipsis)
			for k, id := range config.Accents {
				sr := ccSwatchRect(c, k)
				r8, g8, b8 := accentRGB(id)
				if cfg.Accent == id {
					fillEllipse(hdc, rect{sr.Left - sc(3), sr.Top - sc(3), sr.Right + sc(3), sr.Bottom + sc(3)}, pal.menuText)
					fillEllipse(hdc, rect{sr.Left - sc(1), sr.Top - sc(1), sr.Right + sc(1), sr.Bottom + sc(1)}, card)
				}
				fillEllipse(hdc, sr, rgb(r8, g8, b8))
			}
		}
	}
	pSelectObject.Call(hdc, fontReg)
}

// ---------------------------------------------------------------------------
// Souris
// ---------------------------------------------------------------------------

func ccAt(x, y int32) int {
	for i, c := range cc.ctls {
		if inRect(c.r, x, y) {
			return i
		}
	}
	return -1
}

func ccSlide(i int, x int32) {
	c := cc.ctls[i]
	x0, x1, top, bot := ccTrack(c)
	th := bot - top
	if x1-x0-th <= 0 {
		return
	}
	f := float64(x-x0-th/2) / float64(x1-x0-th)
	if f < 0 {
		f = 0
	}
	if f > 1 {
		f = 1
	}
	v := c.lo + int(f*float64(c.hi-c.lo)+0.5)
	if c.get() != v {
		c.set(v)
	}
	pInvalidateRect.Call(cc.hwnd, 0, 0)
}

func ccProc(hwnd, message, wparam, lparam uintptr) uintptr {
	if hwnd != cc.hwnd {
		r, _, _ := pDefWindowProc.Call(hwnd, message, wparam, lparam)
		return r
	}
	switch uint32(message) {
	case wmEraseBkgnd:
		return 1
	case wmPaint:
		paintBuffered(hwnd, func(mem uintptr, w, h int32) { drawControlCenter(mem, w, h) })
		return 0
	case wmActivate:
		if wparam&0xFFFF == waInactive {
			closeControlCenter(false)
		}
		return 0
	case wmMouseMove:
		x, y := lparamXY(lparam)
		if cc.drag >= 0 {
			ccSlide(cc.drag, x)
			return 0
		}
		mh := -1
		if i := ccAt(x, y); i >= 0 && cc.ctls[i].kind == ccMedia {
			for k := 0; k < 3; k++ {
				if inRect(ccMediaRect(cc.ctls[i], k), x, y) {
					mh = k
				}
			}
		}
		if i := ccAt(x, y); i != cc.hover || mh != cc.mediaHover {
			cc.hover, cc.mediaHover = i, mh
			pInvalidateRect.Call(hwnd, 0, 0)
		}
		return 0
	case wmLButtonDown:
		x, y := lparamXY(lparam)
		i := ccAt(x, y)
		cc.down = i
		if i >= 0 && cc.ctls[i].kind == ccSlider {
			cc.drag = i
			pSetCapture.Call(hwnd)
			ccSlide(i, x)
		}
		return 0
	case wmLButtonUp:
		x, y := lparamXY(lparam)
		if cc.drag >= 0 {
			cc.drag = -1
			pReleaseCapture.Call()
			if cfgDirty {
				saveConfig()
				cfgDirty = false
			}
			ccRefresh()
			return 0
		}
		i := ccAt(x, y)
		if i < 0 || i != cc.down {
			return 0
		}
		c := cc.ctls[i]
		switch {
		case c.kind == ccMedia:
			for k, vk := range []byte{0xB1, 0xB3, 0xB0} { // précédent, lecture/pause, suivant
				if inRect(ccMediaRect(c, k), x, y) {
					sendMediaKey(vk)
				}
			}
		case c.kind == ccSwatches:
			for k, id := range config.Accents {
				if inRect(ccSwatchRect(c, k), x, y) {
					cfg.Accent = id
					settingChanged(true)
				}
			}
			ccRefresh()
		case c.click != nil && c.closes:
			closeControlCenter(true)
			c.click()
		case c.click != nil:
			c.click()
			ccRefresh()
		}
		return 0
	case wmKeyDown:
		if wparam == vkEscape {
			closeControlCenter(true)
		}
		return 0
	case wmClose:
		closeControlCenter(true)
		return 0
	}
	r, _, _ := pDefWindowProc.Call(hwnd, message, wparam, lparam)
	return r
}

func registerControlClass() {
	if ccClassOK {
		return
	}
	hInst, _, _ := pGetModuleHandle.Call(0)
	cursor, _, _ := pLoadCursor.Call(0, idcArrow)
	wc := wndClassEx{WndProc: syscall.NewCallback(ccProc), Instance: hInst, Cursor: cursor, ClassName: utf16(ccClass)}
	wc.Size = uint32(unsafe.Sizeof(wc))
	pRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
	ccClassOK = true
}

// showControlCenter ouvre le panneau sous l'icône de la barre (ou le referme).
func showControlCenter() {
	if cc.hwnd != 0 {
		closeControlCenter(true)
		return
	}
	if menuOpen || dialogOpen {
		return
	}
	registerControlClass()
	initPanelFonts()
	hideLabel()

	r, ok := hitRect(hitControl)
	if !ok {
		return
	}
	sx, sy := screenPos(barHwnd, r)
	var wr rect
	pGetWindowRect.Call(barHwnd, uintptr(unsafe.Pointer(&wr)))
	iconRight := wr.Left + r.Right
	wa := workArea(sx, sy)

	cc.w = sc(330)
	cc.hasWifi = wifiState != wifiNone
	cc.wifiOn = cc.hasWifi && wifiRadioState()
	cc.audio = openAudio()
	cc.down, cc.drag, cc.hover = -1, -1, -1
	cc.compact = false
	cc.h = ccBuild()
	if cc.h > wa.Bottom-sy-sc(20) {
		cc.compact = true
		cc.h = ccBuild()
	}
	cc.x = iconRight
	if cc.x > wa.Right-sc(8) {
		cc.x = wa.Right - sc(8)
	}
	cc.x -= cc.w
	if cc.x < wa.Left {
		cc.x = wa.Left
	}
	cc.y = sy + sc(6)

	cc.prev, _, _ = pGetForeground.Call()
	hInst, _, _ := pGetModuleHandle.Call(0)
	hwnd, _, _ := pCreateWindowEx.Call(wsExTopmost|wsExToolwin, ptr(utf16(ccClass)), ptr(utf16("Control Center")), wsPopup,
		uintptr(cc.x), uintptr(cc.y), uintptr(cc.w), uintptr(cc.h), 0, 0, hInst, 0)
	if hwnd == 0 {
		cc.audio.close()
		cc.audio = nil
		return
	}
	cc.hwnd = hwnd
	styleWindow(hwnd, cc.w, cc.h)
	pShowWindow.Call(hwnd, swShow)
	pSetForeground.Call(hwnd)
	pSetFocus.Call(hwnd)
	invalidate(barHwnd)
}

func closeControlCenter(restore bool) {
	h := cc.hwnd
	if h == 0 {
		return
	}
	cc.hwnd = 0
	pReleaseCapture.Call()
	pDestroyWindow.Call(h)
	cc.audio.close()
	cc.audio = nil
	cc.ctls = nil
	if cfgDirty {
		saveConfig()
		cfgDirty = false
	}
	if barHwnd != 0 {
		invalidate(barHwnd)
	}
	if restore && cc.prev != 0 && cc.prev != barHwnd {
		if ok, _, _ := pIsWindow.Call(cc.prev); ok != 0 {
			pSetForeground.Call(cc.prev)
		}
	}
}
