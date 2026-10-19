//go:build windows

package main

// Comportements du Dock façon macOS : rebond de l'icône pendant le lancement
// d'une application, et masquage automatique (le Dock glisse vers le bas, puis
// revient quand la souris touche le bord de l'écran).

import (
	"math"
	"time"

	"menuwin/internal/dockgfx"
)

const (
	dockBounceTimerID = 6
	dockAutoTimerID   = 5
)

var (
	dockBounce struct {
		key   string
		start time.Time
	}
	dockBounceOn bool

	dockSlide       float64 // 0 : visible, 1 : entièrement caché
	dockSlideHidden bool    // la fenêtre est masquée (Dock replié)
	dockLeaveAt     time.Time
	dockAutoOn      bool
)

// ---------------------------------------------------------------------------
// Rebond au lancement
// ---------------------------------------------------------------------------

func startBounce(key string) {
	if key == "" || dockHwnd == 0 {
		return
	}
	dockBounce.key, dockBounce.start = key, time.Now()
	if !dockBounceOn {
		dockBounceOn = true
		pSetTimer.Call(dockHwnd, dockBounceTimerID, 16, 0)
	}
}

func stopBounce() {
	dockBounce.key = ""
	if dockBounceOn && dockHwnd != 0 {
		pKillTimer.Call(dockHwnd, dockBounceTimerID)
	}
	dockBounceOn = false
}

// bounceLift donne le décalage vers le haut de la case i (0 si elle ne rebondit pas).
func bounceLift(i int) float64 {
	if dockBounce.key == "" || i >= len(dockApps) || dockApps[i] == nil || dockApps[i].key != dockBounce.key {
		return 0
	}
	t := time.Since(dockBounce.start).Seconds()
	return math.Abs(math.Sin(math.Pi*t/0.55)) * dockParams.Base * 0.5
}

func dockBounceTick() {
	if dockBounce.key == "" {
		stopBounce()
		return
	}
	t := time.Since(dockBounce.start)
	running := false
	for _, a := range dockApps {
		if a != nil && a.key == dockBounce.key && len(a.hwnds) > 0 && t > 700*time.Millisecond {
			running = true
		}
	}
	if running || t > 6*time.Second {
		stopBounce()
	}
	dockPaint()
}

// ---------------------------------------------------------------------------
// Masquage automatique
// ---------------------------------------------------------------------------

// shiftBuffer décale l'image vers le bord de l'écran où se trouve le Dock (masquage automatique).
func shiftBuffer(bits []uint32, w, h int, o dockgfx.Orient, n int) {
	switch o {
	case dockgfx.Left:
		if n > w {
			n = w
		}
		for y := 0; y < h; y++ {
			row := bits[y*w : (y+1)*w]
			copy(row, row[n:])
			for x := w - n; x < w; x++ {
				row[x] = 0
			}
		}
	case dockgfx.Right:
		if n > w {
			n = w
		}
		for y := 0; y < h; y++ {
			row := bits[y*w : (y+1)*w]
			copy(row[n:], row[:w-n])
			for x := 0; x < n; x++ {
				row[x] = 0
			}
		}
	default:
		shiftRowsDown(bits, w, h, n)
	}
}

// shiftRowsDown décale l'image de n lignes vers le bas ; le haut devient transparent.
func shiftRowsDown(bits []uint32, w, h, n int) {
	if n <= 0 {
		return
	}
	if n > h {
		n = h
	}
	for y := h - 1; y >= 0; y-- {
		dst := bits[y*w : (y+1)*w]
		if y-n >= 0 {
			copy(dst, bits[(y-n)*w:(y-n+1)*w])
		} else {
			for x := range dst {
				dst[x] = 0
			}
		}
	}
}

// dockApplyAutoHide met en route (ou arrête) la surveillance de la souris.
func dockApplyAutoHide() {
	if dockHwnd == 0 {
		return
	}
	if cfg.DockAutoHide && cfg.DockEnabled {
		if !dockAutoOn {
			dockAutoOn = true
			dockLeaveAt = time.Time{}
			pSetTimer.Call(dockHwnd, dockAutoTimerID, 33, 0)
		}
		return
	}
	if dockAutoOn {
		dockAutoOn = false
		pKillTimer.Call(dockHwnd, dockAutoTimerID)
	}
	dockSlide = 0
	if dockSlideHidden {
		dockSlideHidden = false
		if !dockHidden {
			pShowWindow.Call(dockHwnd, swShowNoActivate)
		}
	}
	dockPaint()
}

// dockAutoTick : la souris touche le bord bas de l'écran (sous le Dock) ou survole
// le Dock : il se montre ; elle s'en éloigne un moment : il se replie.
func dockAutoTick() {
	if dockHidden || dockHwnd == 0 {
		return
	}
	p := cursorPt()
	wa := workArea(p.X, p.Y)
	pad := sc(12)
	band := int32(dockParams.PanelH()+dockParams.Bottom) + sc(10) // épaisseur sensible, depuis le bord
	var inBand, edge bool
	switch dockOrient() {
	case dockgfx.Left:
		inY := p.Y >= dockY-pad && p.Y < dockY+dockH+pad
		inBand, edge = inY && p.X <= dockX+band, inY && p.X <= wa.Left
	case dockgfx.Right:
		inY := p.Y >= dockY-pad && p.Y < dockY+dockH+pad
		inBand, edge = inY && p.X >= dockX+dockW-band, inY && p.X >= wa.Right-1
	default:
		inX := p.X >= dockX-pad && p.X < dockX+dockW+pad
		inBand, edge = inX && p.Y >= dockY+dockH-band, inX && p.Y >= wa.Bottom-1
	}
	inside := !dockSlideHidden && dockSlide < 0.5 && inBand
	want := inside || edge || menuOpen || dialogOpen || dockInside

	target := 0.0
	if !want {
		if dockLeaveAt.IsZero() {
			dockLeaveAt = time.Now()
		}
		if time.Since(dockLeaveAt) > 400*time.Millisecond {
			target = 1
		} else {
			target = dockSlide // on attend avant de se replier
		}
	} else {
		dockLeaveAt = time.Time{}
	}
	if target == dockSlide {
		return
	}
	if dockSlideHidden && target < 1 {
		dockSlideHidden = false
		pShowWindow.Call(dockHwnd, swShowNoActivate)
	}
	d := target - dockSlide
	dockSlide += d * 0.3
	if math.Abs(target-dockSlide) < 0.02 {
		dockSlide = target
	}
	if dockSlide >= 1 {
		dockSlideHidden = true
		hideLabel()
		pShowWindow.Call(dockHwnd, 0)
		return
	}
	dockPaint()
}
