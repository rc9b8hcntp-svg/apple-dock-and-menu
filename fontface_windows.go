//go:build windows

package main

// Police de l'interface : la plus proche de macOS parmi celles qui sont installées.
// SF Pro (la police d'Apple, à installer soi-même) si elle est présente, sinon
// Segoe UI Variable (Windows 11), sinon Segoe UI.

import (
	"strings"
	"syscall"
	"unsafe"
)

var (
	pGetTextFace = gdi32.NewProc("GetTextFaceW")
	uiFaceName   string
)

func faceInstalled(name string) bool {
	f, _, _ := pCreateFont.Call(uintptr(^uint32(12)), 0, 0, 0, 400, 0, 0, 0,
		1 /*DEFAULT_CHARSET*/, 0, 0, 5 /*CLEARTYPE*/, 0, ptr(utf16(name))) // hauteur -13
	if f == 0 {
		return false
	}
	defer pDeleteObject.Call(f)
	dc, _, _ := pCreateCompatDC.Call(0)
	if dc == 0 {
		return false
	}
	defer pDeleteDC.Call(dc)
	old, _, _ := pSelectObject.Call(dc, f)
	defer pSelectObject.Call(dc, old)
	var buf [64]uint16
	n, _, _ := pGetTextFace.Call(dc, 64, uintptr(unsafe.Pointer(&buf[0])))
	if n == 0 {
		return false
	}
	return strings.EqualFold(syscall.UTF16ToString(buf[:]), name)
}

// uiFace renvoie le nom de la police à utiliser (cherchée une seule fois).
func uiFace() string {
	if uiFaceName != "" {
		return uiFaceName
	}
	uiFaceName = "Segoe UI"
	for _, name := range []string{"SF Pro Text", "SF Pro Display", "SF Pro", "Segoe UI Variable Text"} {
		if faceInstalled(name) {
			uiFaceName = name
			break
		}
	}
	return uiFaceName
}
