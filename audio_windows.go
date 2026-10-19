//go:build windows

package main

// Volume du son : l'interface COM IAudioEndpointVolume de Windows, appelée à la
// main par sa table de fonctions. Les nombres à virgule ne passent pas par
// syscall : le volume se règle donc en « crans » (VolumeStepUp / VolumeStepDown),
// et se lit avec GetVolumeStepInfo.

import (
	"syscall"
	"unsafe"
)

var (
	ole32             = syscall.NewLazyDLL("ole32.dll")
	pCoInitializeEx   = ole32.NewProc("CoInitializeEx")
	pCoCreateInstance = ole32.NewProc("CoCreateInstance")

	clsidMMDeviceEnumerator = guid{0xBCDE0395, 0xE52F, 0x467C, [8]byte{0x8E, 0x3D, 0xC4, 0x57, 0x92, 0x91, 0x69, 0x2E}}
	iidIMMDeviceEnumerator  = guid{0xA95664D2, 0x9614, 0x4F35, [8]byte{0xA7, 0x46, 0xDE, 0x8D, 0xB6, 0x36, 0x17, 0xE6}}
	iidIAudioEndpointVolume = guid{0x5CDF2C82, 0x841E, 0x4546, [8]byte{0x97, 0x22, 0x0C, 0xF7, 0x40, 0x78, 0x22, 0x9A}}

	// Variables globales : leur adresse ne change jamais, on peut la donner à Windows.
	comPtr        uintptr
	comU1, comU2  uint32
	currentAudio  *audioEP
	audioWarnOnce bool
)

// comCall appelle la méthode numéro idx de la table de fonctions de l'objet COM obj.
func comCall(obj uintptr, idx int, args ...uintptr) uintptr {
	if obj == 0 {
		return 0x80004003 // E_POINTER
	}
	p := *(*unsafe.Pointer)(unsafe.Pointer(&obj)) // l'objet
	vtbl := *(*unsafe.Pointer)(p)                 // sa table de fonctions
	fn := *(*uintptr)(unsafe.Add(vtbl, idx*int(unsafe.Sizeof(uintptr(0)))))
	all := make([]uintptr, 0, len(args)+1)
	all = append(all, obj)
	all = append(all, args...)
	r, _, _ := syscall.SyscallN(fn, all...)
	return r
}

func comFailed(hr uintptr) bool { return int32(hr) < 0 }

type audioEP struct{ ep uintptr }

// openAudio ouvre le volume du périphérique de lecture par défaut (nil si impossible).
func openAudio() *audioEP {
	if pCoCreateInstance.Find() != nil {
		return nil
	}
	pCoInitializeEx.Call(0, 2) // COINIT_APARTMENTTHREADED ; un autre mode déjà choisi convient aussi
	comPtr = 0
	hr, _, _ := pCoCreateInstance.Call(uintptr(unsafe.Pointer(&clsidMMDeviceEnumerator)), 0, 23, /*CLSCTX_ALL*/
		uintptr(unsafe.Pointer(&iidIMMDeviceEnumerator)), uintptr(unsafe.Pointer(&comPtr)))
	enum := comPtr
	if comFailed(hr) || enum == 0 {
		return nil
	}
	defer comCall(enum, 2) // Release

	comPtr = 0
	if comFailed(comCall(enum, 4, 0 /*eRender*/, 1 /*eMultimedia*/, uintptr(unsafe.Pointer(&comPtr)))) || comPtr == 0 {
		return nil
	}
	dev := comPtr
	defer comCall(dev, 2)

	comPtr = 0
	if comFailed(comCall(dev, 3, uintptr(unsafe.Pointer(&iidIAudioEndpointVolume)), 23, 0, uintptr(unsafe.Pointer(&comPtr)))) || comPtr == 0 {
		return nil
	}
	return &audioEP{ep: comPtr}
}

func (a *audioEP) close() {
	if a != nil && a.ep != 0 {
		comCall(a.ep, 2)
		a.ep = 0
	}
}

func (a *audioEP) steps() (cur, n int, ok bool) {
	comU1, comU2 = 0, 0
	if comFailed(comCall(a.ep, 16, uintptr(unsafe.Pointer(&comU1)), uintptr(unsafe.Pointer(&comU2)))) {
		return 0, 0, false
	}
	return int(comU1), int(comU2), true
}

// percent donne le volume de 0 à 100.
func (a *audioEP) percent() int {
	cur, n, ok := a.steps()
	if !ok || n < 2 {
		return 0
	}
	return (cur*100 + (n-1)/2) / (n - 1)
}

// setPercent règle le volume en montant ou descendant d'autant de crans qu'il faut.
func (a *audioEP) setPercent(p int) {
	cur, n, ok := a.steps()
	if !ok || n < 2 {
		return
	}
	target := (p*(n-1) + 50) / 100
	for i := 0; i < n+2 && cur != target; i++ {
		if cur < target {
			comCall(a.ep, 17, 0) // VolumeStepUp
		} else {
			comCall(a.ep, 18, 0) // VolumeStepDown
		}
		c, _, ok := a.steps()
		if !ok || c == cur {
			break
		}
		cur = c
	}
}

func (a *audioEP) muted() bool {
	comU1 = 0
	if comFailed(comCall(a.ep, 15, uintptr(unsafe.Pointer(&comU1)))) {
		return false
	}
	return comU1 != 0
}

func (a *audioEP) setMuted(m bool) {
	v := uintptr(0)
	if m {
		v = 1
	}
	comCall(a.ep, 14, v, 0)
}

// sendMediaKey simule une touche multimédia (secours quand COM n'est pas disponible).
func sendMediaKey(vk byte) {
	const keyUp = 2
	pKeybdEvent.Call(uintptr(vk), 0, 0, 0)
	pKeybdEvent.Call(uintptr(vk), 0, keyUp, 0)
}
