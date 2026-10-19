//go:build windows

package main

// Widgets du bureau (réglage « Widgets sur le bureau »), comme ceux de macOS :
// une horloge analogique, la date du jour et la batterie, en cartes arrondies en haut à
// droite du bureau. La fenêtre reste derrière toutes les autres, ne prend jamais le
// focus et laisse passer les clics.

import (
	"math"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"menuwin/internal/calendar"
)

const (
	wgClass       = "MenuWindowsWidgets"
	wgExLayered   = 0x00080000
	wgExTransp    = 0x00000020
	wgExNoActive  = 0x08000000
	wgSwpNoSize   = 0x0001
	wgSwpNoMove   = 0x0002
	wgHwndBottom  = 1
	wgRgnOr       = 2
	wgTimerSecond = 1
)

var (
	pCreateRoundRectRgn = gdi32.NewProc("CreateRoundRectRgn")
	pCombineRgn         = gdi32.NewProc("CombineRgn")
	pCreateRectRgn      = gdi32.NewProc("CreateRectRgn")

	wg struct {
		hwnd     uintptr
		w, h     int32
		clockR   rect
		calR     rect
		battR    rect
		hasBatt  bool
		dayFont  uintptr
		classOK  bool
		lastBatt int
	}
)

func wgLayout() {
	cw := sc(168)
	gap := sc(12)
	wg.w = cw
	wg.clockR = rect{0, 0, cw, cw}
	wg.calR = rect{0, cw + gap, cw, 2*cw + gap}
	wg.h = 2*cw + gap
	wg.hasBatt = battPct >= 0
	wg.lastBatt = battPct
	if wg.hasBatt {
		wg.battR = rect{0, wg.h + gap, cw, wg.h + gap + sc(70)}
		wg.h = wg.battR.Bottom
	}
}

func wgLine(hdc uintptr, x0, y0, x1, y1, width int32, col uintptr) {
	pen, _, _ := pCreatePen.Call(0, uintptr(width), col)
	old, _, _ := pSelectObject.Call(hdc, pen)
	pMoveToEx.Call(hdc, uintptr(x0), uintptr(y0), 0)
	pLineTo.Call(hdc, uintptr(x1), uintptr(y1))
	pSelectObject.Call(hdc, old)
	pDeleteObject.Call(pen)
}

func wgHand(hdc uintptr, cx, cy int32, angle, length float64, width int32, col uintptr) {
	x := cx + int32(math.Sin(angle)*length)
	y := cy - int32(math.Cos(angle)*length)
	wgLine(hdc, cx, cy, x, y, width, col)
}

func wgDrawClock(hdc uintptr, r rect, now time.Time) {
	fillRR(hdc, r, sc(36), cardColor())
	cx, cy := (r.Left+r.Right)/2, (r.Top+r.Bottom)/2
	rad := float64((r.Right-r.Left)/2 - sc(14))
	for i := 0; i < 12; i++ {
		a := float64(i) * math.Pi / 6
		l := float64(sc(5))
		w := sc(1)
		if i%3 == 0 {
			l, w = float64(sc(9)), sc(2)
		}
		x0, y0 := cx+int32(math.Sin(a)*rad), cy-int32(math.Cos(a)*rad)
		x1, y1 := cx+int32(math.Sin(a)*(rad-l)), cy-int32(math.Cos(a)*(rad-l))
		wgLine(hdc, x0, y0, x1, y1, w, pal.menuKey)
	}
	h := float64(now.Hour()%12) + float64(now.Minute())/60
	m := float64(now.Minute()) + float64(now.Second())/60
	s := float64(now.Second())
	wgHand(hdc, cx, cy, h*math.Pi/6, rad*0.5, sc(4), pal.menuText)
	wgHand(hdc, cx, cy, m*math.Pi/30, rad*0.8, sc(3), pal.menuText)
	red := rgb(255, 149, 0)
	wgHand(hdc, cx, cy, s*math.Pi/30, rad*0.88, sc(1), red)
	d := sc(8)
	fillEllipse(hdc, rect{cx - d/2, cy - d/2, cx + d/2, cy + d/2}, red)
}

func wgDrawDate(hdc uintptr, r rect, now time.Time) {
	fillRR(hdc, r, sc(36), cardColor())
	pad := sc(16)
	pSetBkMode.Call(hdc, transparent)
	pSelectObject.Call(hdc, fontBold)
	pSetTextColor.Call(hdc, rgb(255, 69, 58))
	drawText(hdc, strings.ToUpper(calendar.DayName(now.Weekday(), langFR)), rect{r.Left + pad, r.Top + pad, r.Right - pad, r.Top + pad + sc(22)}, dtLeft)
	pSelectObject.Call(hdc, wg.dayFont)
	pSetTextColor.Call(hdc, pal.menuText)
	drawText(hdc, strconv.Itoa(now.Day()), rect{r.Left + pad, r.Top + pad + sc(22), r.Right - pad, r.Top + pad + sc(92)}, dtLeft)
	pSelectObject.Call(hdc, panelFont.row)
	pSetTextColor.Call(hdc, pal.menuKey)
	drawText(hdc, calendar.MonthName(now.Month(), langFR)+" "+strconv.Itoa(now.Year()),
		rect{r.Left + pad, r.Bottom - pad - sc(24), r.Right - pad, r.Bottom - pad}, dtLeft|dtEllipsis)
}

func wgDrawBattery(hdc uintptr, r rect) {
	fillRR(hdc, r, sc(28), cardColor())
	pad := sc(16)
	pSetBkMode.Call(hdc, transparent)
	pSelectObject.Call(hdc, panelFont.small)
	pSetTextColor.Call(hdc, pal.menuKey)
	label := tr("Batterie", "Battery")
	if battCharging {
		label = tr("En charge", "Charging")
	}
	drawText(hdc, label, rect{r.Left + pad, r.Top + sc(10), r.Right - pad, r.Top + sc(28)}, dtLeft)
	pSelectObject.Call(hdc, panelFont.big)
	pSetTextColor.Call(hdc, pal.menuText)
	drawText(hdc, strconv.Itoa(battPct)+" %", rect{r.Left + pad, r.Top + sc(24), r.Right - pad, r.Top + sc(52)}, dtLeft)
	track := rect{r.Left + pad, r.Bottom - sc(16), r.Right - pad, r.Bottom - sc(10)}
	fillRR(hdc, track, sc(6), blendColor(pal.menuBg, pal.menuText, 0.16))
	fill := track
	fill.Right = track.Left + (track.Right-track.Left)*int32(battPct)/100
	col := rgb(52, 199, 89)
	if battPct <= 20 && !battCharging {
		col = rgb(255, 69, 58)
	}
	if fill.Right-fill.Left >= sc(6) {
		fillRR(hdc, fill, sc(6), col)
	}
}

func wgPaint(mem uintptr, w, h int32) {
	full := rect{0, 0, w, h}
	pFillRect.Call(mem, uintptr(unsafe.Pointer(&full)), brushMenuBg)
	now := time.Now()
	wgDrawClock(mem, wg.clockR, now)
	wgDrawDate(mem, wg.calR, now)
	if wg.hasBatt {
		wgDrawBattery(mem, wg.battR)
	}
}

func wgSetRegion() {
	rgn, _, _ := pCreateRectRgn.Call(0, 0, 0, 0)
	d := uintptr(sc(36))
	for _, r := range []rect{wg.clockR, wg.calR} {
		part, _, _ := pCreateRoundRectRgn.Call(uintptr(r.Left), uintptr(r.Top), uintptr(r.Right+1), uintptr(r.Bottom+1), d, d)
		pCombineRgn.Call(rgn, rgn, part, wgRgnOr)
		pDeleteObject.Call(part)
	}
	if wg.hasBatt {
		part, _, _ := pCreateRoundRectRgn.Call(uintptr(wg.battR.Left), uintptr(wg.battR.Top), uintptr(wg.battR.Right+1), uintptr(wg.battR.Bottom+1), uintptr(sc(28)), uintptr(sc(28)))
		pCombineRgn.Call(rgn, rgn, part, wgRgnOr)
		pDeleteObject.Call(part)
	}
	pSetWindowRgn.Call(wg.hwnd, rgn, 1)
}

func wgToBottom() {
	if wg.hwnd != 0 {
		pSetWindowPos.Call(wg.hwnd, wgHwndBottom, 0, 0, 0, 0, wgSwpNoMove|wgSwpNoSize|swpNoActivate)
	}
}

func wgProc(hwnd, message, wparam, lparam uintptr) uintptr {
	if hwnd == wg.hwnd && wg.hwnd != 0 {
		switch uint32(message) {
		case wmEraseBkgnd:
			return 1
		case wmPaint:
			paintBuffered(hwnd, wgPaint)
			return 0
		case wmTimer:
			if battPct != wg.lastBatt && (battPct >= 0) != wg.hasBatt {
				refreshWidgets() // la batterie apparaît ou disparaît
				return 0
			}
			pInvalidateRect.Call(hwnd, 0, 0)
			wgToBottom()
			return 0
		}
	}
	r, _, _ := pDefWindowProc.Call(hwnd, message, wparam, lparam)
	return r
}

func showWidgets() {
	if wg.hwnd != 0 {
		return
	}
	initPanelFonts()
	if !wg.classOK {
		hInst, _, _ := pGetModuleHandle.Call(0)
		wc := wndClassEx{WndProc: syscall.NewCallback(wgProc), Instance: hInst, ClassName: utf16(wgClass)}
		wc.Size = uint32(unsafe.Sizeof(wc))
		pRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
		wg.classOK = true
	}
	wgLayout()
	wa := workArea(0, 0)
	x := wa.Right - wg.w - sc(16)
	y := wa.Top + sc(14)
	hInst, _, _ := pGetModuleHandle.Call(0)
	hwnd, _, _ := pCreateWindowEx.Call(wgExLayered|wgExTransp|wsExToolwin|wgExNoActive, ptr(utf16(wgClass)), ptr(utf16("Widgets")), wsPopup,
		uintptr(x), uintptr(y), uintptr(wg.w), uintptr(wg.h), 0, 0, hInst, 0)
	if hwnd == 0 {
		return
	}
	wg.hwnd = hwnd
	wg.dayFont = makeFont(sc(52), 300)
	pSetLayeredWindowAttributes.Call(hwnd, 0, 255, lwaAlpha)
	wgSetRegion()
	pShowWindow.Call(hwnd, swShowNoActivate)
	wgToBottom()
	pSetTimer.Call(hwnd, wgTimerSecond, 1000, 0)
}

func closeWidgets() {
	h := wg.hwnd
	if h == 0 {
		return
	}
	wg.hwnd = 0
	pKillTimer.Call(h, wgTimerSecond)
	pDestroyWindow.Call(h)
	if wg.dayFont != 0 {
		pDeleteObject.Call(wg.dayFont)
		wg.dayFont = 0
	}
}

func refreshWidgets() {
	closeWidgets()
	if cfg.Widgets {
		showWidgets()
	}
}

// applyWidgets affiche ou retire les widgets selon le réglage, et les redessine (thème, langue).
func applyWidgets() {
	switch {
	case cfg.Widgets && wg.hwnd == 0:
		showWidgets()
	case !cfg.Widgets && wg.hwnd != 0:
		closeWidgets()
	case wg.hwnd != 0:
		pInvalidateRect.Call(wg.hwnd, 0, 0)
	}
}
