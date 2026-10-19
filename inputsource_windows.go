//go:build windows

package main

// Source de saisie (disposition du clavier) dans la barre, comme sur macOS : le code de la
// langue du clavier actuel (« FR », « EN »…) s'affiche s'il y en a plusieurs d'installées ;
// le menu liste les dispositions et en active une pour l'application active.

import (
	"strings"
	"syscall"
	"unsafe"
)

var (
	pGetKeyboardLayout     = user32.NewProc("GetKeyboardLayout")
	pGetKeyboardLayoutList = user32.NewProc("GetKeyboardLayoutList")
	pGetLocaleInfo         = kernel32.NewProc("GetLocaleInfoW")

	inputLabel string
)

func localeText(lang uint32, kind uintptr) string {
	var b [96]uint16
	n, _, _ := pGetLocaleInfo.Call(uintptr(lang), kind, uintptr(unsafe.Pointer(&b[0])), 96)
	if n == 0 {
		return ""
	}
	return syscall.UTF16ToString(b[:])
}

func keyboardLayouts() []uintptr {
	n, _, _ := pGetKeyboardLayoutList.Call(0, 0)
	if n <= 0 || n > 64 {
		return nil
	}
	buf := make([]uintptr, n)
	got, _, _ := pGetKeyboardLayoutList.Call(n, uintptr(unsafe.Pointer(&buf[0])))
	return buf[:got]
}

func currentLayout() uintptr {
	var tid uintptr
	if active != 0 {
		tid, _, _ = pGetWindowThreadPI.Call(active, 0)
	}
	l, _, _ := pGetKeyboardLayout.Call(tid)
	return l
}

func layoutCode(hkl uintptr) string {
	return strings.ToUpper(localeText(uint32(hkl&0xFFFF), 0x59 /*LOCALE_SISO639LANGNAME*/))
}

// updateInput relit la disposition active ; renvoie true si l'affichage doit changer.
func updateInput() bool {
	label := ""
	if cfg.ShowInput && len(keyboardLayouts()) > 1 {
		label = layoutCode(currentLayout())
	}
	if label != inputLabel {
		inputLabel = label
		return true
	}
	return false
}

func showInputMenu(hwnd uintptr, x, y int32) {
	cur := currentLayout()
	var items []mItem
	for _, l := range keyboardLayouts() {
		l := l
		name := localeText(uint32(l&0xFFFF), 0x4 /*LOCALE_SNATIVELANGNAME*/)
		if name == "" {
			name = layoutCode(l)
		}
		name = strings.ToUpper(name[:1]) + name[1:]
		items = append(items, mItem{text: name, checkable: true, check: l == cur, act: func() {
			target := active
			if target == 0 {
				target, _, _ = pGetForeground.Call()
			}
			pPostMessage.Call(target, 0x50 /*WM_INPUTLANGCHANGEREQUEST*/, 0, l)
		}})
	}
	items = append(items, msep(),
		cfgCheck(tr("Afficher le clavier dans la barre", "Show Input Menu in Menu Bar"), &cfg.ShowInput),
		mact(tr("Réglages du clavier…", "Keyboard Settings…"), openSettingsURI("ms-settings:regionlanguage")),
	)
	trackMenu(hwnd, items, x, y)
}
