//go:build windows

package main

// Fenêtre de réglages façon « Réglages Système » de macOS : fenêtre sans bordure
// avec les trois boutons rouge / jaune / gris, barre latérale à icônes colorées avec
// le profil en haut, titre dans la barre d'outils, cartes arrondies à lignes
// séparées, interrupteurs, curseurs et défilement doux.

import (
	"menuwin/internal/dockgfx"
	"os"
	"strings"
	"syscall"
	"unicode/utf8"
	"unsafe"
)

var (
	pIntersectClipRect = gdi32.NewProc("IntersectClipRect")
	pSelectClipRgn     = gdi32.NewProc("SelectClipRgn")
)

const (
	wmNCHitTest = 0x0084
	htClient    = 1
	htCaption   = 2
)

func sideW() int32    { return sc(220) }
func toolbarH() int32 { return sc(54) }

// Icône de la barre latérale : un glyphe Segoe MDL2 (ou un dessin) sur une pastille de couleur.
type pageIcon struct {
	glyph   string
	r, g, b uint8
	custom  int // 1 = Dock, 2 = coins actifs
}

var pageIcons = []pageIcon{
	{glyph: "", r: 142, g: 142, b: 147}, // Général
	{glyph: "", r: 88, g: 86, b: 214},   // Apparence
	{glyph: "", r: 99, g: 99, b: 102},   // Barre
	{glyph: "", r: 255, g: 149, b: 0},   // Icônes d'état
	{custom: 1, r: 10, g: 132, b: 255},   // Dock
	{custom: 2, r: 52, g: 199, b: 89},    // Coins actifs
}

func lightRect(i int) rect {
	d := sc(12)
	x := sc(18) + int32(i)*sc(20)
	y := sc(17)
	return rect{x, y, x + d, y + d}
}

func sideListTop() int32 { return sc(112) }

func sLayout() {
	st.slots, st.groups, st.side = st.slots[:0], st.groups[:0], st.side[:0]
	dc, _, _ := pGetDC.Call(0)
	old, _, _ := pSelectObject.Call(dc, fontReg)
	defer func() {
		pSelectObject.Call(dc, old)
		pReleaseDC.Call(0, dc)
	}()

	side, pad := sideW(), sc(24)
	st.lights = rect{sc(12), sc(11), sc(12) + sc(66), sc(35)}
	for i := range st.pages {
		y := sideListTop() + int32(i)*sc(36)
		st.side = append(st.side, rect{sc(10), y, side - sc(10), y + sc(32)})
	}

	x0, x1 := side+pad, st.w-pad
	y := toolbarH() + sc(6) - st.scroll
	rowH, hdrH := sc(40), sc(32)
	groupTop := int32(-1)
	closeGroup := func(bottom int32) {
		if groupTop >= 0 {
			st.groups = append(st.groups, rect{x0, groupTop, x1, bottom})
			groupTop = -1
		}
	}

	for i, row := range st.pages[st.page].rows {
		if row.kind == rowHeader {
			closeGroup(y)
			y += sc(10)
			st.slots = append(st.slots, sSlot{row: i, r: rect{x0, y, x1, y + hdrH}})
			y += hdrH
			continue
		}
		if groupTop < 0 {
			groupTop = y
		}
		sl := sSlot{row: i, r: rect{x0, y, x1, y + rowH}}
		cy := y + rowH/2
		right := x1 - sc(14)
		switch row.kind {
		case rowToggle:
			sl.ctl = rect{right - sc(38), cy - sc(11), right, cy + sc(11)}
		case rowSlider:
			sl.val = rect{right - sc(56), y, right, y + rowH}
			trackW := sc(170)
			sl.ctl = rect{sl.val.Left - sc(10) - trackW, cy - sc(11), sl.val.Left - sc(10), cy + sc(11)}
		case rowChoice:
			var widths []int32
			var total int32
			for _, n := range row.names {
				w := textWidth(dc, n) + sc(24)
				widths = append(widths, w)
				total += w
			}
			x := right - total
			for _, w := range widths {
				sl.segs = append(sl.segs, rect{x, cy - sc(13), x + w, cy + sc(13)})
				x += w
			}
			sl.ctl = rect{right - total, cy - sc(13), right, cy + sc(13)}
		case rowSwatch:
			d, gap := sc(20), sc(10)
			total := int32(len(row.opts))*d + int32(len(row.opts)-1)*gap
			x := right - total
			for range row.opts {
				sl.segs = append(sl.segs, rect{x, cy - d/2, x + d, cy + d/2})
				x += d + gap
			}
			sl.ctl = rect{right - total, cy - d/2, right, cy + d/2}
		case rowButton:
			w := textWidth(dc, row.label) + sc(32)
			sl.ctl = rect{x0 + sc(14), cy - sc(13), x0 + sc(14) + w, cy + sc(13)}
		}
		st.slots = append(st.slots, sl)
		y += rowH
	}
	closeGroup(y)
	st.contentEnd = y + st.scroll
	if m := settingsMaxScroll(); st.scroll > m {
		st.scroll = m
	}
}

// settingsMaxScroll : de combien on peut faire défiler la page courante.
func settingsMaxScroll() int32 {
	m := st.contentEnd + sc(24) - st.h
	if m < 0 {
		return 0
	}
	return m
}

// sHitTest renvoie la ligne et le segment sous (x, y) : (-1, -1) si rien.
func sHitTest(x, y int32) (int, int) {
	if y < toolbarH() || x < sideW() {
		return -1, -1
	}
	for _, sl := range st.slots {
		row := st.pages[st.page].rows[sl.row]
		switch row.kind {
		case rowToggle:
			if inRect(sl.r, x, y) {
				return sl.row, 0
			}
		case rowSlider:
			ex := rect{sl.ctl.Left - sc(8), sl.r.Top, sl.ctl.Right + sc(8), sl.r.Bottom}
			if inRect(ex, x, y) {
				return sl.row, 0
			}
		case rowChoice, rowSwatch:
			for i, s := range sl.segs {
				if inRect(s, x, y) {
					return sl.row, i
				}
			}
		case rowButton:
			if inRect(sl.ctl, x, y) {
				return sl.row, 0
			}
		}
	}
	return -1, -1
}

func drawPageIcon(hdc uintptr, t rect, i int) {
	ic := pageIcons[0]
	if i < len(pageIcons) {
		ic = pageIcons[i]
	}
	col := rgb(ic.r, ic.g, ic.b)
	fillRR(hdc, t, sc(11), col)
	white := rgb(255, 255, 255)
	switch ic.custom {
	case 1: // Dock : une fenêtre et la barre du Dock
		fillRR(hdc, rect{t.Left + sc(5), t.Top + sc(5), t.Right - sc(5), t.Bottom - sc(10)}, sc(3), blendColor(col, white, 0.55))
		fillRR(hdc, rect{t.Left + sc(4), t.Bottom - sc(8), t.Right - sc(4), t.Bottom - sc(4)}, sc(4), white)
	case 2: // coins actifs : un écran et un point dans un coin
		fillRR(hdc, rect{t.Left + sc(4), t.Top + sc(5), t.Right - sc(4), t.Bottom - sc(5)}, sc(4), blendColor(col, white, 0.55))
		fillEllipse(hdc, rect{t.Right - sc(10), t.Top + sc(5), t.Right - sc(4), t.Top + sc(11)}, white)
	default:
		pSelectObject.Call(hdc, st.fontIcon)
		pSetTextColor.Call(hdc, white)
		drawText(hdc, ic.glyph, t, dtCenter)
	}
}

func userInitial() (string, string) {
	name := os.Getenv("USERNAME")
	if name == "" {
		name = "Windows"
	}
	r, _ := utf8.DecodeRuneInString(name)
	return strings.ToUpper(string(r)), name
}

func drawLights(hdc uintptr) {
	grey := blendColor(pal.sideBg, pal.menuKey, 0.45)
	cols := [3]uintptr{rgb(255, 95, 87), rgb(254, 188, 46), grey}
	if !st.active {
		cols[0], cols[1] = grey, grey
	}
	glyphs := [3]string{"×", "–", ""}
	for i := 0; i < 3; i++ {
		r := lightRect(i)
		fillEllipse(hdc, rect{r.Left - 1, r.Top - 1, r.Right + 1, r.Bottom + 1}, blendColor(cols[i], rgb(0, 0, 0), 0.14))
		fillEllipse(hdc, r, cols[i])
		if st.hoverLights && st.active && glyphs[i] != "" {
			pSelectObject.Call(hdc, fontBold)
			pSetTextColor.Call(hdc, blendColor(cols[i], rgb(0, 0, 0), 0.7))
			drawText(hdc, glyphs[i], r, dtCenter)
		}
	}
}

func drawSettings(hdc uintptr, w, h int32) {
	initPanelFonts()
	sLayout()
	side, pad := sideW(), sc(24)
	tb := toolbarH()
	full := rect{0, 0, w, h}
	pFillRect.Call(hdc, uintptr(unsafe.Pointer(&full)), brushWin)
	pFillRect.Call(hdc, uintptr(unsafe.Pointer(&rect{0, 0, side, h})), brushSide)
	fillColor(hdc, rect{side - 1, 0, side, h}, pal.cardLine)
	pSetBkMode.Call(hdc, transparent)

	// Contenu (rogné sous la barre d'outils)
	pIntersectClipRect.Call(hdc, uintptr(side), uintptr(tb), uintptr(w), uintptr(h))
	for _, g := range st.groups {
		fillRR(hdc, g, sc(20), pal.cardLine)
		fillRR(hdc, shrink(g, 1), sc(19), pal.card)
	}
	rows := st.pages[st.page].rows
	for _, sl := range st.slots {
		if sl.r.Bottom < tb || sl.r.Top > h {
			continue
		}
		row := rows[sl.row]
		if row.kind == rowHeader {
			pSelectObject.Call(hdc, fontBold)
			pSetTextColor.Call(hdc, pal.menuText)
			drawText(hdc, row.label, rect{sl.r.Left + sc(4), sl.r.Top, sl.r.Right, sl.r.Bottom}, dtLeft|dtEllipsis)
			continue
		}
		// Trait entre deux lignes d'une même carte
		if sl.row+1 < len(rows) && rows[sl.row+1].kind != rowHeader {
			fillColor(hdc, rect{sl.r.Left + sc(14), sl.r.Bottom, sl.r.Right - sc(14), sl.r.Bottom + 1}, pal.cardLine)
		}
		if row.kind != rowButton {
			pSelectObject.Call(hdc, fontReg)
			pSetTextColor.Call(hdc, pal.menuText)
			drawText(hdc, row.label, rect{sl.r.Left + sc(14), sl.r.Top, sl.ctl.Left - sc(12), sl.r.Bottom}, dtLeft|dtEllipsis)
		}
		switch row.kind {
		case rowToggle:
			drawToggle(hdc, sl.ctl, row.getB())
		case rowSlider:
			drawSlider(hdc, sl, row)
		case rowChoice:
			drawChoice(hdc, sl, row)
		case rowSwatch:
			drawSwatches(hdc, sl, row)
		case rowButton:
			drawPushButton(hdc, sl.ctl, row.label, st.hoverRow == sl.row)
		}
	}
	// Barre de défilement fine
	if m := settingsMaxScroll(); m > 0 {
		view := h - tb
		total := view + m
		th := view - sc(8)
		bh := th * view / total
		if bh < sc(24) {
			bh = sc(24)
		}
		by := tb + sc(4) + (th-bh)*st.scroll/m
		fillRR(hdc, rect{w - sc(8), by, w - sc(4), by + bh}, sc(4), blendColor(pal.winBg, pal.menuText, 0.30))
	}
	pSelectClipRgn.Call(hdc, 0)

	// Barre d'outils : le titre de la page
	if st.scroll > 0 {
		fillColor(hdc, rect{side, tb - 1, w, tb}, pal.cardLine)
	}
	pSelectObject.Call(hdc, st.fontBig)
	pSetTextColor.Call(hdc, pal.menuText)
	drawText(hdc, st.pages[st.page].name, rect{side + pad, sc(8), w - pad, tb - sc(2)}, dtLeft|dtEllipsis)

	// Barre latérale : boutons de fenêtre, profil, pages
	drawLights(hdc)
	initial, name := userInitial()
	av := rect{sc(16), sc(50), sc(16) + sc(44), sc(50) + sc(44)}
	fillEllipse(hdc, av, blendColor(pal.sideBg, pal.menuKey, 0.55))
	pSelectObject.Call(hdc, st.fontBig)
	pSetTextColor.Call(hdc, rgb(255, 255, 255))
	drawText(hdc, initial, av, dtCenter)
	pSelectObject.Call(hdc, fontBold)
	pSetTextColor.Call(hdc, pal.menuText)
	drawText(hdc, name, rect{av.Right + sc(10), av.Top + sc(2), side - sc(10), av.Top + sc(22)}, dtLeft|dtEllipsis)
	pSelectObject.Call(hdc, panelFont.small)
	pSetTextColor.Call(hdc, pal.menuKey)
	drawText(hdc, tr("Compte Windows", "Windows Account"), rect{av.Right + sc(10), av.Top + sc(22), side - sc(10), av.Top + sc(40)}, dtLeft|dtEllipsis)

	for i, r := range st.side {
		txt := pal.menuText
		if i == st.page {
			fillRR(hdc, r, sc(12), pal.accent)
			txt = rgb(255, 255, 255)
		}
		tile := rect{r.Left + sc(8), r.Top + (r.Bottom-r.Top-sc(22))/2, r.Left + sc(8) + sc(22), r.Top + (r.Bottom-r.Top-sc(22))/2 + sc(22)}
		drawPageIcon(hdc, tile, i)
		pSelectObject.Call(hdc, fontReg)
		pSetTextColor.Call(hdc, txt)
		drawText(hdc, st.pages[i].name, rect{tile.Right + sc(10), r.Top, r.Right - sc(8), r.Bottom}, dtLeft|dtEllipsis)
	}
}

// drawPushButton : un bouton à bord fin, comme un bouton poussoir macOS.
func drawPushButton(hdc uintptr, r rect, label string, hover bool) {
	bg := pal.btn
	if hover {
		bg = pal.btnHover
	}
	fillRR(hdc, r, sc(12), blendColor(bg, pal.menuText, 0.10))
	fillRR(hdc, shrink(r, 1), sc(11), bg)
	pSelectObject.Call(hdc, fontReg)
	pSetTextColor.Call(hdc, pal.menuText)
	drawText(hdc, label, r, dtCenter)
}

func drawToggle(hdc uintptr, r rect, on bool) {
	col := pal.ctrlOff
	if on {
		col = pal.accent
	}
	h := r.Bottom - r.Top
	fillRR(hdc, r, h, col)
	d := h - sc(4)
	kx := r.Left + sc(2)
	if on {
		kx = r.Right - sc(2) - d
	}
	fillEllipse(hdc, rect{kx - 1, r.Top + sc(2), kx + d + 1, r.Top + sc(2) + d + 2}, blendColor(col, rgb(0, 0, 0), 0.28))
	fillEllipse(hdc, rect{kx, r.Top + sc(2), kx + d, r.Top + sc(2) + d}, rgb(255, 255, 255))
}

func drawSlider(hdc uintptr, sl sSlot, row sRow) {
	tr := sl.ctl
	cy := (tr.Top + tr.Bottom) / 2
	th := sc(4)
	track := rect{tr.Left, cy - th/2, tr.Right, cy + th/2 + th%2}
	fillRR(hdc, track, th, pal.ctrlOff)
	frac := float64(*row.n-row.min) / float64(row.max-row.min)
	kd := sc(20)
	kx := tr.Left + int32(frac*float64(tr.Right-tr.Left-kd))
	filled := rect{tr.Left, track.Top, kx + kd/2, track.Bottom}
	fillRR(hdc, filled, th, pal.accent)
	fillEllipse(hdc, rect{kx - 1, cy - kd/2, kx + kd + 1, cy + kd/2 + 2}, blendColor(pal.card, rgb(0, 0, 0), 0.22))
	fillEllipse(hdc, rect{kx, cy - kd/2, kx + kd, cy + kd/2}, rgb(255, 255, 255))

	pSelectObject.Call(hdc, fontReg)
	pSetTextColor.Call(hdc, pal.menuKey)
	drawText(hdc, itoa(*row.n)+row.unit, sl.val, dtRight)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// drawChoice : un contrôle segmenté, le segment choisi en couleur d'accentuation.
func drawChoice(hdc uintptr, sl sSlot, row sRow) {
	fillRR(hdc, sl.ctl, sc(14), blendColor(pal.card, pal.menuText, 0.12))
	fillRR(hdc, shrink(sl.ctl, 1), sc(13), pal.winBg)
	pSelectObject.Call(hdc, fontReg)
	for i, seg := range sl.segs {
		sel := row.opts[i] == *row.s
		if sel {
			fillRR(hdc, shrink(seg, sc(2)), sc(11), pal.accent)
			pSetTextColor.Call(hdc, rgb(255, 255, 255))
		} else {
			pSetTextColor.Call(hdc, pal.menuText)
		}
		drawText(hdc, row.names[i], seg, dtCenter)
	}
}

func drawSwatches(hdc uintptr, sl sSlot, row sRow) {
	for i, seg := range sl.segs {
		if row.opts[i] == *row.s {
			fillEllipse(hdc, rect{seg.Left - sc(4), seg.Top - sc(4), seg.Right + sc(4), seg.Bottom + sc(4)}, pal.menuKey)
			fillEllipse(hdc, rect{seg.Left - sc(2), seg.Top - sc(2), seg.Right + sc(2), seg.Bottom + sc(2)}, pal.card)
		}
		r, g, b := accentRGB(row.opts[i])
		fillEllipse(hdc, seg, rgb(r, g, b))
	}
}

func styleSettingsWindow() {
	if st.hwnd == 0 {
		return
	}
	styleWindow(st.hwnd, st.w, st.h)
	if pDwmSetWindowAttr.Find() == nil {
		dark := uint32(0)
		if pal.dark {
			dark = 1
		}
		pDwmSetWindowAttr.Call(st.hwnd, 20 /*DWMWA_USE_IMMERSIVE_DARK_MODE*/, uintptr(unsafe.Pointer(&dark)), 4)
	}
}

func settingsGoPage(i int) {
	if i < 0 || i >= len(st.pages) || i == st.page {
		return
	}
	st.page, st.scroll, st.hoverRow = i, 0, -1
	pInvalidateRect.Call(st.hwnd, 0, 0)
}

func settingsProc(hwnd, message, wparam, lparam uintptr) uintptr {
	switch uint32(message) {
	case wmEraseBkgnd:
		return 1

	case wmPaint:
		paintBuffered(hwnd, drawSettings)
		return 0

	case wmNCHitTest:
		// Sans barre de titre : la barre d'outils sert à déplacer la fenêtre.
		var wr rect
		pGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&wr)))
		x, y := int32(int16(lparam))-wr.Left, int32(int16(lparam>>16))-wr.Top
		if y >= 0 && y < toolbarH() && !inRect(st.lights, x, y) {
			return htCaption
		}
		return htClient

	case wmActivate:
		st.active = wparam&0xFFFF != waInactive
		pInvalidateRect.Call(hwnd, 0, 0)

	case wmMouseMove:
		x, y := lparamXY(lparam)
		if st.drag >= 0 {
			setSliderFromX(st.drag, x)
			return 0
		}
		hl := inRect(st.lights, x, y)
		row, seg := sHitTest(x, y)
		if row != st.hoverRow || seg != st.hoverSeg || hl != st.hoverLights {
			st.hoverRow, st.hoverSeg, st.hoverLights = row, seg, hl
			pInvalidateRect.Call(hwnd, 0, 0)
		}
		if !st.tracking {
			tme := trackMouseEvent{Flags: tmeLeave, HwndTrack: hwnd}
			tme.Size = uint32(unsafe.Sizeof(tme))
			pTrackMouseEvent.Call(uintptr(unsafe.Pointer(&tme)))
			st.tracking = true
		}
		return 0

	case wmMouseLeave:
		st.tracking, st.hoverRow, st.hoverSeg, st.hoverLights = false, -1, -1, false
		pInvalidateRect.Call(hwnd, 0, 0)
		return 0

	case wmMouseWheel:
		delta := int32(int16(wparam >> 16))
		ns := st.scroll - delta*sc(56)/120
		if m := settingsMaxScroll(); ns > m {
			ns = m
		}
		if ns < 0 {
			ns = 0
		}
		if ns != st.scroll {
			st.scroll = ns
			pInvalidateRect.Call(hwnd, 0, 0)
		}
		return 0

	case wmLButtonDown:
		x, y := lparamXY(lparam)
		for i, r := range st.side {
			if inRect(r, x, y) {
				settingsGoPage(i)
				return 0
			}
		}
		row, seg := sHitTest(x, y)
		st.pressRow, st.pressSeg = row, seg
		if row >= 0 && st.pages[st.page].rows[row].kind == rowSlider {
			st.drag, st.dragChanged = row, false
			pSetCapture.Call(hwnd)
			setSliderFromX(row, x)
		}
		return 0

	case wmLButtonUp:
		x, y := lparamXY(lparam)
		if st.drag >= 0 {
			st.drag = -1
			pReleaseCapture.Call()
			if st.dragChanged {
				settingChanged(true)
				// Recharge les icônes du Dock à la bonne taille une fois le curseur relâché.
				spriteCache = map[string]*dockgfx.Sprite{}
				dockSig = ""
				dockRefresh()
			}
			st.pressRow = -1
			return 0
		}
		for i := 0; i < 2; i++ {
			lr := lightRect(i)
			if inRect(rect{lr.Left - sc(3), lr.Top - sc(3), lr.Right + sc(3), lr.Bottom + sc(3)}, x, y) {
				if i == 0 {
					pDestroyWindow.Call(hwnd)
				} else {
					pShowWindow.Call(hwnd, swMinimize)
				}
				return 0
			}
		}
		row, seg := sHitTest(x, y)
		if row >= 0 && row == st.pressRow && seg == st.pressSeg {
			activateRow(row, seg)
		}
		st.pressRow = -1
		return 0

	case wmKeyDown:
		switch wparam {
		case vkEscape:
			pDestroyWindow.Call(hwnd)
		case vkDown:
			settingsGoPage(st.page + 1)
		case vkUp:
			settingsGoPage(st.page - 1)
		}
		return 0

	case wmClose:
		pDestroyWindow.Call(hwnd)
		return 0

	case wmDestroy:
		if hwnd == st.hwnd {
			st.hwnd = 0
			pDeleteObject.Call(st.fontBig)
			pDeleteObject.Call(st.fontIcon)
			st.fontBig, st.fontIcon = 0, 0
		}
		return 0
	}
	r, _, _ := pDefWindowProc.Call(hwnd, message, wparam, lparam)
	return r
}

var settingsClassOK bool

// showSettings ouvre la fenêtre de réglages (ou la ramène devant).
func showSettings() {
	if st.hwnd != 0 {
		pShowWindow.Call(st.hwnd, swRestore)
		pSetForeground.Call(st.hwnd)
		return
	}
	hInst, _, _ := pGetModuleHandle.Call(0)
	if !settingsClassOK {
		cursor, _, _ := pLoadCursor.Call(0, idcArrow)
		icon := appIcon(sc(32))
		wc := wndClassEx{Style: csDropShadow, WndProc: syscall.NewCallback(settingsProc), Instance: hInst, Cursor: cursor, Icon: icon, ClassName: utf16(settingsClass)}
		wc.Size = uint32(unsafe.Sizeof(wc))
		pRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
		settingsClassOK = true
	}
	buildPages()
	wa := workArea(0, 0)
	st.w, st.h = sc(780), sc(640)
	if max := wa.Bottom - wa.Top - sc(48); st.h > max {
		st.h = max
	}
	if max := wa.Right - wa.Left - sc(48); st.w > max {
		st.w = max
	}
	st.hoverRow, st.hoverSeg, st.pressRow, st.pressSeg, st.drag = -1, -1, -1, -1, -1
	st.scroll, st.active, st.hoverLights = 0, true, false

	x := wa.Left + (wa.Right-wa.Left-st.w)/2
	y := wa.Top + (wa.Bottom-wa.Top-st.h)/3
	if y < wa.Top {
		y = wa.Top
	}
	hwnd, _, _ := pCreateWindowEx.Call(0, ptr(utf16(settingsClass)),
		ptr(utf16(tr("Réglages de Lumen", "Lumen Settings"))), wsPopup|wsMinimizeBox,
		uintptr(x), uintptr(y), uintptr(st.w), uintptr(st.h), 0, 0, hInst, 0)
	if hwnd == 0 {
		return
	}
	st.hwnd = hwnd
	st.fontBig = makeFont(sc(19), 700)
	icon, _, _ := pCreateFont.Call(uintptr(-sc(13)), 0, 0, 0, 400, 0, 0, 0, 1, 0, 0, 5, 0, ptr(utf16("Segoe MDL2 Assets")))
	st.fontIcon = icon
	styleSettingsWindow()
	pShowWindow.Call(hwnd, swShow)
	pSetForeground.Call(hwnd)
}
