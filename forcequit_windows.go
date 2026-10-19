//go:build windows

package main

// « Forcer à quitter les applications » façon macOS : une petite fenêtre avec la
// liste des applications ouvertes (icône, nom, « ne répond pas » en rouge), et le
// bouton « Forcer à quitter ». L'Explorateur Windows (le « Finder » de Windows) se
// « relance » au lieu de se fermer. Raccourci : Ctrl+Alt+Échap, comme ⌥⌘⎋.

import (
	"fmt"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"menuwin/internal/dockgfx"
)

const fqClass = "MenuWindowsForceQuit"

var (
	pTerminateProcess = kernel32.NewProc("TerminateProcess")
	pIsHungAppWindow  = user32.NewProc("IsHungAppWindow")

	fqOK bool
	fq   struct {
		hwnd     uintptr
		prev     uintptr
		w, h     int32
		rows     []fqRow
		sel      int
		top      int // première ligne visible
		list     rect
		btn      rect
		hoverRow int
		hoverBtn bool
	}
)

const fqVisible = 7

type fqRow struct {
	name   string
	path   string
	hwnds  []uintptr
	frame  bool // appli du Store : on ferme la fenêtre, le processus est partagé
	shell  bool // l'Explorateur : « Relancer »
	hung   bool
	sprite *dockgfx.Sprite
}

func fqRowH() int32 { return sc(40) }

func fqCollect() {
	fq.rows = nil
	idx := map[string]int{}
	for _, w := range wins {
		path := cachedProcPath(w.hwnd)
		if path == "" {
			continue
		}
		var pid uint32
		pGetWindowThreadPI.Call(w.hwnd, uintptr(unsafe.Pointer(&pid)))
		if uintptr(pid) == ownPID {
			continue
		}
		base := filepath.Base(path)
		frame := strings.EqualFold(base, "ApplicationFrameHost.exe")
		key := normKey(path)
		if frame {
			key = fmt.Sprintf("frame:%x", w.hwnd)
		}
		hung, _, _ := pIsHungAppWindow.Call(w.hwnd)
		if i, ok := idx[key]; ok {
			fq.rows[i].hwnds = append(fq.rows[i].hwnds, w.hwnd)
			fq.rows[i].hung = fq.rows[i].hung || hung != 0
			continue
		}
		r := fqRow{path: path, hwnds: []uintptr{w.hwnd}, frame: frame, hung: hung != 0,
			shell: strings.EqualFold(base, "explorer.exe")}
		if frame {
			r.name = w.title
			r.sprite = windowSprite(w.hwnd)
		} else {
			r.name = appLabel(path)
			r.sprite = spriteForPath(path)
		}
		if r.sprite == nil {
			r.sprite = defaultSprite()
		}
		idx[key] = len(fq.rows)
		fq.rows = append(fq.rows, r)
	}
	// Une application qui ne répond plus passe en premier : c'est celle qu'on cherche.
	for i := 1; i < len(fq.rows); i++ {
		for j := i; j > 0 && fq.rows[j].hung && !fq.rows[j-1].hung; j-- {
			fq.rows[j], fq.rows[j-1] = fq.rows[j-1], fq.rows[j]
		}
	}
	fq.sel = -1
	for i, r := range fq.rows {
		if r.hung {
			fq.sel = i
			break
		}
	}
	if fq.sel < 0 && len(fq.rows) > 0 {
		fq.sel = 0
	}
	fq.top = 0
}

func fqRowRect(i int) rect {
	y := fq.list.Top + int32(i-fq.top)*fqRowH()
	return rect{fq.list.Left, y, fq.list.Right, y + fqRowH()}
}

func fqButtonText() string {
	if fq.sel >= 0 && fq.sel < len(fq.rows) && fq.rows[fq.sel].shell {
		return tr("Relancer", "Relaunch")
	}
	return tr("Forcer à quitter", "Force Quit")
}

func fqDraw(hdc uintptr, w, h int32) {
	full := rect{0, 0, w, h}
	pFillRect.Call(hdc, uintptr(unsafe.Pointer(&full)), brushMenuBg)
	pSetBkMode.Call(hdc, transparent)
	pad := sc(20)
	pSelectObject.Call(hdc, fontBold)
	pSetTextColor.Call(hdc, pal.menuText)
	drawText(hdc, tr("Forcer à quitter les applications", "Force Quit Applications"),
		rect{pad, pad, w - pad, pad + sc(24)}, dtLeft|dtEllipsis)
	pSelectObject.Call(hdc, panelFont.small)
	pSetTextColor.Call(hdc, pal.menuKey)
	l1, l2 := tr("Si une application ne répond plus, sélectionnez-la", "If an app stops responding, select it in the list,"),
		tr("dans la liste, puis cliquez sur « Forcer à quitter ».", "then click Force Quit.")
	drawText(hdc, l1, rect{pad, pad + sc(28), w - pad, pad + sc(46)}, dtLeft|dtEllipsis)
	drawText(hdc, l2, rect{pad, pad + sc(46), w - pad, pad + sc(64)}, dtLeft|dtEllipsis)

	fillRR(hdc, fq.list, sc(12), cardColor())
	n := len(fq.rows)
	if n == 0 {
		pSelectObject.Call(hdc, panelFont.row)
		pSetTextColor.Call(hdc, pal.menuKey)
		drawText(hdc, tr("Aucune application ouverte", "No open applications"), fq.list, dtCenter)
	}
	for i := fq.top; i < n && i < fq.top+fqVisible; i++ {
		r := fqRowRect(i)
		row := fq.rows[i]
		txt, sub := pal.menuText, pal.menuKey
		if i == fq.sel {
			fillRR(hdc, rect{r.Left + sc(4), r.Top + sc(2), r.Right - sc(4), r.Bottom - sc(2)}, sc(8), pal.accent)
			txt, sub = rgb(255, 255, 255), rgb(235, 235, 245)
		} else if i == fq.hoverRow {
			fillRR(hdc, rect{r.Left + sc(4), r.Top + sc(2), r.Right - sc(4), r.Bottom - sc(2)}, sc(8), blendColor(pal.menuBg, pal.menuText, 0.10))
		}
		blitSprite(hdc, row.sprite, r.Left+sc(12), r.Top+(fqRowH()-sc(26))/2, sc(26))
		textR := rect{r.Left + sc(48), r.Top, r.Right - sc(12), r.Bottom}
		pSelectObject.Call(hdc, panelFont.row)
		pSetTextColor.Call(hdc, txt)
		if row.hung {
			nameR := textR
			nameR.Bottom = r.Top + fqRowH()/2 + sc(4)
			drawText(hdc, row.name, nameR, dtLeft|dtEllipsis)
			pSelectObject.Call(hdc, panelFont.small)
			if i == fq.sel {
				pSetTextColor.Call(hdc, sub)
			} else {
				pSetTextColor.Call(hdc, rgb(255, 69, 58))
			}
			sr := textR
			sr.Top = nameR.Bottom - sc(4)
			drawText(hdc, tr("(ne répond pas)", "(not responding)"), sr, dtLeft|dtEllipsis)
		} else {
			drawText(hdc, row.name, textR, dtLeft|dtEllipsis)
		}
	}
	// Barre de défilement discrète s'il y a plus de lignes que de place.
	if n > fqVisible {
		th := fq.list.Bottom - fq.list.Top - sc(8)
		bh := th * fqVisible / int32(n)
		by := fq.list.Top + sc(4) + th*int32(fq.top)/int32(n)
		fillRR(hdc, rect{fq.list.Right - sc(7), by, fq.list.Right - sc(3), by + bh}, sc(4), blendColor(pal.menuBg, pal.menuText, 0.30))
	}

	enabled := fq.sel >= 0 && fq.sel < n
	var bg, fg uintptr
	switch {
	case !enabled:
		bg, fg = cardColor(), pal.menuKey
	case fq.hoverBtn:
		bg, fg = rgb(255, 95, 86), rgb(255, 255, 255)
	default:
		bg, fg = rgb(255, 69, 58), rgb(255, 255, 255)
	}
	fillRR(hdc, fq.btn, sc(12), bg)
	pSelectObject.Call(hdc, fontBold)
	pSetTextColor.Call(hdc, fg)
	drawText(hdc, fqButtonText(), fq.btn, dtCenter)
	pSelectObject.Call(hdc, fontReg)
}

func fqRowAt(x, y int32) int {
	if !inRect(fq.list, x, y) {
		return -1
	}
	i := fq.top + int((y-fq.list.Top)/fqRowH())
	if i < 0 || i >= len(fq.rows) || i >= fq.top+fqVisible {
		return -1
	}
	return i
}

func fqScrollTo(i int) {
	if i < fq.top {
		fq.top = i
	}
	if i >= fq.top+fqVisible {
		fq.top = i - fqVisible + 1
	}
}

func fqMax() int {
	m := len(fq.rows) - fqVisible
	if m < 0 {
		m = 0
	}
	return m
}

func fqProc(hwnd, message, wparam, lparam uintptr) uintptr {
	if hwnd == fq.hwnd && fq.hwnd != 0 {
		switch uint32(message) {
		case wmEraseBkgnd:
			return 1
		case wmPaint:
			paintBuffered(hwnd, fqDraw)
			return 0
		case wmActivate:
			if wparam&0xFFFF == waInactive {
				closeForceQuit(false)
			}
			return 0
		case wmMouseMove:
			x, y := lparamXY(lparam)
			hr, hb := fqRowAt(x, y), inRect(fq.btn, x, y)
			if hr != fq.hoverRow || hb != fq.hoverBtn {
				fq.hoverRow, fq.hoverBtn = hr, hb
				pInvalidateRect.Call(hwnd, 0, 0)
			}
			return 0
		case wmMouseWheel:
			if int16(wparam>>16) > 0 && fq.top > 0 {
				fq.top--
			} else if int16(wparam>>16) < 0 && fq.top < fqMax() {
				fq.top++
			}
			pInvalidateRect.Call(hwnd, 0, 0)
			return 0
		case wmLButtonDown:
			x, y := lparamXY(lparam)
			if i := fqRowAt(x, y); i >= 0 {
				fq.sel = i
				pInvalidateRect.Call(hwnd, 0, 0)
			}
			return 0
		case wmLButtonUp:
			x, y := lparamXY(lparam)
			if inRect(fq.btn, x, y) {
				fqGo()
			}
			return 0
		case wmKeyDown:
			switch wparam {
			case vkEscape:
				closeForceQuit(true)
			case vkReturn:
				fqGo()
			case vkDown:
				if fq.sel < len(fq.rows)-1 {
					fq.sel++
					fqScrollTo(fq.sel)
					pInvalidateRect.Call(hwnd, 0, 0)
				}
			case vkUp:
				if fq.sel > 0 {
					fq.sel--
					fqScrollTo(fq.sel)
					pInvalidateRect.Call(hwnd, 0, 0)
				}
			}
			return 0
		case wmClose:
			closeForceQuit(true)
			return 0
		}
	}
	r, _, _ := pDefWindowProc.Call(hwnd, message, wparam, lparam)
	return r
}

// fqGo : confirmation, puis fermeture forcée de l'application choisie.
func fqGo() {
	if fq.sel < 0 || fq.sel >= len(fq.rows) {
		return
	}
	row := fq.rows[fq.sel]
	closeForceQuit(false)
	var q, ok string
	if row.shell {
		q = fmt.Sprintf(tr("Voulez-vous vraiment relancer « %s » ?", "Do you want to relaunch “%s”?"), row.name)
		ok = tr("Relancer", "Relaunch")
	} else {
		q = fmt.Sprintf(tr("Voulez-vous vraiment forcer « %s » à quitter ?", "Do you want to force “%s” to quit?"), row.name)
		ok = tr("Forcer à quitter", "Force Quit")
	}
	if !confirm(barHwnd, q, ok) {
		return
	}
	if row.shell {
		go func() {
			run("taskkill.exe", "/f", "/im", "explorer.exe")
			// Windows relance normalement le shell ; sinon on le fait nous-mêmes.
			time.Sleep(2500 * time.Millisecond)
			if h, _, _ := pFindWindowEx.Call(0, 0, ptr(utf16("Shell_TrayWnd")), 0); h == 0 {
				run("explorer.exe")
			}
		}()
		return
	}
	if row.frame {
		for _, h := range row.hwnds {
			pPostMessage.Call(h, wmClose, 0, 0)
		}
		return
	}
	done := map[uint32]bool{}
	for _, h := range row.hwnds {
		var pid uint32
		pGetWindowThreadPI.Call(h, uintptr(unsafe.Pointer(&pid)))
		if pid == 0 || done[pid] || uintptr(pid) == ownPID {
			continue
		}
		done[pid] = true
		if p, _, _ := pOpenProcess.Call(0x0001 /*PROCESS_TERMINATE*/, 0, uintptr(pid)); p != 0 {
			pTerminateProcess.Call(p, 1)
			pCloseHandle.Call(p)
		}
	}
}

func showForceQuit() {
	if fq.hwnd != 0 {
		closeForceQuit(true)
		return
	}
	initPanelFonts()
	hideLabel()
	if !fqOK {
		hInst, _, _ := pGetModuleHandle.Call(0)
		cursor, _, _ := pLoadCursor.Call(0, idcArrow)
		wc := wndClassEx{WndProc: syscall.NewCallback(fqProc), Instance: hInst, Cursor: cursor, ClassName: utf16(fqClass)}
		wc.Size = uint32(unsafe.Sizeof(wc))
		pRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
		fqOK = true
	}
	refresh()
	fqCollect()
	fq.hoverRow, fq.hoverBtn = -1, false
	fq.w = sc(400)
	pad := sc(20)
	rows := len(fq.rows)
	if rows > fqVisible {
		rows = fqVisible
	}
	if rows < 3 {
		rows = 3
	}
	listTop := pad + sc(76)
	fq.list = rect{pad, listTop, fq.w - pad, listTop + int32(rows)*fqRowH()}
	fq.btn = rect{fq.w - pad - sc(160), fq.list.Bottom + sc(16), fq.w - pad, fq.list.Bottom + sc(16) + sc(36)}
	fq.h = fq.btn.Bottom + pad
	cur := cursorPt()
	m := monitorRect(cur.X, cur.Y)
	x := m.Left + (m.Right-m.Left-fq.w)/2
	y := m.Top + (m.Bottom-m.Top-fq.h)/3
	fq.prev, _, _ = pGetForeground.Call()
	hInst, _, _ := pGetModuleHandle.Call(0)
	hwnd, _, _ := pCreateWindowEx.Call(wsExTopmost|wsExToolwin, ptr(utf16(fqClass)), ptr(utf16("Force Quit")), wsPopup,
		uintptr(x), uintptr(y), uintptr(fq.w), uintptr(fq.h), 0, 0, hInst, 0)
	if hwnd == 0 {
		return
	}
	fq.hwnd = hwnd
	styleWindow(hwnd, fq.w, fq.h)
	pShowWindow.Call(hwnd, swShow)
	pSetForeground.Call(hwnd)
	pSetFocus.Call(hwnd)
}

func closeForceQuit(restore bool) {
	h := fq.hwnd
	if h == 0 {
		return
	}
	fq.hwnd = 0
	fq.rows = nil
	pDestroyWindow.Call(h)
	if restore && fq.prev != 0 && fq.prev != barHwnd {
		if ok, _, _ := pIsWindow.Call(fq.prev); ok != 0 {
			pSetForeground.Call(fq.prev)
		}
	}
}
