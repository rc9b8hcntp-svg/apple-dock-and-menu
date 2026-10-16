//go:build windows

package main

// Réglages de Menu Windows : lecture et écriture de settings.ini, application
// des réglages en direct, et la fenêtre « Réglages de Menu Windows… ».
//
// La fenêtre est dessinée par le programme, comme les menus : une barre
// latérale (Général, Apparence, Barre, Dock) et des lignes avec des
// interrupteurs, des curseurs, des choix et des boutons. Chaque changement est
// appliqué tout de suite et enregistré dans %APPDATA%\MenuWindows\settings.ini.

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"

	"menuwin/internal/config"
	"menuwin/internal/dockgfx"
)

// ---------------------------------------------------------------------------
// Fichier de réglages et langue
// ---------------------------------------------------------------------------

func configFile() string {
	return filepath.Join(os.Getenv("APPDATA"), "MenuWindows", "settings.ini")
}

func loadConfig() {
	if data, err := os.ReadFile(configFile()); err == nil {
		cfg = config.Parse(string(data))
	}
}

func saveConfig() {
	_ = os.MkdirAll(filepath.Dir(configFile()), 0o755)
	_ = os.WriteFile(configFile(), []byte(cfg.Marshal()), 0o644)
}

var langFR bool

// tr choisit le texte français ou anglais selon la langue en vigueur.
func tr(f, e string) string {
	if langFR {
		return f
	}
	return e
}

func applyLanguage() {
	switch cfg.Language {
	case "fr":
		langFR = true
	case "en":
		langFR = false
	default:
		lang, _, _ := pGetUILanguage.Call()
		langFR = lang&0xFF == 0x0C // LANG_FRENCH
	}
	if langFR {
		t = fr
	} else {
		t = en
	}
}

// ---------------------------------------------------------------------------
// Application des réglages
// ---------------------------------------------------------------------------

var (
	applied   config.Config // ce qui est déjà appliqué à l'écran
	hotkeysOn bool
)

func setHotkeys(on bool) {
	if on == hotkeysOn || barHwnd == 0 {
		return
	}
	if on {
		pRegisterHotKey.Call(barHwnd, 1, modControl|modAlt|modNoRepeat, 'M')
		pRegisterHotKey.Call(barHwnd, 2, modControl|modAlt|modNoRepeat, 'L')
	} else {
		pUnregisterHotKey.Call(barHwnd, 1)
		pUnregisterHotKey.Call(barHwnd, 2)
	}
	hotkeysOn = on
}

func applyBarOpacity() {
	if barHwnd == 0 {
		return
	}
	pSetLayeredWindowAttributes.Call(barHwnd, 0, uintptr(255*cfg.BarOpacity/100), lwaAlpha)
}

func dockParamsFromConfig() dockgfx.Params {
	p := dockgfx.NewParams(scale * float64(cfg.DockSize) / 48)
	p.Max = p.Base * float64(cfg.DockZoom) / 100
	return p
}

// applyDock réapplique les réglages du Dock (taille, cases, étiquettes…).
func applyDock() {
	if dockHwnd == 0 {
		return
	}
	dockParams = dockParamsFromConfig()
	dockSig = ""
	dockUpdateHidden()
	dockRefresh()
}

// applyConfig met à l'écran ce qui a changé depuis le dernier appel.
func applyConfig() {
	old := applied
	applied = cfg

	if cfg.Language != old.Language {
		applyLanguage()
		if st.hwnd != 0 {
			buildPages()
			pSetWindowText.Call(st.hwnd, ptr(utf16(tr("Réglages de Menu Windows", "Menu Windows Settings"))))
		}
	}
	if cfg.Theme != old.Theme || cfg.Accent != old.Accent {
		rebuildTheme()
		styleSettingsWindow()
	}
	if cfg.BarHeight != old.BarHeight {
		barH = sc(float64(cfg.BarHeight))
		positionBar(barHwnd)
	}
	if cfg.BarOpacity != old.BarOpacity {
		applyBarOpacity()
	}
	setHotkeys(cfg.Hotkey)
	if cfg.ShowDate != old.ShowDate || cfg.ShowSeconds != old.ShowSeconds || cfg.Clock != old.Clock {
		updateClock()
	}

	dockChanged := cfg.Language != old.Language || cfg.DockEnabled != old.DockEnabled ||
		cfg.DockSize != old.DockSize || cfg.DockZoom != old.DockZoom ||
		cfg.DockLabels != old.DockLabels || cfg.DockDots != old.DockDots ||
		cfg.DockRunning != old.DockRunning || cfg.DockTrash != old.DockTrash ||
		cfg.DockLaunchpad != old.DockLaunchpad
	if dockChanged {
		applyDock()
	}
	if barHwnd != 0 {
		invalidate(barHwnd)
	}
	if st.hwnd != 0 {
		pInvalidateRect.Call(st.hwnd, 0, 0)
	}
}

// checkTheme suit le mode clair/sombre de Windows quand le thème est « Automatique ».
func checkTheme() {
	if cfg.Theme == "auto" && effectiveDark() != pal.dark {
		rebuildTheme()
		styleSettingsWindow()
		if barHwnd != 0 {
			invalidate(barHwnd)
		}
		if st.hwnd != 0 {
			pInvalidateRect.Call(st.hwnd, 0, 0)
		}
	}
}

// settingChanged est appelé après chaque modification dans la fenêtre.
func settingChanged(save bool) {
	if save {
		saveConfig()
	}
	applyConfig()
}

// ---------------------------------------------------------------------------
// Description des pages
// ---------------------------------------------------------------------------

const (
	rowToggle = iota
	rowSlider
	rowChoice
	rowSwatch
	rowButton
	rowHeader
)

type sRow struct {
	kind           int
	label          string
	getB           func() bool
	setB           func(bool)
	n              *int
	min, max, step int
	unit           string
	s              *string
	opts, names    []string
	action         func()
}

type sPage struct {
	name string
	rows []sRow
}

func rToggle(label string, b *bool) sRow {
	return sRow{kind: rowToggle, label: label, getB: func() bool { return *b }, setB: func(v bool) { *b = v }}
}

func rToggleF(label string, get func() bool, set func(bool)) sRow {
	return sRow{kind: rowToggle, label: label, getB: get, setB: set}
}

func rSlider(label string, n *int, min, max, step int, unit string) sRow {
	return sRow{kind: rowSlider, label: label, n: n, min: min, max: max, step: step, unit: unit}
}

func rChoice(label string, s *string, opts, names []string) sRow {
	return sRow{kind: rowChoice, label: label, s: s, opts: opts, names: names}
}

func rSwatch(label string, s *string) sRow {
	return sRow{kind: rowSwatch, label: label, s: s, opts: config.Accents}
}

func rButton(label string, action func()) sRow {
	return sRow{kind: rowButton, label: label, action: action}
}

func rHeader(label string) sRow { return sRow{kind: rowHeader, label: label} }

func openIconsFolder() {
	_ = os.MkdirAll(iconsDir(), 0o755)
	openURI(iconsDir())
}

func resetAllSettings() {
	confirmHintOverride = tr("Tous les réglages reviennent à leurs valeurs d’origine.", "All settings go back to their default values.")
	ok := confirm(st.hwnd, tr("Rétablir tous les réglages ?", "Reset all settings?"), tr("Rétablir", "Reset"))
	confirmHintOverride = ""
	if ok {
		cfg = config.Default()
		settingChanged(true)
	}
}

func resetDockPins() {
	confirmHintOverride = tr("Le Dock reprend sa liste d’applications d’origine.", "The Dock goes back to its original list of apps.")
	ok := confirm(st.hwnd, tr("Rétablir les applications du Dock ?", "Reset the Dock apps?"), tr("Rétablir", "Reset"))
	confirmHintOverride = ""
	if ok {
		_ = os.Remove(pinsFile())
		loadPins()
		rebuildNow()
	}
}

func buildPages() {
	auto := tr("Automatique", "Automatic")
	st.pages = []sPage{
		{tr("Général", "General"), []sRow{
			rChoice(tr("Langue", "Language"), &cfg.Language, config.Languages, []string{auto, "Français", "English"}),
			rToggleF(tr("Lancer à l’ouverture de session", "Open at login"), autostartEnabled, setAutostart),
			rToggle(tr("Raccourcis clavier (Ctrl+Alt+M et Ctrl+Alt+L)", "Keyboard shortcuts (Ctrl+Alt+M and Ctrl+Alt+L)"), &cfg.Hotkey),
			rHeader(tr("Icônes et réglages", "Icons and settings")),
			rButton(tr("Ouvrir le dossier des icônes", "Open the icons folder"), openIconsFolder),
			rButton(tr("Rétablir tous les réglages…", "Reset all settings…"), resetAllSettings),
		}},
		{tr("Apparence", "Appearance"), []sRow{
			rChoice(tr("Thème", "Theme"), &cfg.Theme, config.Themes, []string{auto, tr("Clair", "Light"), tr("Sombre", "Dark")}),
			rSwatch(tr("Couleur d’accentuation", "Accent color"), &cfg.Accent),
			rSlider(tr("Opacité de la barre", "Menu bar opacity"), &cfg.BarOpacity, config.BarOpacityMin, config.BarOpacityMax, 5, " %"),
		}},
		{tr("Barre", "Menu Bar"), []sRow{
			rSlider(tr("Hauteur de la barre", "Menu bar height"), &cfg.BarHeight, config.BarHeightMin, config.BarHeightMax, 1, " px"),
			rToggle(tr("Nom de l’application active", "Active app name"), &cfg.ShowAppName),
			rToggle(tr("Menu Applications", "Applications menu"), &cfg.ShowApps),
			rToggle(tr("Menu Fenêtre", "Window menu"), &cfg.ShowWindow),
			rHeader(tr("Horloge", "Clock")),
			rToggle(tr("Afficher la date", "Show the date"), &cfg.ShowDate),
			rToggle(tr("Afficher les secondes", "Show seconds"), &cfg.ShowSeconds),
			rChoice(tr("Format de l’heure", "Time format"), &cfg.Clock, config.ClockFormats, []string{auto, "24 h", "12 h"}),
		}},
		{tr("Icônes d’état", "Status Icons"), []sRow{
			rToggle(tr("Batterie", "Battery"), &cfg.ShowBattery),
			rToggle(tr("Pourcentage de la batterie", "Battery percentage"), &cfg.ShowBatteryPct),
			rToggle("Wi-Fi", &cfg.ShowWifi),
			rToggle(tr("Son", "Sound"), &cfg.ShowSound),
			rToggle(tr("Recherche", "Search"), &cfg.ShowSearch),
			rToggle(tr("Centre de contrôle", "Control Center"), &cfg.ShowControl),
		}},
		{"Dock", []sRow{
			rToggle(tr("Afficher le Dock", "Show the Dock"), &cfg.DockEnabled),
			rSlider(tr("Taille des icônes", "Icon size"), &cfg.DockSize, config.DockSizeMin, config.DockSizeMax, 2, " px"),
			rSlider(tr("Agrandissement", "Magnification"), &cfg.DockZoom, config.DockZoomMin, config.DockZoomMax, 5, " %"),
			rToggle(tr("Icône du Launchpad", "Launchpad icon"), &cfg.DockLaunchpad),
			rToggle(tr("Étiquettes au survol", "Hover labels"), &cfg.DockLabels),
			rToggle(tr("Point sous les applications ouvertes", "Dot under open apps"), &cfg.DockDots),
			rToggle(tr("Applications ouvertes non épinglées", "Open apps that aren’t pinned"), &cfg.DockRunning),
			rToggle(tr("Corbeille", "Recycle Bin"), &cfg.DockTrash),
			rButton(tr("Rétablir les applications du Dock…", "Reset the Dock apps…"), resetDockPins),
		}},
	}
	if st.page >= len(st.pages) {
		st.page = 0
	}
}

// ---------------------------------------------------------------------------
// État et mise en page de la fenêtre
// ---------------------------------------------------------------------------

type sSlot struct {
	row  int
	r    rect   // toute la ligne
	ctl  rect   // contrôle principal (interrupteur, piste, bouton)
	val  rect   // valeur affichée (curseur)
	segs []rect // segments d'un choix, pastilles de couleur
}

var st struct {
	hwnd        uintptr
	pages       []sPage
	page        int
	slots       []sSlot
	groups      []rect
	side        []rect
	w, h        int32
	fontBig     uintptr
	hoverRow    int
	hoverSeg    int
	pressRow    int
	pressSeg    int
	drag        int // ligne dont on déplace le curseur, -1 sinon
	dragChanged bool
	tracking    bool
}

const settingsClass = "MenuWindowsSettings"

func sideW() int32 { return sc(190) }

func sLayout() {
	st.slots, st.groups, st.side = st.slots[:0], st.groups[:0], st.side[:0]
	dc, _, _ := pGetDC.Call(0)
	old, _, _ := pSelectObject.Call(dc, fontReg)
	defer func() {
		pSelectObject.Call(dc, old)
		pReleaseDC.Call(0, dc)
	}()

	side, pad := sideW(), sc(24)
	for i := range st.pages {
		st.side = append(st.side, rect{sc(10), sc(16) + int32(i)*sc(36), side - sc(10), sc(16) + int32(i)*sc(36) + sc(32)})
	}

	x0, x1 := side+pad, st.w-pad
	y := sc(78)
	rowH, hdrH := sc(42), sc(34)
	groupTop := int32(-1)
	closeGroup := func(bottom int32) {
		if groupTop >= 0 {
			st.groups = append(st.groups, rect{x0, groupTop, x1, bottom})
			groupTop = -1
		}
	}

	for i, row := range st.pages[st.page].rows {
		if row.kind == rowHeader {
			closeGroup(y)
			y += sc(8)
			st.slots = append(st.slots, sSlot{row: i, r: rect{x0, y, x1, y + hdrH}})
			y += hdrH
			continue
		}
		if groupTop < 0 {
			groupTop = y
		}
		sl := sSlot{row: i, r: rect{x0, y, x1, y + rowH}}
		cy := y + rowH/2
		right := x1 - sc(14)
		switch row.kind {
		case rowToggle:
			sl.ctl = rect{right - sc(44), cy - sc(13), right, cy + sc(13)}
		case rowSlider:
			sl.val = rect{right - sc(56), y, right, y + rowH}
			trackW := sc(170)
			sl.ctl = rect{sl.val.Left - sc(10) - trackW, cy - sc(11), sl.val.Left - sc(10), cy + sc(11)}
		case rowChoice:
			var widths []int32
			var total int32
			for _, n := range row.names {
				w := textWidth(dc, n) + sc(26)
				widths = append(widths, w)
				total += w
			}
			x := right - total
			for _, w := range widths {
				sl.segs = append(sl.segs, rect{x, cy - sc(14), x + w, cy + sc(14)})
				x += w
			}
			sl.ctl = rect{right - total, cy - sc(14), right, cy + sc(14)}
		case rowSwatch:
			d, gap := sc(22), sc(10)
			total := int32(len(row.opts))*d + int32(len(row.opts)-1)*gap
			x := right - total
			for range row.opts {
				sl.segs = append(sl.segs, rect{x, cy - d/2, x + d, cy + d/2})
				x += d + gap
			}
			sl.ctl = rect{right - total, cy - d/2, right, cy + d/2}
		case rowButton:
			w := textWidth(dc, row.label) + sc(36)
			sl.ctl = rect{x0 + sc(14), cy - sc(15), x0 + sc(14) + w, cy + sc(15)}
		}
		st.slots = append(st.slots, sl)
		y += rowH
	}
	closeGroup(y)
}

func inRect(r rect, x, y int32) bool { return x >= r.Left && x < r.Right && y >= r.Top && y < r.Bottom }

// sHitTest renvoie la ligne et le segment sous (x, y) : (-1, -1) si rien.
func sHitTest(x, y int32) (int, int) {
	for _, sl := range st.slots {
		row := st.pages[st.page].rows[sl.row]
		switch row.kind {
		case rowToggle:
			if inRect(sl.r, x, y) {
				return sl.row, 0
			}
		case rowSlider:
			ex := rect{sl.ctl.Left - sc(8), sl.r.Top, sl.ctl.Right + sc(8), sl.r.Bottom}
			if inRect(ex, x, y) {
				return sl.row, 0
			}
		case rowChoice, rowSwatch:
			for i, s := range sl.segs {
				if inRect(s, x, y) {
					return sl.row, i
				}
			}
		case rowButton:
			if inRect(sl.ctl, x, y) {
				return sl.row, 0
			}
		}
	}
	return -1, -1
}

func slotOf(row int) *sSlot {
	for i := range st.slots {
		if st.slots[i].row == row {
			return &st.slots[i]
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Dessin
// ---------------------------------------------------------------------------

func fillColor(hdc uintptr, r rect, col uintptr) {
	b, _, _ := pCreateSolidBrush.Call(col)
	pFillRect.Call(hdc, uintptr(unsafe.Pointer(&r)), b)
	pDeleteObject.Call(b)
}

func shrink(r rect, d int32) rect { return rect{r.Left + d, r.Top + d, r.Right - d, r.Bottom - d} }

func drawSettings(hdc uintptr, w, h int32) {
	sLayout()
	side, pad := sideW(), sc(24)
	full := rect{0, 0, w, h}
	pFillRect.Call(hdc, uintptr(unsafe.Pointer(&full)), brushWin)
	pFillRect.Call(hdc, uintptr(unsafe.Pointer(&rect{0, 0, side, h})), brushSide)
	fillColor(hdc, rect{side - 1, 0, side, h}, pal.cardLine)
	pSetBkMode.Call(hdc, transparent)

	// Barre latérale
	pSelectObject.Call(hdc, fontReg)
	for i, r := range st.side {
		if i == st.page {
			fillRR(hdc, r, sc(10), pal.accent)
			pSetTextColor.Call(hdc, rgb(255, 255, 255))
		} else {
			pSetTextColor.Call(hdc, pal.menuText)
		}
		drawText(hdc, st.pages[i].name, rect{r.Left + sc(14), r.Top, r.Right - sc(8), r.Bottom}, dtLeft|dtEllipsis)
	}

	// Titre
	pSelectObject.Call(hdc, st.fontBig)
	pSetTextColor.Call(hdc, pal.menuText)
	drawText(hdc, st.pages[st.page].name, rect{side + pad, sc(22), w - pad, sc(64)}, dtLeft)

	// Cartes
	for _, g := range st.groups {
		fillRR(hdc, g, sc(12), pal.cardLine)
		fillRR(hdc, shrink(g, 1), sc(11), pal.card)
	}

	rows := st.pages[st.page].rows
	for _, sl := range st.slots {
		row := rows[sl.row]
		if row.kind == rowHeader {
			pSelectObject.Call(hdc, fontBold)
			pSetTextColor.Call(hdc, pal.menuKey)
			drawText(hdc, row.label, rect{sl.r.Left + sc(4), sl.r.Top, sl.r.Right, sl.r.Bottom}, dtLeft)
			continue
		}
		// Trait entre deux lignes d'une même carte
		if sl.row+1 < len(rows) && rows[sl.row+1].kind != rowHeader {
			fillColor(hdc, rect{sl.r.Left + sc(14), sl.r.Bottom, sl.r.Right - sc(14), sl.r.Bottom + 1}, pal.cardLine)
		}
		if row.kind != rowButton {
			pSelectObject.Call(hdc, fontReg)
			pSetTextColor.Call(hdc, pal.menuText)
			left := sl.r.Left + sc(14)
			drawText(hdc, row.label, rect{left, sl.r.Top, sl.ctl.Left - sc(12), sl.r.Bottom}, dtLeft|dtEllipsis)
		}
		switch row.kind {
		case rowToggle:
			drawToggle(hdc, sl.ctl, row.getB())
		case rowSlider:
			drawSlider(hdc, sl, row)
		case rowChoice:
			drawChoice(hdc, sl, row)
		case rowSwatch:
			drawSwatches(hdc, sl, row)
		case rowButton:
			col := pal.btn
			if st.hoverRow == sl.row {
				col = pal.btnHover
			}
			fillRR(hdc, sl.ctl, sc(9), col)
			pSelectObject.Call(hdc, fontReg)
			pSetTextColor.Call(hdc, pal.menuText)
			drawText(hdc, row.label, sl.ctl, dtCenter)
		}
	}
}

func drawToggle(hdc uintptr, r rect, on bool) {
	col := pal.ctrlOff
	if on {
		col = pal.accent
	}
	h := r.Bottom - r.Top
	fillRR(hdc, r, h, col)
	d := h - sc(4)
	kx := r.Left + sc(2)
	if on {
		kx = r.Right - sc(2) - d
	}
	fillEllipse(hdc, rect{kx, r.Top + sc(2), kx + d, r.Top + sc(2) + d}, rgb(255, 255, 255))
}

func drawSlider(hdc uintptr, sl sSlot, row sRow) {
	tr := sl.ctl
	cy := (tr.Top + tr.Bottom) / 2
	th := sc(4)
	track := rect{tr.Left, cy - th/2, tr.Right, cy + th/2 + th%2}
	fillRR(hdc, track, th, pal.ctrlOff)
	frac := float64(*row.n-row.min) / float64(row.max-row.min)
	kd := sc(18)
	kx := tr.Left + int32(frac*float64(tr.Right-tr.Left-kd))
	filled := rect{tr.Left, track.Top, kx + kd/2, track.Bottom}
	fillRR(hdc, filled, th, pal.accent)
	fillEllipse(hdc, rect{kx - 1, cy - kd/2 - 1, kx + kd + 1, cy + kd/2 + 1}, pal.cardLine)
	fillEllipse(hdc, rect{kx, cy - kd/2, kx + kd, cy + kd/2}, rgb(255, 255, 255))

	pSelectObject.Call(hdc, fontReg)
	pSetTextColor.Call(hdc, pal.menuKey)
	drawText(hdc, fmt.Sprintf("%d%s", *row.n, row.unit), sl.val, dtRight)
}

func drawChoice(hdc uintptr, sl sSlot, row sRow) {
	fillRR(hdc, sl.ctl, sc(10), pal.winBg)
	pSelectObject.Call(hdc, fontReg)
	for i, seg := range sl.segs {
		sel := row.opts[i] == *row.s
		if sel {
			fillRR(hdc, shrink(seg, sc(2)), sc(8), pal.accent)
			pSetTextColor.Call(hdc, rgb(255, 255, 255))
		} else {
			pSetTextColor.Call(hdc, pal.menuText)
		}
		drawText(hdc, row.names[i], seg, dtCenter)
	}
}

func drawSwatches(hdc uintptr, sl sSlot, row sRow) {
	for i, seg := range sl.segs {
		if row.opts[i] == *row.s {
			fillEllipse(hdc, rect{seg.Left - sc(4), seg.Top - sc(4), seg.Right + sc(4), seg.Bottom + sc(4)}, pal.menuKey)
			fillEllipse(hdc, rect{seg.Left - sc(2), seg.Top - sc(2), seg.Right + sc(2), seg.Bottom + sc(2)}, pal.card)
		}
		r, g, b := accentRGB(row.opts[i])
		fillEllipse(hdc, seg, rgb(r, g, b))
	}
}

// ---------------------------------------------------------------------------
// Actions
// ---------------------------------------------------------------------------

func setSliderFromX(row int, x int32) {
	sl := slotOf(row)
	if sl == nil {
		return
	}
	r := st.pages[st.page].rows[row]
	kd := sc(18)
	span := float64(sl.ctl.Right - sl.ctl.Left - kd)
	if span <= 0 {
		return
	}
	frac := float64(x-sl.ctl.Left-kd/2) / span
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	steps := int(frac*float64(r.max-r.min)/float64(r.step) + 0.5)
	v := r.min + steps*r.step
	if v > r.max {
		v = r.max
	}
	if v != *r.n {
		*r.n = v
		st.dragChanged = true
		settingChanged(false)
	}
}

func activateRow(row, seg int) {
	r := st.pages[st.page].rows[row]
	switch r.kind {
	case rowToggle:
		r.setB(!r.getB())
		settingChanged(true)
	case rowChoice, rowSwatch:
		if seg >= 0 && seg < len(r.opts) && *r.s != r.opts[seg] {
			*r.s = r.opts[seg]
			settingChanged(true)
		}
	case rowButton:
		r.action()
	}
	pInvalidateRect.Call(st.hwnd, 0, 0)
}

// ---------------------------------------------------------------------------
// Fenêtre
// ---------------------------------------------------------------------------

func styleSettingsWindow() {
	if st.hwnd == 0 || pDwmSetWindowAttr.Find() != nil {
		return
	}
	dark := uint32(0)
	if pal.dark {
		dark = 1
	}
	pDwmSetWindowAttr.Call(st.hwnd, 20 /*DWMWA_USE_IMMERSIVE_DARK_MODE*/, uintptr(unsafe.Pointer(&dark)), 4)
}

func settingsProc(hwnd, message, wparam, lparam uintptr) uintptr {
	switch uint32(message) {
	case wmEraseBkgnd:
		return 1

	case wmPaint:
		paintBuffered(hwnd, drawSettings)
		return 0

	case wmMouseMove:
		x, y := lparamXY(lparam)
		if st.drag >= 0 {
			setSliderFromX(st.drag, x)
			return 0
		}
		row, seg := sHitTest(x, y)
		if row != st.hoverRow || seg != st.hoverSeg {
			st.hoverRow, st.hoverSeg = row, seg
			pInvalidateRect.Call(hwnd, 0, 0)
		}
		if !st.tracking {
			tme := trackMouseEvent{Flags: tmeLeave, HwndTrack: hwnd}
			tme.Size = uint32(unsafe.Sizeof(tme))
			pTrackMouseEvent.Call(uintptr(unsafe.Pointer(&tme)))
			st.tracking = true
		}
		return 0

	case wmMouseLeave:
		st.tracking, st.hoverRow, st.hoverSeg = false, -1, -1
		pInvalidateRect.Call(hwnd, 0, 0)
		return 0

	case wmLButtonDown:
		x, y := lparamXY(lparam)
		for i, r := range st.side {
			if inRect(r, x, y) {
				st.page = i
				st.hoverRow = -1
				pInvalidateRect.Call(hwnd, 0, 0)
				return 0
			}
		}
		row, seg := sHitTest(x, y)
		st.pressRow, st.pressSeg = row, seg
		if row >= 0 && st.pages[st.page].rows[row].kind == rowSlider {
			st.drag, st.dragChanged = row, false
			pSetCapture.Call(hwnd)
			setSliderFromX(row, x)
		}
		return 0

	case wmLButtonUp:
		x, y := lparamXY(lparam)
		if st.drag >= 0 {
			st.drag = -1
			pReleaseCapture.Call()
			if st.dragChanged {
				settingChanged(true)
				// Recharge les icônes du Dock à la bonne taille une fois le curseur relâché.
				spriteCache = map[string]*dockgfx.Sprite{}
				dockSig = ""
				dockRefresh()
			}
			st.pressRow = -1
			return 0
		}
		row, seg := sHitTest(x, y)
		if row >= 0 && row == st.pressRow && seg == st.pressSeg {
			activateRow(row, seg)
		}
		st.pressRow = -1
		return 0

	case wmKeyDown:
		if wparam == vkEscape {
			pDestroyWindow.Call(hwnd)
		}
		return 0

	case wmClose:
		pDestroyWindow.Call(hwnd)
		return 0

	case wmDestroy:
		if hwnd == st.hwnd {
			st.hwnd = 0
			pDeleteObject.Call(st.fontBig)
			st.fontBig = 0
		}
		return 0
	}
	r, _, _ := pDefWindowProc.Call(hwnd, message, wparam, lparam)
	return r
}

var settingsClassOK bool

// showSettings ouvre la fenêtre de réglages (ou la ramène devant).
func showSettings() {
	if st.hwnd != 0 {
		pShowWindow.Call(st.hwnd, swRestore)
		pSetForeground.Call(st.hwnd)
		return
	}
	hInst, _, _ := pGetModuleHandle.Call(0)
	if !settingsClassOK {
		cursor, _, _ := pLoadCursor.Call(0, idcArrow)
		icon, _, _ := pLoadIcon.Call(0, idiApp)
		wc := wndClassEx{WndProc: syscall.NewCallback(settingsProc), Instance: hInst, Cursor: cursor, Icon: icon, ClassName: utf16(settingsClass)}
		wc.Size = uint32(unsafe.Sizeof(wc))
		pRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
		settingsClassOK = true
	}
	buildPages()
	st.w, st.h = sc(700), sc(540)
	st.hoverRow, st.hoverSeg, st.pressRow, st.pressSeg, st.drag = -1, -1, -1, -1, -1

	const style = wsCaption | wsSysMenu | wsMinimizeBox
	rc := rect{0, 0, st.w, st.h}
	pAdjustWindowRectEx.Call(uintptr(unsafe.Pointer(&rc)), style, 0, 0)
	ww, wh := rc.Right-rc.Left, rc.Bottom-rc.Top
	wa := workArea(0, 0)
	x := wa.Left + (wa.Right-wa.Left-ww)/2
	y := wa.Top + (wa.Bottom-wa.Top-wh)/3
	if y < wa.Top {
		y = wa.Top
	}
	hwnd, _, _ := pCreateWindowEx.Call(0, ptr(utf16(settingsClass)),
		ptr(utf16(tr("Réglages de Menu Windows", "Menu Windows Settings"))), style,
		uintptr(x), uintptr(y), uintptr(ww), uintptr(wh), 0, 0, hInst, 0)
	if hwnd == 0 {
		return
	}
	st.hwnd = hwnd
	st.fontBig = makeFont(sc(22), 700)
	styleSettingsWindow()
	pShowWindow.Call(hwnd, swShow)
	pSetForeground.Call(hwnd)
}
