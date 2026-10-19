//go:build windows

package main

// Le calendrier de l'horloge : un clic sur la date et l'heure ouvre un panneau avec
// le jour en grand et le calendrier du mois (aujourd'hui entouré), comme le centre
// de notifications de macOS. Flèches ‹ › ou molette pour changer de mois, le point
// ramène à aujourd'hui. Clic droit sur l'horloge : les réglages de l'horloge.

import (
	"strconv"
	"syscall"
	"time"
	"unsafe"

	"menuwin/internal/calendar"
)

const calClass = "MenuWindowsCalendar"

var cal struct {
	hwnd        uintptr
	prev        uintptr
	x, y, w, h  int32
	year        int
	month       time.Month
	prevR       rect
	nextR       rect
	todayR      rect
	footR       rect
	hoverFooter bool
}

var calClassOK bool

func calMondayFirst() bool { return langFR }

func calLayout() {
	pad := sc(14)
	top := pad + sc(72) // en-tête : jour et date
	cal.prevR = rect{cal.w - pad - sc(96), top, cal.w - pad - sc(64), top + sc(28)}
	cal.todayR = rect{cal.w - pad - sc(64), top, cal.w - pad - sc(32), top + sc(28)}
	cal.nextR = rect{cal.w - pad - sc(32), top, cal.w - pad, top + sc(28)}
	cal.footR = rect{pad, cal.h - pad - sc(34), cal.w - pad, cal.h - pad}
}

func calCell(row, col int) rect {
	pad := sc(14)
	cw := (cal.w - 2*pad) / 7
	top := pad + sc(72) + sc(28) + sc(26) // sous le titre du mois et les initiales
	ch := sc(34)
	return rect{pad + int32(col)*cw, top + int32(row)*ch, pad + int32(col+1)*cw, top + int32(row+1)*ch}
}

func drawCalendar(hdc uintptr, w, h int32) {
	full := rect{0, 0, w, h}
	pFillRect.Call(hdc, uintptr(unsafe.Pointer(&full)), brushMenuBg)
	pSetBkMode.Call(hdc, transparent)
	pad := sc(14)
	now := time.Now()

	// En-tête : le jour de la semaine en rouge, la date en grand.
	pSelectObject.Call(hdc, fontBold)
	pSetTextColor.Call(hdc, rgb(255, 69, 58))
	drawText(hdc, calendar.DayName(now.Weekday(), langFR), rect{pad, pad, w - pad, pad + sc(22)}, dtLeft)
	pSelectObject.Call(hdc, panelFont.big)
	pSetTextColor.Call(hdc, pal.menuText)
	drawText(hdc, strconv.Itoa(now.Day())+" "+calendar.MonthName(now.Month(), langFR)+" "+strconv.Itoa(now.Year()),
		rect{pad, pad + sc(22), w - pad, pad + sc(60)}, dtLeft)

	// Titre du mois et boutons.
	top := pad + sc(72)
	pSelectObject.Call(hdc, fontBold)
	pSetTextColor.Call(hdc, pal.menuText)
	title := calendar.MonthName(cal.month, langFR) + " " + strconv.Itoa(cal.year)
	drawText(hdc, title, rect{pad, top, cal.prevR.Left, top + sc(28)}, dtLeft|dtEllipsis)
	drawGlyphIn(hdc, panelFont.glyph, "", cal.prevR, pal.menuText)
	drawGlyphIn(hdc, panelFont.glyph, "", cal.nextR, pal.menuText)
	d := sc(8)
	cx, cy := (cal.todayR.Left+cal.todayR.Right)/2, (cal.todayR.Top+cal.todayR.Bottom)/2
	fillEllipse(hdc, rect{cx - d/2, cy - d/2, cx - d/2 + d, cy - d/2 + d}, pal.menuText)

	// Initiales des jours.
	pSelectObject.Call(hdc, panelFont.small)
	pSetTextColor.Call(hdc, pal.menuKey)
	for c, ini := range calendar.Initials(calMondayFirst(), langFR) {
		r := calCell(0, c)
		drawText(hdc, ini, rect{r.Left, r.Top - sc(26), r.Right, r.Top}, dtCenter)
	}

	// Jours.
	grid := calendar.Grid(cal.year, cal.month, calMondayFirst())
	pSelectObject.Call(hdc, panelFont.row)
	for row := 0; row < 6; row++ {
		for col := 0; col < 7; col++ {
			c := grid[row][col]
			r := calCell(row, col)
			isToday := c.Year == now.Year() && c.Month == now.Month() && c.Day == now.Day()
			col2 := pal.menuText
			if !c.InMonth {
				col2 = pal.menuOff
			}
			if isToday {
				dd := sc(30)
				mx, my := (r.Left+r.Right)/2, (r.Top+r.Bottom)/2
				fillEllipse(hdc, rect{mx - dd/2, my - dd/2, mx - dd/2 + dd, my - dd/2 + dd}, rgb(255, 69, 58))
				col2 = rgb(255, 255, 255)
			}
			pSetTextColor.Call(hdc, col2)
			drawText(hdc, strconv.Itoa(c.Day), r, dtCenter)
		}
	}

	// Pied : réglages de la date et de l'heure.
	bg := cardColor()
	if cal.hoverFooter {
		bg = blendColor(pal.menuBg, pal.menuText, 0.14)
	}
	fillRR(hdc, cal.footR, sc(12), bg)
	pSelectObject.Call(hdc, fontReg)
	pSetTextColor.Call(hdc, pal.menuText)
	drawText(hdc, tr("Réglages de date et d’heure…", "Date & Time Settings…"),
		rect{cal.footR.Left + sc(14), cal.footR.Top, cal.footR.Right - sc(10), cal.footR.Bottom}, dtLeft|dtEllipsis)
}

func calMove(delta int) {
	cal.year, cal.month = calendar.Add(cal.year, cal.month, delta)
	pInvalidateRect.Call(cal.hwnd, 0, 0)
}

func calProc(hwnd, message, wparam, lparam uintptr) uintptr {
	if hwnd != cal.hwnd {
		r, _, _ := pDefWindowProc.Call(hwnd, message, wparam, lparam)
		return r
	}
	switch uint32(message) {
	case wmEraseBkgnd:
		return 1
	case wmPaint:
		paintBuffered(hwnd, func(mem uintptr, w, h int32) { drawCalendar(mem, w, h) })
		return 0
	case wmActivate:
		if wparam&0xFFFF == waInactive {
			closeCalendar(false)
		}
		return 0
	case wmMouseMove:
		x, y := lparamXY(lparam)
		if in := inRect(cal.footR, x, y); in != cal.hoverFooter {
			cal.hoverFooter = in
			pInvalidateRect.Call(hwnd, 0, 0)
		}
		return 0
	case wmLButtonUp:
		x, y := lparamXY(lparam)
		switch {
		case inRect(cal.prevR, x, y):
			calMove(-1)
		case inRect(cal.nextR, x, y):
			calMove(1)
		case inRect(cal.todayR, x, y):
			now := time.Now()
			cal.year, cal.month = now.Year(), now.Month()
			pInvalidateRect.Call(hwnd, 0, 0)
		case inRect(cal.footR, x, y):
			closeCalendar(true)
			openURI("ms-settings:dateandtime")
		}
		return 0
	case wmMouseWheel:
		if int16(wparam>>16) > 0 {
			calMove(-1)
		} else {
			calMove(1)
		}
		return 0
	case wmKeyDown:
		switch wparam {
		case vkEscape:
			closeCalendar(true)
		case vkLeft, vkUp, vkPrior:
			calMove(-1)
		case vkRight, vkDown, vkNext:
			calMove(1)
		}
		return 0
	case wmClose:
		closeCalendar(true)
		return 0
	}
	r, _, _ := pDefWindowProc.Call(hwnd, message, wparam, lparam)
	return r
}

func registerCalendarClass() {
	if calClassOK {
		return
	}
	hInst, _, _ := pGetModuleHandle.Call(0)
	cursor, _, _ := pLoadCursor.Call(0, idcArrow)
	wc := wndClassEx{WndProc: syscall.NewCallback(calProc), Instance: hInst, Cursor: cursor, ClassName: utf16(calClass)}
	wc.Size = uint32(unsafe.Sizeof(wc))
	pRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
	calClassOK = true
}

// showCalendar ouvre le calendrier sous l'horloge (ou le referme).
func showCalendar() {
	if cal.hwnd != 0 {
		closeCalendar(true)
		return
	}
	if menuOpen || dialogOpen {
		return
	}
	registerCalendarClass()
	initPanelFonts()
	hideLabel()

	r, ok := hitRect(hitClock)
	if !ok {
		return
	}
	sx, sy := screenPos(barHwnd, r)
	var wr rect
	pGetWindowRect.Call(barHwnd, uintptr(unsafe.Pointer(&wr)))
	wa := workArea(sx, sy)

	cal.w = sc(320)
	cal.h = sc(14) + sc(72) + sc(28) + sc(26) + 6*sc(34) + sc(12) + sc(34) + sc(14)
	cal.x = wr.Left + r.Right
	if cal.x > wa.Right-sc(8) {
		cal.x = wa.Right - sc(8)
	}
	cal.x -= cal.w
	if cal.x < wa.Left {
		cal.x = wa.Left
	}
	cal.y = sy + sc(6)
	now := time.Now()
	cal.year, cal.month = now.Year(), now.Month()
	cal.hoverFooter = false
	calLayout()

	cal.prev, _, _ = pGetForeground.Call()
	hInst, _, _ := pGetModuleHandle.Call(0)
	hwnd, _, _ := pCreateWindowEx.Call(wsExTopmost|wsExToolwin, ptr(utf16(calClass)), ptr(utf16("Calendar")), wsPopup,
		uintptr(cal.x), uintptr(cal.y), uintptr(cal.w), uintptr(cal.h), 0, 0, hInst, 0)
	if hwnd == 0 {
		return
	}
	cal.hwnd = hwnd
	styleWindow(hwnd, cal.w, cal.h)
	pShowWindow.Call(hwnd, swShow)
	pSetForeground.Call(hwnd)
	pSetFocus.Call(hwnd)
	invalidate(barHwnd)
}

func closeCalendar(restore bool) {
	h := cal.hwnd
	if h == 0 {
		return
	}
	cal.hwnd = 0
	pDestroyWindow.Call(h)
	if barHwnd != 0 {
		invalidate(barHwnd)
	}
	if restore && cal.prev != 0 && cal.prev != barHwnd {
		if ok, _, _ := pIsWindow.Call(cal.prev); ok != 0 {
			pSetForeground.Call(cal.prev)
		}
	}
}
