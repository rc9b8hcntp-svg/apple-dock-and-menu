//go:build windows

package main

// Icônes d'état à droite de la barre, comme sur macOS : batterie, Wi-Fi, son,
// recherche, centre de contrôle. Leur état est lu régulièrement ; un clic ouvre
// le panneau Windows qui correspond.

import (
	"encoding/binary"
	"strconv"
	"syscall"
	"unsafe"
)

const (
	wifiNone = iota // aucune carte Wi-Fi : l'icône est masquée
	wifiOff         // carte présente mais non connectée
	wifiOn
)

type systemPowerStatus struct {
	ACLineStatus        byte
	BatteryFlag         byte
	BatteryLifePercent  byte
	SystemStatusFlag    byte
	BatteryLifeTime     uint32
	BatteryFullLifeTime uint32
}

var (
	iphlpapi              = syscall.NewLazyDLL("iphlpapi.dll")
	pGetAdaptersAddresses = iphlpapi.NewProc("GetAdaptersAddresses")
	pGetSystemPowerStatus = kernel32.NewProc("GetSystemPowerStatus")
	pKeybdEvent           = user32.NewProc("keybd_event")
	pIsZoomed             = user32.NewProc("IsZoomed")

	fontIcon     uintptr // Segoe MDL2 Assets : les glyphes des icônes
	battPct      = -1    // -1 : pas de batterie
	battCharging bool
	wifiState    = wifiOn
	statusTick   int
)

func makeIconFont() uintptr {
	f, _, _ := pCreateFont.Call(uintptr(-sc(14)), 0, 0, 0, 400, 0, 0, 0,
		1 /*DEFAULT_CHARSET*/, 0, 0, 5 /*CLEARTYPE*/, 0, ptr(utf16("Segoe MDL2 Assets")))
	return f
}

// updateStatus relit la batterie (à chaque appel) et le Wi-Fi (toutes les 5 s).
// Renvoie true si l'affichage doit changer.
func updateStatus() bool {
	changed := false
	pct, charging := -1, false
	var sp systemPowerStatus
	if r, _, _ := pGetSystemPowerStatus.Call(uintptr(unsafe.Pointer(&sp))); r != 0 &&
		sp.BatteryFlag != 128 && sp.BatteryFlag != 255 && sp.BatteryLifePercent <= 100 {
		pct, charging = int(sp.BatteryLifePercent), sp.BatteryFlag&8 != 0
	}
	if pct != battPct || charging != battCharging {
		battPct, battCharging, changed = pct, charging, true
	}
	if statusTick%10 == 0 {
		if w := queryWifi(); w != wifiState {
			wifiState, changed = w, true
		}
	}
	statusTick++
	return changed
}

// queryWifi regarde les cartes réseau : y en a-t-il une de type Wi-Fi (IF_TYPE_IEEE80211),
// et est-elle connectée ?
func queryWifi() int {
	if pGetAdaptersAddresses.Find() != nil {
		return wifiOn
	}
	size := uint32(16 * 1024)
	var buf []byte
	for try := 0; try < 3; try++ {
		buf = make([]byte, size)
		// AF_UNSPEC ; on saute les adresses, ce qui ne laisse que l'état des cartes.
		r, _, _ := pGetAdaptersAddresses.Call(0, 0xF, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
		if r == 0 {
			break
		}
		if r != 111 { // ERROR_BUFFER_OVERFLOW : on réessaie avec la taille demandée
			return wifiOn
		}
		buf = nil
	}
	if buf == nil {
		return wifiOn
	}
	ps := int(unsafe.Sizeof(uintptr(0)))
	// IP_ADAPTER_ADDRESSES : Length, IfIndex, Next, 8 pointeurs, adresse physique (8 octets),
	// longueur, Flags, Mtu, puis IfType et OperStatus.
	ifOff := 8 + 9*ps + 8 + 12
	opOff := ifOff + 4
	base := uintptr(unsafe.Pointer(&buf[0]))
	state := wifiNone
	off := 0
	for off >= 0 && off+opOff+4 <= len(buf) {
		ifType := binary.LittleEndian.Uint32(buf[off+ifOff:])
		oper := binary.LittleEndian.Uint32(buf[off+opOff:])
		if ifType == 71 { // IF_TYPE_IEEE80211
			if oper == 1 { // IfOperStatusUp
				return wifiOn
			}
			state = wifiOff
		}
		var next uint64
		if ps == 8 {
			next = binary.LittleEndian.Uint64(buf[off+8:])
		} else {
			next = uint64(binary.LittleEndian.Uint32(buf[off+8:]))
		}
		if next == 0 {
			break
		}
		off = int(uintptr(next) - base)
	}
	return state
}

// ---------------------------------------------------------------------------
// Actions
// ---------------------------------------------------------------------------

// sendWinKey simule Win+<touche> : ouvre la recherche, le centre de notifications…
func sendWinKey(vk byte) {
	const vkLWin, keyUp = 0x5B, 2
	pKeybdEvent.Call(vkLWin, 0, 0, 0)
	pKeybdEvent.Call(uintptr(vk), 0, 0, 0)
	pKeybdEvent.Call(uintptr(vk), 0, keyUp, 0)
	pKeybdEvent.Call(vkLWin, 0, keyUp, 0)
}

func barAction(kind int) {
	switch kind {
	case hitBattery:
		openURI("ms-settings:batterysaver")
	case hitWifi:
		openURI("ms-settings:network-wifi")
	case hitSound:
		openURI("ms-settings:sound")
	case hitSearch:
		sendWinKey('S')
	case hitControl:
		sendWinKey('A')
	case hitClock:
		sendWinKey('N')
	}
}

// ---------------------------------------------------------------------------
// Dessin des icônes
// ---------------------------------------------------------------------------

// blendColor mélange deux COLORREF : k = 0 donne a, k = 1 donne b.
func blendColor(a, b uintptr, k float64) uintptr {
	mix := func(sh uint) uintptr {
		x, y := float64((a>>sh)&0xFF), float64((b>>sh)&0xFF)
		return uintptr(x+(y-x)*k+0.5) << sh
	}
	return mix(0) | mix(8) | mix(16)
}

// drawIconGlyph écrit un glyphe de la police d'icônes centré dans r.
func drawIconGlyph(hdc uintptr, glyph string, r rect, col uintptr) {
	pSelectObject.Call(hdc, fontIcon)
	pSetTextColor.Call(hdc, col)
	drawText(hdc, glyph, r, dtCenter)
	pSelectObject.Call(hdc, fontReg)
	pSetTextColor.Call(hdc, pal.barText)
}

// drawControlIcon dessine les deux interrupteurs du centre de contrôle.
func drawControlIcon(hdc uintptr, r rect) {
	pw, ph := sc(14), sc(6)
	cx, cy := (r.Left+r.Right)/2, (r.Top+r.Bottom)/2
	x0 := cx - pw/2
	top := rect{x0, cy - ph - sc(1), x0 + pw, cy - sc(1)}
	bot := rect{x0, cy + sc(1), x0 + pw, cy + sc(1) + ph}
	fillRR(hdc, top, ph, pal.barText)
	fillRR(hdc, bot, ph, pal.barText)
	k := ph - 2*sc(1)
	fillEllipse(hdc, rect{top.Right - sc(1) - k, top.Top + sc(1), top.Right - sc(1), top.Top + sc(1) + k}, pal.bar)
	fillEllipse(hdc, rect{bot.Left + sc(1), bot.Top + sc(1), bot.Left + sc(1) + k, bot.Top + sc(1) + k}, pal.bar)
}

func batteryText() string { return strconv.Itoa(battPct) + "%" }

// batteryItemWidth : largeur de l'élément (pourcentage compris).
func batteryItemWidth(hdc uintptr) int32 {
	w := sc(8) + sc(22) + sc(2) + sc(8)
	if cfg.ShowBatteryPct {
		pSelectObject.Call(hdc, fontReg)
		w += textWidth(hdc, batteryText()) + sc(5)
	}
	return w
}

func drawBattery(hdc uintptr, r rect) {
	bodyW, bodyH, nub := sc(22), sc(11), sc(2)
	cy := (r.Top + r.Bottom) / 2
	right := r.Right - sc(8)
	bx := right - nub
	body := rect{bx - bodyW, cy - bodyH/2, bx, cy - bodyH/2 + bodyH}
	outline := blendColor(pal.bar, pal.barText, 0.5)
	fillRR(hdc, body, sc(5), outline)
	fillRR(hdc, shrink(body, sc(1)), sc(4), pal.bar)
	in := shrink(body, sc(2))
	lvl := (in.Right - in.Left) * int32(battPct) / 100
	if lvl < sc(2) {
		lvl = sc(2)
	}
	col := pal.barText
	switch {
	case battCharging:
		col = rgb(48, 209, 88)
	case battPct <= 20:
		col = rgb(255, 69, 58)
	}
	fillRR(hdc, rect{in.Left, in.Top, in.Left + lvl, in.Bottom}, sc(3), col)
	fillRR(hdc, rect{bx, cy - sc(2), bx + nub, cy + sc(2)}, nub, outline)
	if cfg.ShowBatteryPct {
		pSelectObject.Call(hdc, fontReg)
		pSetTextColor.Call(hdc, pal.barText)
		drawText(hdc, batteryText(), rect{r.Left + sc(8), r.Top, body.Left - sc(5), r.Bottom}, dtRight)
	}
}
