//go:build windows

package main

// « À propos de ce PC » façon « À propos de ce Mac » : une petite fenêtre avec le nom du
// système, sa version, le processeur, la mémoire, les graphiques, le disque et la durée
// de fonctionnement, plus un bouton vers les informations détaillées de Windows.

import (
	"fmt"
	"os"
	"strings"
	"syscall"
	"unsafe"
)

const (
	aboutClass     = "MenuWindowsAbout"
	hkeyLocalMachn = 0x80000002
)

var (
	pGlobalMemoryStatusEx = kernel32.NewProc("GlobalMemoryStatusEx")
	pGetTickCount64       = kernel32.NewProc("GetTickCount64")
	pGetDiskFreeSpaceEx   = kernel32.NewProc("GetDiskFreeSpaceExW")

	aboutOK bool
	about   struct {
		hwnd       uintptr
		prev       uintptr
		w, h       int32
		rows       [][2]string
		title, sub string
		btn        rect
		hoverBtn   bool
	}
)

type memoryStatusEx struct {
	Length               uint32
	Load                 uint32
	TotalPhys, AvailPhys uint64
	TotalPage, AvailPage uint64
	TotalVirt, AvailVirt uint64
	AvailExtVirt         uint64
}

func regLMString(subkey, name string) string {
	buf := make([]uint16, 512)
	n := uint32(len(buf) * 2)
	r, _, _ := pRegGetValue.Call(hkeyLocalMachn, ptr(utf16(subkey)), ptr(utf16(name)), 0x2 /*RRF_RT_REG_SZ*/, 0,
		uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&n)))
	if r != 0 {
		return ""
	}
	return strings.TrimSpace(syscall.UTF16ToString(buf))
}

func regLMDword(subkey, name string) (uint32, bool) {
	var v uint32
	n := uint32(4)
	r, _, _ := pRegGetValue.Call(hkeyLocalMachn, ptr(utf16(subkey)), ptr(utf16(name)), rrfRtDword, 0,
		uintptr(unsafe.Pointer(&v)), uintptr(unsafe.Pointer(&n)))
	return v, r == 0
}

func gigabytes(b uint64) string {
	g := float64(b) / (1 << 30)
	unit := tr(" Go", " GB")
	if g >= 100 {
		return fmt.Sprintf("%.0f%s", g, unit)
	}
	return fmt.Sprintf("%.1f%s", g, unit)
}

func aboutCollect() {
	const nt = `SOFTWARE\Microsoft\Windows NT\CurrentVersion`
	name := regLMString(nt, "ProductName")
	build := regLMString(nt, "CurrentBuild")
	if b := 0; build != "" {
		fmt.Sscanf(build, "%d", &b)
		if b >= 22000 {
			name = strings.Replace(name, "Windows 10", "Windows 11", 1)
		}
	}
	if name == "" {
		name = "Windows"
	}
	about.title = name
	ver := regLMString(nt, "DisplayVersion")
	if ver == "" {
		ver = regLMString(nt, "ReleaseId")
	}
	sub := tr("Version ", "Version ")
	if ver != "" {
		sub += ver
	}
	if build != "" {
		if ubr, ok := regLMDword(nt, "UBR"); ok {
			sub += fmt.Sprintf(" (%s.%d)", build, ubr)
		} else {
			sub += " (" + build + ")"
		}
	}
	about.sub = sub

	var rows [][2]string
	host, _ := os.Hostname()
	if host != "" {
		rows = append(rows, [2]string{tr("Nom", "Name"), host})
	}
	if cpu := regLMString(`HARDWARE\DESCRIPTION\System\CentralProcessor\0`, "ProcessorNameString"); cpu != "" {
		rows = append(rows, [2]string{tr("Processeur", "Processor"), strings.Join(strings.Fields(cpu), " ")})
	}
	var ms memoryStatusEx
	ms.Length = uint32(unsafe.Sizeof(ms))
	if r, _, _ := pGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&ms))); r != 0 {
		rows = append(rows, [2]string{tr("Mémoire", "Memory"), gigabytes(ms.TotalPhys)})
	}
	const gpuKey = `SYSTEM\CurrentControlSet\Control\Class\{4d36e968-e325-11ce-bfc1-08002be10318}\`
	for _, n := range []string{"0000", "0001", "0002", "0003"} {
		if d := regLMString(gpuKey+n, "DriverDesc"); d != "" && !strings.Contains(d, "Basic") {
			rows = append(rows, [2]string{tr("Graphiques", "Graphics"), d})
			break
		}
	}
	var avail, total, free uint64
	if r, _, _ := pGetDiskFreeSpaceEx.Call(ptr(utf16(`C:\`)), uintptr(unsafe.Pointer(&avail)), uintptr(unsafe.Pointer(&total)), uintptr(unsafe.Pointer(&free))); r != 0 && total > 0 {
		rows = append(rows, [2]string{tr("Disque", "Storage"), fmt.Sprintf(tr("%s disponibles sur %s", "%s available of %s"), gigabytes(avail), gigabytes(total))})
	}
	if up, _, _ := pGetTickCount64.Call(); up > 0 {
		s := uint64(up) / 1000
		d, h, m := s/86400, s/3600%24, s/60%60
		txt := fmt.Sprintf(tr("%d h %d min", "%d hr %d min"), h, m)
		if d > 0 {
			txt = fmt.Sprintf(tr("%d j %d h", "%d d %d hr"), d, h)
		}
		rows = append(rows, [2]string{tr("Allumé depuis", "Up for"), txt})
	}
	if u := os.Getenv("USERNAME"); u != "" {
		rows = append(rows, [2]string{tr("Utilisateur", "User"), u})
	}
	about.rows = rows
}

func aboutDraw(hdc uintptr, w, h int32) {
	full := rect{0, 0, w, h}
	pFillRect.Call(hdc, uintptr(unsafe.Pointer(&full)), brushMenuBg)
	pSetBkMode.Call(hdc, transparent)
	pad := sc(24)
	y := sc(24)
	sq, gap := sc(16), sc(4) // logo : quatre carrés, comme le symbole de la barre
	x0 := (w - 2*sq - gap) / 2
	for i := int32(0); i < 2; i++ {
		for j := int32(0); j < 2; j++ {
			fillRR(hdc, rect{x0 + j*(sq+gap), y + i*(sq+gap), x0 + j*(sq+gap) + sq, y + i*(sq+gap) + sq}, sc(4), pal.accent)
		}
	}
	y += sc(48)
	pSelectObject.Call(hdc, panelFont.big)
	pSetTextColor.Call(hdc, pal.menuText)
	drawText(hdc, about.title, rect{pad, y, w - pad, y + sc(34)}, dtCenter|dtEllipsis)
	y += sc(34)
	pSelectObject.Call(hdc, panelFont.small)
	pSetTextColor.Call(hdc, pal.menuKey)
	drawText(hdc, about.sub, rect{pad, y, w - pad, y + sc(20)}, dtCenter|dtEllipsis)
	y += sc(36)
	keyW := sc(110)
	for _, r := range about.rows {
		pSelectObject.Call(hdc, fontBold)
		pSetTextColor.Call(hdc, pal.menuKey)
		drawText(hdc, r[0], rect{pad, y, pad + keyW, y + sc(24)}, dtRight)
		pSelectObject.Call(hdc, fontReg)
		pSetTextColor.Call(hdc, pal.menuText)
		drawText(hdc, r[1], rect{pad + keyW + sc(12), y, w - pad, y + sc(24)}, dtLeft|dtEllipsis)
		y += sc(26)
	}
	bg := cardColor()
	if about.hoverBtn {
		bg = blendColor(pal.menuBg, pal.menuText, 0.16)
	}
	fillRR(hdc, about.btn, sc(12), bg)
	pSelectObject.Call(hdc, fontReg)
	pSetTextColor.Call(hdc, pal.menuText)
	drawText(hdc, tr("Plus d’infos…", "More Info…"), about.btn, dtCenter)
}

func aboutProc(hwnd, message, wparam, lparam uintptr) uintptr {
	if hwnd == about.hwnd && about.hwnd != 0 {
		switch uint32(message) {
		case wmEraseBkgnd:
			return 1
		case wmPaint:
			paintBuffered(hwnd, aboutDraw)
			return 0
		case wmActivate:
			if wparam&0xFFFF == waInactive {
				closeAbout(false)
			}
			return 0
		case wmMouseMove:
			x, y := lparamXY(lparam)
			if in := inRect(about.btn, x, y); in != about.hoverBtn {
				about.hoverBtn = in
				pInvalidateRect.Call(hwnd, 0, 0)
			}
			return 0
		case wmLButtonUp:
			x, y := lparamXY(lparam)
			if inRect(about.btn, x, y) {
				closeAbout(true)
				openURI("ms-settings:about")
			}
			return 0
		case wmKeyDown:
			if wparam == vkEscape {
				closeAbout(true)
			}
			return 0
		case wmClose:
			closeAbout(true)
			return 0
		}
	}
	r, _, _ := pDefWindowProc.Call(hwnd, message, wparam, lparam)
	return r
}

func showAbout() {
	if about.hwnd != 0 {
		closeAbout(true)
		return
	}
	initPanelFonts()
	hideLabel()
	if !aboutOK {
		hInst, _, _ := pGetModuleHandle.Call(0)
		cursor, _, _ := pLoadCursor.Call(0, idcArrow)
		wc := wndClassEx{WndProc: syscall.NewCallback(aboutProc), Instance: hInst, Cursor: cursor, ClassName: utf16(aboutClass)}
		wc.Size = uint32(unsafe.Sizeof(wc))
		pRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
		aboutOK = true
	}
	aboutCollect()
	about.w = sc(440)
	about.h = sc(24) + sc(48) + sc(34) + sc(36) + int32(len(about.rows))*sc(26) + sc(18) + sc(36) + sc(24)
	about.btn = rect{(about.w - sc(150)) / 2, about.h - sc(24) - sc(36), (about.w + sc(150)) / 2, about.h - sc(24)}
	about.hoverBtn = false
	cur := cursorPt()
	m := monitorRect(cur.X, cur.Y)
	x := m.Left + (m.Right-m.Left-about.w)/2
	y := m.Top + (m.Bottom-m.Top-about.h)/3
	about.prev, _, _ = pGetForeground.Call()
	hInst, _, _ := pGetModuleHandle.Call(0)
	hwnd, _, _ := pCreateWindowEx.Call(wsExTopmost|wsExToolwin, ptr(utf16(aboutClass)), ptr(utf16("About")), wsPopup,
		uintptr(x), uintptr(y), uintptr(about.w), uintptr(about.h), 0, 0, hInst, 0)
	if hwnd == 0 {
		return
	}
	about.hwnd = hwnd
	styleWindow(hwnd, about.w, about.h)
	pShowWindow.Call(hwnd, swShow)
	pSetForeground.Call(hwnd)
	pSetFocus.Call(hwnd)
}

func closeAbout(restore bool) {
	h := about.hwnd
	if h == 0 {
		return
	}
	about.hwnd = 0
	pDestroyWindow.Call(h)
	if restore && about.prev != 0 && about.prev != barHwnd {
		if ok, _, _ := pIsWindow.Call(about.prev); ok != 0 {
			pSetForeground.Call(about.prev)
		}
	}
}
