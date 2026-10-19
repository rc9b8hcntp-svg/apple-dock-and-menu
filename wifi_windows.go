//go:build windows

package main

// Wi-Fi : liste des réseaux (netsh), connexion à un réseau connu, et activation
// ou coupure de la radio (API WLAN de Windows).

import (
	"context"
	"encoding/binary"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var (
	wlanapi             = syscall.NewLazyDLL("wlanapi.dll")
	pWlanOpenHandle     = wlanapi.NewProc("WlanOpenHandle")
	pWlanCloseHandle    = wlanapi.NewProc("WlanCloseHandle")
	pWlanEnumInterfaces = wlanapi.NewProc("WlanEnumInterfaces")
	pWlanQueryInterface = wlanapi.NewProc("WlanQueryInterface")
	pWlanSetInterface   = wlanapi.NewProc("WlanSetInterface")
	pWlanFreeMemory     = wlanapi.NewProc("WlanFreeMemory")
	pMultiByteToWide    = kernel32.NewProc("MultiByteToWideChar")

	// Mémoire à adresse fixe pour les appels WLAN.
	wlanVer    uint32
	wlanHandle uintptr
	wlanList   uintptr
	wlanData   uintptr
	wlanSize   uint32
	wlanType   uint32
	wlanGUID   [16]byte
	wlanRadio  [3]uint32
)

// at fabrique un pointeur à partir d'une adresse rendue par Windows.
func at(addr uintptr) unsafe.Pointer { return *(*unsafe.Pointer)(unsafe.Pointer(&addr)) }

func readMem(addr uintptr, n int) []byte {
	out := make([]byte, n)
	copy(out, unsafe.Slice((*byte)(at(addr)), n))
	return out
}

// forWlanInterfaces appelle f pour chaque carte Wi-Fi (son GUID est dans wlanGUID).
func forWlanInterfaces(f func(h uintptr)) bool {
	if pWlanOpenHandle.Find() != nil {
		return false
	}
	wlanHandle, wlanVer = 0, 0
	if r, _, _ := pWlanOpenHandle.Call(2, 0, uintptr(unsafe.Pointer(&wlanVer)), uintptr(unsafe.Pointer(&wlanHandle))); r != 0 || wlanHandle == 0 {
		return false
	}
	h := wlanHandle
	defer pWlanCloseHandle.Call(h, 0)
	wlanList = 0
	if r, _, _ := pWlanEnumInterfaces.Call(h, 0, uintptr(unsafe.Pointer(&wlanList))); r != 0 || wlanList == 0 {
		return false
	}
	list := wlanList
	defer pWlanFreeMemory.Call(list)
	n := int(binary.LittleEndian.Uint32(readMem(list, 4)))
	const itemSize = 16 + 512 + 4 // GUID, description (256 WCHAR), état
	if n <= 0 || n > 16 {
		return false
	}
	buf := readMem(list+8, n*itemSize)
	for i := 0; i < n; i++ {
		copy(wlanGUID[:], buf[i*itemSize:i*itemSize+16])
		f(h)
	}
	return true
}

// wifiRadioOn dit si la radio Wi-Fi est allumée (ok = false : impossible à savoir).
func wifiRadioOn() (on, ok bool) {
	on = false
	found := forWlanInterfaces(func(h uintptr) {
		wlanData, wlanSize, wlanType = 0, 0, 0
		// wlan_intf_opcode_radio_state = 4
		if r, _, _ := pWlanQueryInterface.Call(h, uintptr(unsafe.Pointer(&wlanGUID[0])), 4, 0,
			uintptr(unsafe.Pointer(&wlanSize)), uintptr(unsafe.Pointer(&wlanData)), uintptr(unsafe.Pointer(&wlanType))); r != 0 || wlanData == 0 {
			return
		}
		data := wlanData
		defer pWlanFreeMemory.Call(data)
		if wlanSize < 16 {
			return
		}
		b := readMem(data, 16) // nombre de radios, puis (index, état logiciel, état matériel)
		if binary.LittleEndian.Uint32(b[0:]) >= 1 && binary.LittleEndian.Uint32(b[8:]) == 1 {
			on = true
		}
		ok = true
	})
	return on, found && ok
}

// setWifiRadio allume ou coupe la radio Wi-Fi. Renvoie false si Windows refuse.
func setWifiRadio(on bool) bool {
	done := false
	forWlanInterfaces(func(h uintptr) {
		state := uint32(2) // dot11_radio_state_off
		if on {
			state = 1
		}
		wlanRadio = [3]uint32{0, state, 0}
		if r, _, _ := pWlanSetInterface.Call(h, uintptr(unsafe.Pointer(&wlanGUID[0])), 4, 12,
			uintptr(unsafe.Pointer(&wlanRadio[0])), 0); r == 0 {
			done = true
		}
	})
	return done
}

// ---------------------------------------------------------------------------
// Réseaux (netsh)
// ---------------------------------------------------------------------------

func oemToString(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	n, _, _ := pMultiByteToWide.Call(1 /*CP_OEMCP*/, 0, uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)), 0, 0)
	if n == 0 {
		return string(b)
	}
	w := make([]uint16, n)
	pMultiByteToWide.Call(1, 0, uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)), uintptr(unsafe.Pointer(&w[0])), n)
	return syscall.UTF16ToString(w)
}

func netshWlan(args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	c := exec.CommandContext(ctx, "netsh", append([]string{"wlan"}, args...)...)
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWin}
	out, _ := c.Output()
	return oemToString(out)
}

func afterColon(line string) string {
	if i := strings.IndexByte(line, ':'); i >= 0 {
		return strings.TrimSpace(line[i+1:])
	}
	return ""
}

type wifiNet struct {
	ssid  string
	known bool // un profil enregistré existe : la connexion est possible
	cur   bool
}

// wifiNetworks liste les réseaux visibles (et à défaut les réseaux connus).
func wifiNetworks() []wifiNet {
	var nets, profs, ifs string
	var wg sync.WaitGroup
	for _, job := range []struct {
		out  *string
		args []string
	}{{&nets, []string{"show", "networks"}}, {&profs, []string{"show", "profiles"}}, {&ifs, []string{"show", "interfaces"}}} {
		wg.Add(1)
		go func(out *string, args []string) {
			defer wg.Done()
			*out = netshWlan(args...)
		}(job.out, job.args)
	}
	wg.Wait()

	known := map[string]bool{}
	var knownList []string
	for _, ln := range strings.Split(profs, "\n") {
		ln = strings.TrimRight(ln, "\r")
		if strings.HasPrefix(ln, " ") || strings.HasPrefix(ln, "\t") {
			if v := afterColon(ln); v != "" {
				known[v] = true
				knownList = append(knownList, v)
			}
		}
	}
	cur := ""
	for _, ln := range strings.Split(ifs, "\n") {
		t := strings.TrimSpace(ln)
		if strings.HasPrefix(t, "SSID") && strings.HasPrefix(strings.TrimSpace(t[4:]), ":") {
			if v := afterColon(t); v != "" {
				cur = v
			}
		}
	}
	var out []wifiNet
	seen := map[string]bool{}
	add := func(s string) {
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		out = append(out, wifiNet{ssid: s, known: known[s], cur: s == cur})
	}
	add(cur)
	for _, ln := range strings.Split(nets, "\n") {
		t := strings.TrimSpace(ln)
		if len(t) > 5 && strings.HasPrefix(t, "SSID ") && t[5] >= '0' && t[5] <= '9' {
			add(afterColon(t))
		}
	}
	if len(out) <= 1 { // pas de liste (autorisation de localisation, par exemple) : réseaux connus
		for _, k := range knownList {
			add(k)
		}
	}
	if len(out) > 12 {
		out = out[:12]
	}
	return out
}
