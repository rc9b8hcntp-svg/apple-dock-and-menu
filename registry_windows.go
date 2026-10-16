//go:build windows

package main

import (
	"encoding/binary"
	"os"
	"syscall"
	"unsafe"
)

// Accès minimal au registre (HKEY_CURRENT_USER) : lire un nombre, écrire un texte
// ou des octets, supprimer une valeur.

var (
	advapi32        = syscall.NewLazyDLL("advapi32.dll")
	pRegGetValue    = advapi32.NewProc("RegGetValueW")
	pRegCreateKeyEx = advapi32.NewProc("RegCreateKeyExW")
	pRegOpenKeyEx   = advapi32.NewProc("RegOpenKeyExW")
	pRegSetValueEx  = advapi32.NewProc("RegSetValueExW")
	pRegDeleteValue = advapi32.NewProc("RegDeleteValueW")
	pRegCloseKey    = advapi32.NewProc("RegCloseKey")
)

const (
	hkeyCurrentUser = 0x80000001
	rrfRtDword      = 0x10
	rrfRtBinary     = 0x08
	keySetValue     = 0x0002
	regSZ           = 1
	regBinary       = 3
)

func regGet(subkey, name string, flags uintptr, buf []byte) (int, bool) {
	n := uint32(len(buf))
	r, _, _ := pRegGetValue.Call(hkeyCurrentUser, ptr(utf16(subkey)), ptr(utf16(name)), flags, 0,
		uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&n)))
	return int(n), r == 0
}

func regDword(subkey, name string) (uint32, bool) {
	var b [4]byte
	n, ok := regGet(subkey, name, rrfRtDword, b[:])
	if !ok || n != 4 {
		return 0, false
	}
	return binary.LittleEndian.Uint32(b[:]), true
}

func regSet(subkey, name string, typ uintptr, data []byte) bool {
	var hk uintptr
	if r, _, _ := pRegCreateKeyEx.Call(hkeyCurrentUser, ptr(utf16(subkey)), 0, 0, 0, keySetValue, 0,
		uintptr(unsafe.Pointer(&hk)), 0); r != 0 {
		return false
	}
	defer pRegCloseKey.Call(hk)
	r, _, _ := pRegSetValueEx.Call(hk, ptr(utf16(name)), 0, typ, uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)))
	return r == 0
}

func regSetString(subkey, name, value string) bool {
	u, _ := syscall.UTF16FromString(value)
	b := make([]byte, len(u)*2)
	for i, c := range u {
		b[2*i], b[2*i+1] = byte(c), byte(c>>8)
	}
	return regSet(subkey, name, regSZ, b)
}

func regDelete(subkey, name string) {
	var hk uintptr
	if r, _, _ := pRegOpenKeyEx.Call(hkeyCurrentUser, ptr(utf16(subkey)), 0, keySetValue, uintptr(unsafe.Pointer(&hk))); r != 0 {
		return
	}
	defer pRegCloseKey.Call(hk)
	pRegDeleteValue.Call(hk, ptr(utf16(name)))
}

// ---------------------------------------------------------------------------
// Lancement à l'ouverture de session
// ---------------------------------------------------------------------------

const (
	runKey      = `Software\Microsoft\Windows\CurrentVersion\Run`
	approvedKey = `Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run`
	runName     = "MenuWindows"
)

// autostartEnabled : le Gestionnaire des tâches (onglet Démarrage) et ce réglage
// partagent la valeur StartupApproved : un premier octet impair signifie « désactivé ».
func autostartEnabled() bool {
	var b [12]byte
	if n, ok := regGet(approvedKey, runName, rrfRtBinary, b[:]); ok && n > 0 {
		return b[0]&1 == 0
	}
	return true
}

func setAutostart(on bool) {
	var flag [12]byte
	if on {
		if exe, err := os.Executable(); err == nil {
			regSetString(runKey, runName, `"`+exe+`"`)
		}
		flag[0] = 2
	} else {
		regDelete(runKey, runName)
		flag[0] = 3
	}
	regSet(approvedKey, runName, regBinary, flag[:])
}
