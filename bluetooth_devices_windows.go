//go:build windows

package main

// Appareils Bluetooth avec l'API native de Windows (bthprops.cpl) : liste des appareils
// appairés avec leur état réel (connecté ou non), et connexion / déconnexion. Windows
// n'a pas d'appel « connecter » direct : on active (ou désactive) les services de
// l'appareil (audio A2DP, mains libres, casque, HID), ce qui fait établir (ou couper)
// la liaison, comme le font les outils de connexion Bluetooth.

import (
	"sort"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var (
	bthprops                  = syscall.NewLazyDLL("bthprops.cpl")
	pBluetoothFindFirstRadio  = bthprops.NewProc("BluetoothFindFirstRadio")
	pBluetoothFindRadioClose  = bthprops.NewProc("BluetoothFindRadioClose")
	pBluetoothFindFirstDevice = bthprops.NewProc("BluetoothFindFirstDevice")
	pBluetoothFindNextDevice  = bthprops.NewProc("BluetoothFindNextDevice")
	pBluetoothFindDeviceClose = bthprops.NewProc("BluetoothFindDeviceClose")
	pBluetoothSetServiceState = bthprops.NewProc("BluetoothSetServiceState")
	btPending                 = map[string]bool{} // appareils en cours de connexion / déconnexion
	btPendingMu               sync.Mutex
	btServiceGUIDs            = []syscall.GUID{
		btGUID(0x110B), // audio (A2DP, récepteur)
		btGUID(0x111E), // mains libres
		btGUID(0x1108), // casque
		btGUID(0x1124), // souris, clavier, manettes (HID)
	}
)

func btGUID(short uint32) syscall.GUID {
	return syscall.GUID{Data1: short, Data2: 0x0000, Data3: 0x1000,
		Data4: [8]byte{0x80, 0x00, 0x00, 0x80, 0x5F, 0x9B, 0x34, 0xFB}}
}

// btDeviceInfo : BLUETOOTH_DEVICE_INFO (560 octets).
type btDeviceInfo struct {
	Size          uint32
	_             uint32
	Address       uint64
	Class         uint32
	Connected     uint32
	Remembered    uint32
	Authenticated uint32
	LastSeen      [16]byte
	LastUsed      [16]byte
	Name          [248]uint16
}

// btSearchParams : BLUETOOTH_DEVICE_SEARCH_PARAMS.
type btSearchParams struct {
	Size          uint32
	Authenticated uint32
	Remembered    uint32
	Unknown       uint32
	Connected     uint32
	IssueInquiry  uint32
	Timeout       uint8
	Radio         uintptr
}

func btOpenRadio() (uintptr, bool) {
	if pBluetoothFindFirstRadio.Find() != nil {
		return 0, false
	}
	params := struct{ Size uint32 }{4}
	var radio uintptr
	find, _, _ := pBluetoothFindFirstRadio.Call(uintptr(unsafe.Pointer(&params)), uintptr(unsafe.Pointer(&radio)))
	if find == 0 || radio == 0 {
		return 0, false
	}
	pBluetoothFindRadioClose.Call(find)
	return radio, true
}

func btCloseRadio(h uintptr) { pCloseHandle.Call(h) }

// btEnumerate liste les appareils appairés (connectés en premier).
func btEnumerate(radio uintptr) []btDeviceInfo {
	if pBluetoothFindFirstDevice.Find() != nil {
		return nil
	}
	sp := btSearchParams{Authenticated: 1, Remembered: 1, Connected: 1, Radio: radio}
	sp.Size = uint32(unsafe.Sizeof(sp))
	info := new(btDeviceInfo)
	info.Size = uint32(unsafe.Sizeof(*info))
	var out []btDeviceInfo
	h, _, _ := pBluetoothFindFirstDevice.Call(uintptr(unsafe.Pointer(&sp)), uintptr(unsafe.Pointer(info)))
	if h == 0 {
		return nil
	}
	defer pBluetoothFindDeviceClose.Call(h)
	for n := 0; n < 64; n++ {
		out = append(out, *info)
		info.Size = uint32(unsafe.Sizeof(*info))
		if ok, _, _ := pBluetoothFindNextDevice.Call(h, uintptr(unsafe.Pointer(info))); ok == 0 {
			break
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Connected != 0 && out[j].Connected == 0 })
	return out
}

// btNativeDevices renvoie la présence d'une radio et la liste des appareils.
func btNativeDevices() (present bool, devs []btDev) {
	radio, ok := btOpenRadio()
	if !ok {
		return false, nil
	}
	defer btCloseRadio(radio)
	for _, d := range btEnumerate(radio) {
		name := syscall.UTF16ToString(d.Name[:])
		if name == "" {
			continue
		}
		devs = append(devs, btDev{name: name, connected: d.Connected != 0, addr: d.Address})
	}
	return true, devs
}

func btIsPending(name string) bool {
	btPendingMu.Lock()
	defer btPendingMu.Unlock()
	return btPending[name]
}

func btSetPending(name string, v bool) {
	btPendingMu.Lock()
	if v {
		btPending[name] = true
	} else {
		delete(btPending, name)
	}
	btPendingMu.Unlock()
}

// btConnect connecte ou déconnecte l'appareil (en arrière-plan) puis attend le résultat.
func btConnect(d btDev, connect bool) {
	if btIsPending(d.name) {
		return
	}
	btSetPending(d.name, true)
	invalidate(barHwnd)
	go func() {
		defer func() {
			btSetPending(d.name, false)
			btRefresh()
		}()
		radio, ok := btOpenRadio()
		if !ok {
			return
		}
		defer btCloseRadio(radio)
		var target *btDeviceInfo
		for _, e := range btEnumerate(radio) {
			if e.Address == d.addr {
				e := e
				target = &e
				break
			}
		}
		if target == nil {
			return
		}
		target.Size = uint32(unsafe.Sizeof(*target))
		set := func(g syscall.GUID, enable bool) {
			flag := uintptr(0)
			if enable {
				flag = 1
			}
			pBluetoothSetServiceState.Call(radio, uintptr(unsafe.Pointer(target)), uintptr(unsafe.Pointer(&g)), flag)
		}
		for _, g := range btServiceGUIDs {
			set(g, false)
			if connect {
				time.Sleep(200 * time.Millisecond)
				set(g, true)
			}
		}
		// Attendre que l'état change (jusqu'à 12 s).
		for i := 0; i < 24; i++ {
			time.Sleep(500 * time.Millisecond)
			for _, e := range btEnumerate(radio) {
				if e.Address == d.addr && (e.Connected != 0) == connect {
					return
				}
			}
		}
	}()
}
