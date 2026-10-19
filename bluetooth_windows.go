//go:build windows

package main

// Bluetooth : état de la radio, appareils appairés, activation et coupure. Windows ne
// donne cela qu'en WinRT (Windows.Devices.Radios) : on passe par PowerShell, toujours
// en arrière-plan, et le résultat est gardé en mémoire pour le menu et la barre.
// « Connecté » est une approximation (état du périphérique vu par Windows).

import (
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type btDev struct {
	name      string
	connected bool
	addr      uint64
}

var bt struct {
	mu      sync.Mutex
	known   bool // une lecture a abouti
	present bool // il y a une radio Bluetooth
	on      bool
	devs    []btDev
}

var btBusy atomic.Bool

const btPrelude = `$ErrorActionPreference='SilentlyContinue'
Add-Type -AssemblyName System.Runtime.WindowsRuntime
$asTask=([System.WindowsRuntimeSystemExtensions].GetMethods()|?{$_.Name -eq 'AsTask' -and $_.GetParameters().Count -eq 1 -and $_.GetParameters()[0].ParameterType.Name -eq 'IAsyncOperation` + "`" + `1'})[0]
Function Await($t,$r){$m=$asTask.MakeGenericMethod($r);$n=$m.Invoke($null,@($t));$n.Wait(-1)|Out-Null;$n.Result}
[Windows.Devices.Radios.Radio,Windows.System.Devices,ContentType=WindowsRuntime]|Out-Null
[Windows.Devices.Radios.RadioAccessStatus,Windows.System.Devices,ContentType=WindowsRuntime]|Out-Null
Await ([Windows.Devices.Radios.Radio]::RequestAccessAsync()) ([Windows.Devices.Radios.RadioAccessStatus])|Out-Null
$radios=Await ([Windows.Devices.Radios.Radio]::GetRadiosAsync()) ([System.Collections.Generic.IReadOnlyList[Windows.Devices.Radios.Radio]])
$bt=$radios|?{$_.Kind -eq 'Bluetooth'}|select -First 1
`

const btRead = btPrelude + `if($bt){'STATE:'+$bt.State}else{'STATE:None'}
`

func btSetScript(on bool) string {
	s := "Off"
	if on {
		s = "On"
	}
	return btPrelude + `if($bt){Await ($bt.SetStateAsync('` + s + `')) ([Windows.Devices.Radios.RadioAccessStatus])|Out-Null}`
}

func btParse(out string) {
	present, on, got := false, false, false
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "STATE:"):
			got = true
			st := strings.TrimPrefix(line, "STATE:")
			present = st != "None" && st != ""
			on = st == "On"
		}
	}
	if !got {
		return
	}
	bt.mu.Lock()
	bt.known, bt.present, bt.on = true, bt.present || present, on
	bt.mu.Unlock()
}

// btRefresh relit l'état en arrière-plan (un seul à la fois).
func btRefresh() {
	if !btBusy.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer btBusy.Store(false)
		// Appareils : API native, instantanée.
		if present, devs := btNativeDevices(); present {
			bt.mu.Lock()
			bt.present, bt.devs = true, devs
			bt.mu.Unlock()
			invalidate(barHwnd)
		}
		// Radio allumée ou non : PowerShell (WinRT), plus lent.
		btParse(powershellEnc(btRead, 20*time.Second))
		invalidate(barHwnd)
	}()
}

// btSetAsync allume ou coupe la radio ; l'affichage change tout de suite.
func btSetAsync(on bool) {
	bt.mu.Lock()
	bt.on = on
	bt.mu.Unlock()
	go func() {
		for btBusy.Load() { // attendre une lecture en cours
			time.Sleep(100 * time.Millisecond)
		}
		btBusy.Store(true)
		powershellEnc(btSetScript(on), 20*time.Second)
		btBusy.Store(false)
		btRefresh()
	}()
}

func btState() (present, on bool, devs []btDev) {
	bt.mu.Lock()
	defer bt.mu.Unlock()
	return bt.present, bt.on, append([]btDev(nil), bt.devs...)
}
