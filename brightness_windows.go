//go:build windows

package main

// Luminosité de l'écran dans le Centre de contrôle. Windows ne l'expose que pour les
// écrans intégrés (portables) via WMI : on la lit en arrière-plan au démarrage ; si la
// lecture échoue (écran externe, PC fixe), le curseur n'est tout simplement pas affiché.
// Les écrans externes DDC/CI ne sont pas gérés.

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

var (
	brightOK    atomic.Bool  // la luminosité est réglable sur cette machine
	brightVal   atomic.Int32 // 0 à 100
	brightQueue = make(chan int, 1)
	brightInit  atomic.Bool
)

func powershell(script string, timeout time.Duration) string {
	return powershellEnc(script, timeout)
}

// powershellEnc exécute un script (passé en Base64 UTF-16 : pas de souci de guillemets).
func powershellEnc(script string, timeout time.Duration) string {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	c := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-EncodedCommand", btEncode(script))
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWin}
	out, _ := c.Output()
	return strings.TrimSpace(string(out))
}

// brightnessRead interroge WMI (lent : toujours dans une goroutine).
func brightnessRead() {
	out := powershell("(Get-WmiObject -Namespace root/WMI -Class WmiMonitorBrightness | Select-Object -First 1).CurrentBrightness", 8*time.Second)
	v, err := strconv.Atoi(out)
	if err != nil || v < 0 || v > 100 {
		return
	}
	brightVal.Store(int32(v))
	brightOK.Store(true)
}

// startBrightness lance la lecture initiale et le travailleur qui applique les valeurs.
func startBrightness() {
	if !brightInit.CompareAndSwap(false, true) {
		return
	}
	go brightnessRead()
	go func() {
		for v := range brightQueue {
			powershell("(Get-WmiObject -Namespace root/WMI -Class WmiMonitorBrightnessMethods | Select-Object -First 1).WmiSetBrightness(1,"+strconv.Itoa(v)+")", 8*time.Second)
		}
	}()
}

func brightnessGet() int { return int(brightVal.Load()) }

// brightnessSet mémorise la valeur et l'envoie à l'écran (seule la dernière valeur compte).
func brightnessSet(v int) {
	if v < 0 {
		v = 0
	}
	if v > 100 {
		v = 100
	}
	brightVal.Store(int32(v))
	select {
	case <-brightQueue:
	default:
	}
	select {
	case brightQueue <- v:
	default:
	}
}
