//go:build windows

package main

// Le Dock : la barre flottante du bas de l'écran.
//
//   - applications épinglées, puis applications ouvertes, puis la Corbeille ;
//   - un point sous chaque application ouverte ;
//   - les icônes grossissent sous la souris (moteur : internal/dockgfx) ;
//   - clic : ouvre ou ramène l'application ; clic droit : menu ;
//   - la fenêtre est translucide (UpdateLayeredWindow, pixels ARGB).

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"menuwin/internal/dockgfx"
	"menuwin/internal/launchpad"
)

const (
	dockClass   = "MenuWindowsDock"
	labelClass  = "MenuWindowsDockLabel"
	dockTimerID = 1
)

// Commandes du menu contextuel du Dock.
const (
	cmdDockOpen = 3000 + iota
	cmdDockShow
	cmdDockMin
	cmdDockClose
	cmdDockPin
	cmdDockUnpin
	cmdDockOpenBin
	cmdDockEmptyBin
)

type pinEntry struct{ path, launch string }

type dockApp struct {
	key       string
	path      string // fichier dont on prend l'icône
	launch    string // ce que l'on ouvre au clic
	label     string
	pinned    bool
	frame     bool // fenêtre d'une appli du Store : une icône par fenêtre
	trash     bool
	launchpad bool      // première case : ouvre le Launchpad
	hwnds     []uintptr // fenêtres, la plus récente d'abord
	spriteKey string
}

var (
	dockHwnd, labelHwnd uintptr
	dockParams          dockgfx.Params
	dockW, dockH        int32
	dockX, dockY        int32
	dockHidden          bool

	dockApps  []*dockApp // une entrée par case (nil pour le séparateur)
	dockItems []dockgfx.Item
	dockSig   string
	dockLay   dockgfx.Layout

	dockMouse, dockMouseTarget, dockStrength float64
	dockMouseY                               int32
	dockInside, dockTimerOn, dockTracking    bool
	dockHover                                = -1

	pins        []pinEntry
	runOrder    []string
	spriteCache = map[string]*dockgfx.Sprite{}
	descCache   = map[string]string{}
	pathCache   = map[uintptr]string{}
	trashFull   bool
	trashAt     time.Time

	labelText string
	labelW    int32

	dib struct {
		dc, bmp, old uintptr
		bits         []uint32
		w, h         int32
	}
)

// ---------------------------------------------------------------------------
// Applications épinglées (enregistrées dans %APPDATA%\MenuWindows\dock.txt)
// ---------------------------------------------------------------------------

func pinsFile() string {
	return filepath.Join(os.Getenv("APPDATA"), "MenuWindows", "dock.txt")
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func defaultPins() []pinEntry {
	win := os.Getenv("SystemRoot")
	if win == "" {
		win = `C:\Windows`
	}
	var out []pinEntry
	add := func(path, launch string) {
		if fileExists(path) {
			if launch == "" {
				launch = path
			}
			out = append(out, pinEntry{path, launch})
		}
	}
	add(filepath.Join(win, "explorer.exe"), "")
	for _, env := range []string{"ProgramFiles(x86)", "ProgramFiles", "ProgramW6432"} {
		if base := os.Getenv(env); base != "" {
			edge := filepath.Join(base, `Microsoft\Edge\Application\msedge.exe`)
			if fileExists(edge) {
				add(edge, "")
				break
			}
		}
	}
	add(filepath.Join(win, `ImmersiveControlPanel\SystemSettings.exe`), "ms-settings:")
	add(filepath.Join(win, `System32\notepad.exe`), "")
	add(filepath.Join(win, `System32\calc.exe`), "")
	add(filepath.Join(win, `System32\mspaint.exe`), "")
	return out
}

func loadPins() {
	data, err := os.ReadFile(pinsFile())
	if err != nil {
		pins = defaultPins()
		return
	}
	pins = nil
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		e := pinEntry{path: parts[0], launch: parts[0]}
		if len(parts) == 2 && parts[1] != "" {
			e.launch = parts[1]
		}
		pins = append(pins, e)
	}
}

func savePins() {
	var b strings.Builder
	for _, p := range pins {
		b.WriteString(p.path + "\t" + p.launch + "\n")
	}
	_ = os.MkdirAll(filepath.Dir(pinsFile()), 0o755)
	_ = os.WriteFile(pinsFile(), []byte(b.String()), 0o644)
}

// ---------------------------------------------------------------------------
// Construction de la liste : épinglées + ouvertes + Corbeille
// ---------------------------------------------------------------------------

func appLabel(path string) string {
	k := normKey(path)
	if v, ok := descCache[k]; ok {
		return v
	}
	d := fileDescription(path)
	if d == "" {
		base := filepath.Base(path)
		d = strings.TrimSuffix(base, filepath.Ext(base))
	}
	descCache[k] = d
	return d
}

func cachedProcPath(h uintptr) string {
	if p, ok := pathCache[h]; ok {
		return p
	}
	p := procPath(h)
	pathCache[h] = p
	return p
}

func buildApps() []*dockApp {
	type group struct {
		path, title string
		frame       bool
		hwnds       []uintptr
	}
	groups := map[string]*group{}
	var order []string
	seen := map[uintptr]bool{}
	for _, w := range found { // `found` : fenêtres en ordre d'empilement, la plus haute d'abord
		seen[w.hwnd] = true
		path := cachedProcPath(w.hwnd)
		if path == "" {
			continue
		}
		frame := strings.EqualFold(filepath.Base(path), "ApplicationFrameHost.exe")
		key := normKey(path)
		if frame {
			key = fmt.Sprintf("frame:%x", w.hwnd)
		}
		g := groups[key]
		if g == nil {
			g = &group{path: path, title: w.title, frame: frame}
			groups[key] = g
			order = append(order, key)
		}
		g.hwnds = append(g.hwnds, w.hwnd)
	}
	for h := range pathCache {
		if !seen[h] {
			delete(pathCache, h)
		}
	}

	var apps []*dockApp
	if cfg.DockLaunchpad {
		apps = append(apps, &dockApp{key: "launchpad", launchpad: true, spriteKey: "launchpad", label: t.launchpad})
	}
	used := map[string]bool{}
	for _, p := range pins {
		k := normKey(p.path)
		a := &dockApp{key: k, path: p.path, launch: p.launch, pinned: true, spriteKey: k, label: appLabel(p.path)}
		if g := groups[k]; g != nil {
			a.hwnds = g.hwnds
			used[k] = true
		}
		apps = append(apps, a)
	}

	// Applications ouvertes mais non épinglées : on garde leur ordre d'arrivée.
	if !cfg.DockRunning {
		for k := range used {
			delete(used, k)
		}
		groups = map[string]*group{}
		order = nil
	}
	var keep []string
	for _, k := range runOrder {
		if g := groups[k]; g != nil && !used[k] {
			keep = append(keep, k)
		}
	}
	for _, k := range order {
		if used[k] {
			continue
		}
		known := false
		for _, e := range keep {
			if e == k {
				known = true
				break
			}
		}
		if !known {
			keep = append(keep, k)
		}
	}
	runOrder = keep
	for _, k := range runOrder {
		g := groups[k]
		a := &dockApp{key: k, path: g.path, launch: g.path, frame: g.frame, hwnds: g.hwnds, spriteKey: k}
		if g.frame {
			a.label = g.title
		} else {
			a.label = appLabel(g.path)
		}
		apps = append(apps, a)
	}

	if cfg.DockTrash {
		bin := "trash:empty"
		if trashFull {
			bin = "trash:full"
		}
		apps = append(apps, &dockApp{key: "trash", trash: true, spriteKey: bin, label: t.bin})
	}
	return apps
}

func appSprite(a *dockApp) *dockgfx.Sprite {
	if s, ok := spriteCache[a.spriteKey]; ok {
		return s
	}
	var s *dockgfx.Sprite
	switch {
	case a.launchpad:
		if s = customSprite("launchpad"); s == nil {
			s = launchpad.Icon(int(dockParams.Max*1.5 + 0.5))
		}
	case a.trash:
		name := "trash"
		if trashFull {
			name = "trash-full"
		}
		if s = customSprite(name); s == nil {
			s = trashSprite(trashFull)
		}
	case a.frame && len(a.hwnds) > 0:
		if s = customTextSprite(a.label); s == nil {
			if s = windowSprite(a.hwnds[0]); s != nil {
				s = fitSprite(s)
			}
		}
	default:
		if s = customAppSprite(a.path, a.label); s == nil {
			s = spriteForPath(a.path)
		}
	}
	if s == nil {
		s = defaultSprite()
	}
	spriteCache[a.spriteKey] = s
	return s
}

// dockRefresh est appelée régulièrement (toutes les 500 ms) par la barre du haut.
func dockRefresh() {
	if dockHwnd == 0 || !cfg.DockEnabled {
		return
	}
	if time.Since(trashAt) > 4*time.Second {
		trashAt = time.Now()
		trashFull = recycleBinFull()
	}
	apps := buildApps()

	var sig strings.Builder
	for _, a := range apps {
		fmt.Fprintf(&sig, "%s|%s|%t;", a.spriteKey, a.label, len(a.hwnds) > 0)
	}
	changed := sig.String() != dockSig
	if changed {
		dockSig = sig.String()
		rebuildItems(apps)
	} else {
		// même disposition : on garde les cases, mais on met à jour les fenêtres
		j := 0
		for i, a := range dockApps {
			if a == nil {
				continue
			}
			a.hwnds = apps[j].hwnds
			dockApps[i] = a
			j++
		}
	}
	if placeDock() || changed {
		dockPaint()
	}
}

func rebuildItems(apps []*dockApp) {
	wanted := map[string]bool{}
	dockApps = dockApps[:0]
	dockItems = dockItems[:0]
	for i, a := range apps {
		if a.trash && i > 0 {
			dockApps = append(dockApps, nil)
			dockItems = append(dockItems, dockgfx.Item{Separator: true})
		}
		wanted[a.spriteKey] = true
		dockApps = append(dockApps, a)
		dockItems = append(dockItems, dockgfx.Item{Sprite: appSprite(a), Running: len(a.hwnds) > 0 && cfg.DockDots})
	}
	for k := range spriteCache {
		if !wanted[k] {
			delete(spriteCache, k)
		}
	}
	dockHover = -1
	hideLabel()
}

// placeDock règle la taille et la position de la fenêtre (en bas, au centre de
// la zone de travail). Renvoie true si quelque chose a changé.
func placeDock() bool {
	w := int32(dockParams.WindowW(dockItems))
	h := int32(dockParams.WindowH())
	wa := workArea(0, 0)
	if limit := wa.Right - wa.Left; w > limit {
		w = limit
	}
	x := wa.Left + (wa.Right-wa.Left-w)/2
	y := wa.Bottom - h
	if w == dockW && h == dockH && x == dockX && y == dockY && dib.bits != nil {
		return false
	}
	if w != dib.w || h != dib.h || dib.bits == nil {
		makeDIB(w, h)
	}
	dockW, dockH, dockX, dockY = w, h, x, y
	pSetWindowPos.Call(dockHwnd, ^uintptr(0), uintptr(x), uintptr(y), uintptr(w), uintptr(h), swpNoActivate)
	return true
}

// ---------------------------------------------------------------------------
// Dessin dans la fenêtre translucide
// ---------------------------------------------------------------------------

func freeDIB() {
	if dib.dc != 0 {
		pSelectObject.Call(dib.dc, dib.old)
		pDeleteObject.Call(dib.bmp)
		pDeleteDC.Call(dib.dc)
	}
	dib.dc, dib.bmp, dib.old, dib.bits = 0, 0, 0, nil
}

func makeDIB(w, h int32) {
	freeDIB()
	screen, _, _ := pGetDC.Call(0)
	defer pReleaseDC.Call(0, screen)
	dc, _, _ := pCreateCompatDC.Call(screen)
	bi := bitmapInfoHeader{Width: w, Height: -h, Planes: 1, BitCount: 32}
	bi.Size = uint32(unsafe.Sizeof(bi))
	var bits unsafe.Pointer
	bmp, _, _ := pCreateDIBSection.Call(dc, uintptr(unsafe.Pointer(&bi)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if bmp == 0 || bits == nil {
		pDeleteDC.Call(dc)
		return
	}
	old, _, _ := pSelectObject.Call(dc, bmp)
	dib.dc, dib.bmp, dib.old, dib.w, dib.h = dc, bmp, old, w, h
	dib.bits = unsafe.Slice((*uint32)(bits), int(w)*int(h))
}

func dockPaint() {
	if dockHwnd == 0 || dib.bits == nil || dockHidden {
		return
	}
	dockLay = dockgfx.Compute(dockParams, dockItems, int(dockW), int(dockH), dockMouse, dockStrength)
	dockgfx.Render(dib.bits, int(dockW), int(dockH), dockParams, dockItems, dockLay, 2)

	pos := point{dockX, dockY}
	sz := size{dockW, dockH}
	src := point{}
	bf := blendFunction{SourceConstantAlpha: 255, AlphaFormat: 1 /*AC_SRC_ALPHA*/}
	pUpdateLayeredWindow.Call(dockHwnd, 0, uintptr(unsafe.Pointer(&pos)), uintptr(unsafe.Pointer(&sz)),
		dib.dc, uintptr(unsafe.Pointer(&src)), 0, uintptr(unsafe.Pointer(&bf)), ulwAlpha)
}

// ---------------------------------------------------------------------------
// Animation du zoom
// ---------------------------------------------------------------------------

func startAnim() {
	if !dockTimerOn {
		dockTimerOn = true
		pSetTimer.Call(dockHwnd, dockTimerID, 16, 0)
	}
}

func dockTick() {
	target := 0.0
	if dockInside {
		target = 1
	}
	dockStrength += (target - dockStrength) * 0.35
	dockMouse += (dockMouseTarget - dockMouse) * 0.45
	if math.Abs(dockStrength-target) < 0.01 && math.Abs(dockMouse-dockMouseTarget) < 0.5 {
		dockStrength, dockMouse = target, dockMouseTarget
		pKillTimer.Call(dockHwnd, dockTimerID)
		dockTimerOn = false
	}
	dockPaint()
	if dockInside {
		setHover(dockgfx.HitTest(dockParams, dockItems, dockLay, dockMouseTarget, float64(dockMouseY)))
	}
	moveLabel()
}

// ---------------------------------------------------------------------------
// Étiquette au survol
// ---------------------------------------------------------------------------

func setHover(i int) {
	if i == dockHover {
		return
	}
	dockHover = i
	if i >= 0 && i < len(dockApps) && dockApps[i] != nil && cfg.DockLabels {
		showLabel(dockApps[i].label)
	} else {
		hideLabel()
	}
}

func labelHeight() int32 { return sc(28) }

func showLabel(text string) {
	if text == "" {
		hideLabel()
		return
	}
	registerDockClasses()
	hInst, _, _ := pGetModuleHandle.Call(0)
	if labelHwnd == 0 {
		labelHwnd, _, _ = pCreateWindowEx.Call(wsExTopmost|wsExToolwin|wsExNoActive, ptr(utf16(labelClass)), 0, wsPopup,
			0, 0, 10, 10, 0, 0, hInst, 0)
		if labelHwnd == 0 {
			return
		}
	}
	dc, _, _ := pGetDC.Call(0)
	old, _, _ := pSelectObject.Call(dc, fontReg)
	tw := textWidth(dc, text)
	pSelectObject.Call(dc, old)
	pReleaseDC.Call(0, dc)

	labelText = text
	labelW = tw + 2*sc(14)
	styleWindow(labelHwnd, labelW, labelHeight())
	moveLabel()
	pShowWindow.Call(labelHwnd, swShowNoActivate)
	pInvalidateRect.Call(labelHwnd, 0, 0)
}

// moveLabel place l'étiquette au-dessus de l'icône survolée.
func moveLabel() {
	if labelHwnd == 0 || dockHover < 0 || dockHover >= len(dockLay.Items) {
		return
	}
	r := dockLay.Items[dockHover]
	x := dockX + int32(r.X+r.W/2) - labelW/2
	y := dockY + int32(r.Y) - labelHeight() - sc(6)
	wa := workArea(x, y)
	if x+labelW > wa.Right {
		x = wa.Right - labelW
	}
	if x < wa.Left {
		x = wa.Left
	}
	pSetWindowPos.Call(labelHwnd, ^uintptr(0), uintptr(x), uintptr(y), uintptr(labelW), uintptr(labelHeight()), swpNoActivate|swpShowWindow)
}

func hideLabel() {
	if labelHwnd != 0 {
		pShowWindow.Call(labelHwnd, 0 /*SW_HIDE*/)
	}
}

func labelProc(hwnd, message, wparam, lparam uintptr) uintptr {
	switch uint32(message) {
	case wmEraseBkgnd:
		return 1
	case wmPaint:
		paintBuffered(hwnd, func(mem uintptr, w, h int32) {
			r := rect{0, 0, w, h}
			pFillRect.Call(mem, uintptr(unsafe.Pointer(&r)), brushMenuBg)
			pSetBkMode.Call(mem, transparent)
			pSetTextColor.Call(mem, pal.menuText)
			pSelectObject.Call(mem, fontReg)
			drawText(mem, labelText, r, dtCenter)
		})
		return 0
	}
	r, _, _ := pDefWindowProc.Call(hwnd, message, wparam, lparam)
	return r
}

// ---------------------------------------------------------------------------
// Actions
// ---------------------------------------------------------------------------

func bringToFront(h uintptr) {
	if ic, _, _ := pIsIconic.Call(h); ic != 0 {
		pShowWindow.Call(h, swRestore)
	}
	pSetForeground.Call(h)
}

func dockActivate(i int) {
	if i < 0 || i >= len(dockApps) || dockApps[i] == nil {
		return
	}
	a := dockApps[i]
	switch {
	case a.launchpad:
		showLaunchpad()
	case a.trash:
		openURI("shell:RecycleBinFolder")
	case len(a.hwnds) > 0:
		// Plusieurs fenêtres : un clic de plus passe à la suivante.
		fg, _, _ := pGetForeground.Call()
		target := a.hwnds[0]
		for j, h := range a.hwnds {
			if h == fg && len(a.hwnds) > 1 {
				target = a.hwnds[(j+1)%len(a.hwnds)]
			}
		}
		bringToFront(target)
	default:
		openURI(a.launch)
	}
}

func rebuildNow() {
	dockSig = ""
	dockRefresh()
}

func dockContextMenu(i int) {
	if i < 0 || i >= len(dockApps) || dockApps[i] == nil {
		return
	}
	a := dockApps[i]
	if a.launchpad {
		return
	}
	r := dockLay.Items[i]
	x, y := dockX+int32(r.X), dockY+int32(r.Y)

	var items []mItem
	switch {
	case a.trash:
		items = []mItem{mi(cmdDockOpenBin, t.openBin), mi(cmdDockEmptyBin, t.emptyBin)}
	default:
		if len(a.hwnds) > 0 {
			items = append(items, mi(cmdDockShow, t.restoreWin), mi(cmdDockMin, t.minimizeWin), mi(cmdDockClose, t.closeWin))
		} else {
			items = append(items, mi(cmdDockOpen, t.open))
		}
		if !a.frame {
			items = append(items, msep())
			if a.pinned {
				items = append(items, mi(cmdDockUnpin, t.unpin))
			} else {
				items = append(items, mi(cmdDockPin, t.pin))
			}
		}
	}
	setHover(-1)
	switch trackMenu(dockHwnd, items, x, y) {
	case cmdDockOpen:
		openURI(a.launch)
	case cmdDockShow:
		if len(a.hwnds) > 0 {
			bringToFront(a.hwnds[0])
		}
	case cmdDockMin:
		for _, h := range a.hwnds {
			pShowWindow.Call(h, swMinimize)
		}
	case cmdDockClose:
		for _, h := range a.hwnds {
			pPostMessage.Call(h, wmClose, 0, 0)
		}
	case cmdDockPin:
		pins = append(pins, pinEntry{path: a.path, launch: a.path})
		savePins()
		rebuildNow()
	case cmdDockUnpin:
		var keep []pinEntry
		for _, p := range pins {
			if normKey(p.path) != a.key {
				keep = append(keep, p)
			}
		}
		pins = keep
		savePins()
		rebuildNow()
	case cmdDockOpenBin:
		openURI("shell:RecycleBinFolder")
	case cmdDockEmptyBin:
		confirmHintOverride = t.emptyHint
		ok := confirm(dockHwnd, t.confirmEmpty, t.emptyBtn)
		confirmHintOverride = ""
		if ok {
			pSHEmptyRecycleBin.Call(0, 0, shErbNoConfirm|shErbNoProgress|shErbNoSound)
			trashAt = time.Time{}
		}
	}
}

// ---------------------------------------------------------------------------
// Fenêtre du Dock
// ---------------------------------------------------------------------------

func inDock(p point) bool {
	if dockHwnd == 0 || dockHidden {
		return false
	}
	var wr rect
	pGetWindowRect.Call(dockHwnd, uintptr(unsafe.Pointer(&wr)))
	return p.X >= wr.Left && p.X < wr.Right && p.Y >= wr.Top && p.Y < wr.Bottom
}

func dockHit(lparam uintptr) (int, int32, int32) {
	x, y := lparamXY(lparam)
	return dockgfx.HitTest(dockParams, dockItems, dockLay, float64(x), float64(y)), x, y
}

func dockProc(hwnd, message, wparam, lparam uintptr) uintptr {
	switch uint32(message) {
	case wmEraseBkgnd:
		return 1

	case wmMouseMove:
		if menuOpen || dialogOpen {
			return 0
		}
		i, x, y := dockHit(lparam)
		dockMouseTarget, dockMouseY = float64(x), y
		if !dockInside && dockStrength < 0.02 {
			dockMouse = dockMouseTarget // le zoom démarre sous la souris, sans balayage
		}
		dockInside = true
		if !dockTracking {
			tme := trackMouseEvent{Flags: tmeLeave, HwndTrack: hwnd}
			tme.Size = uint32(unsafe.Sizeof(tme))
			pTrackMouseEvent.Call(uintptr(unsafe.Pointer(&tme)))
			dockTracking = true
		}
		setHover(i)
		startAnim()
		return 0

	case wmMouseLeave:
		dockTracking, dockInside = false, false
		setHover(-1)
		startAnim()
		return 0

	case wmTimer:
		if wparam == dockTimerID {
			dockTick()
		}
		return 0

	case wmLButtonDown, wmRButtonDown:
		if menuOpen {
			ignoreNextUp = true
			cancelMenu()
		} else {
			ignoreNextUp = false
		}
		return 0

	case wmLButtonUp:
		if ignoreNextUp {
			ignoreNextUp = false
			return 0
		}
		if menuOpen || dialogOpen {
			return 0
		}
		i, _, _ := dockHit(lparam)
		dockActivate(i)
		return 0

	case wmRButtonUp:
		if ignoreNextUp {
			ignoreNextUp = false
			return 0
		}
		if menuOpen || dialogOpen {
			return 0
		}
		i, _, _ := dockHit(lparam)
		dockContextMenu(i)
		return 0

	case wmDisplayChange:
		if placeDock() {
			dockPaint()
		}
		return 0
	}
	r, _, _ := pDefWindowProc.Call(hwnd, message, wparam, lparam)
	return r
}

var dockClassesOK bool

func registerDockClasses() {
	if dockClassesOK {
		return
	}
	hInst, _, _ := pGetModuleHandle.Call(0)
	cursor, _, _ := pLoadCursor.Call(0, idcArrow)
	reg := func(name string, proc uintptr) {
		wc := wndClassEx{WndProc: proc, Instance: hInst, Cursor: cursor, ClassName: utf16(name)}
		wc.Size = uint32(unsafe.Sizeof(wc))
		pRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
	}
	reg(dockClass, syscall.NewCallback(dockProc))
	reg(labelClass, syscall.NewCallback(labelProc))
	dockClassesOK = true
}

func initDock() {
	dockParams = dockParamsFromConfig()
	loadPins()
	registerDockClasses()
	hInst, _, _ := pGetModuleHandle.Call(0)
	dockHwnd, _, _ = pCreateWindowEx.Call(
		wsExLayered|wsExTopmost|wsExToolwin|wsExNoActive,
		ptr(utf16(dockClass)), ptr(utf16(t.appTitle)), wsPopup,
		0, 0, 10, 10, 0, 0, hInst, 0)
	if dockHwnd == 0 {
		return
	}
	trashAt = time.Now()
	trashFull = recycleBinFull()
	dockRefresh()
	pShowWindow.Call(dockHwnd, swShowNoActivate)
	dockPaint()
	dockUpdateHidden() // Dock désactivé dans les réglages : on le cache aussitôt
}

var dockFull bool // une application est en plein écran

// dockSetHidden masque le Dock quand une application passe en plein écran.
func dockSetHidden(full bool) {
	dockFull = full
	dockUpdateHidden()
}

// dockUpdateHidden affiche ou masque la fenêtre : plein écran, ou Dock désactivé dans les réglages.
func dockUpdateHidden() {
	hidden := dockFull || !cfg.DockEnabled
	if dockHwnd == 0 || hidden == dockHidden {
		return
	}
	dockHidden = hidden
	if hidden {
		hideLabel()
		pShowWindow.Call(dockHwnd, 0)
		return
	}
	pShowWindow.Call(dockHwnd, swShowNoActivate)
	dockPaint()
}

func shutdownDock() {
	if dockTimerOn && dockHwnd != 0 {
		pKillTimer.Call(dockHwnd, dockTimerID)
	}
	for _, h := range []uintptr{labelHwnd, dockHwnd} {
		if h != 0 {
			pDestroyWindow.Call(h)
		}
	}
	labelHwnd, dockHwnd = 0, 0
	freeDIB()
}
