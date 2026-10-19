//go:build windows

package main

// Réglages de Lumen : lecture et écriture de settings.ini, application
// des réglages en direct, et la fenêtre « Réglages de Lumen… ».
//
// La fenêtre est dessinée par le programme, comme les menus : une barre
// latérale (Général, Apparence, Barre, Dock) et des lignes avec des
// interrupteurs, des curseurs, des choix et des boutons. Chaque changement est
// appliqué tout de suite et enregistré dans %APPDATA%\MenuWindows\settings.ini.

import (
	"os"
	"path/filepath"
	"unsafe"

	"menuwin/internal/config"
	"menuwin/internal/dockgfx"
)

// ---------------------------------------------------------------------------
// Fichier de réglages et langue
// ---------------------------------------------------------------------------

func configFile() string {
	return filepath.Join(appDataDir(), "settings.ini")
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
		pRegisterHotKey.Call(barHwnd, 3, modControl|modAlt|modNoRepeat, 0x20 /*VK_SPACE*/)
		pRegisterHotKey.Call(barHwnd, 4, modControl|modAlt|modNoRepeat, vkEscape)
	} else {
		pUnregisterHotKey.Call(barHwnd, 1)
		pUnregisterHotKey.Call(barHwnd, 2)
		pUnregisterHotKey.Call(barHwnd, 3)
		pUnregisterHotKey.Call(barHwnd, 4)
	}
	hotkeysOn = on
}

func applyBarOpacity() {
	if barHwnd == 0 {
		return
	}
	alpha := 255 * cfg.BarOpacity / 100
	if cfg.BarBlur {
		alpha = 255 // le flou compose lui-même la transparence
	}
	pSetLayeredWindowAttributes.Call(barHwnd, 0, uintptr(alpha), lwaAlpha)
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
	if cfg.DockPlates != dockPlatesApplied {
		dockPlatesApplied = cfg.DockPlates
		spriteCache = map[string]*dockgfx.Sprite{} // les icônes sont recalculées avec ou sans plaque
		dockBD.shadowByName = nil
	}
	dockBackdropUpdate(true)
	dockSig = ""
	dockUpdateHidden()
	dockApplyAutoHide()
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
			pSetWindowText.Call(st.hwnd, ptr(utf16(tr("Réglages de Lumen", "Lumen Settings"))))
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
	if cfg.BarOpacity != old.BarOpacity || cfg.BarBlur != old.BarBlur {
		applyBarOpacity()
	}
	setHotkeys(cfg.Hotkey)
	if cfg.HideTaskbar != old.HideTaskbar {
		applyTaskbar()
	}
	if cfg.ShowDate != old.ShowDate || cfg.ShowSeconds != old.ShowSeconds || cfg.Clock != old.Clock {
		updateClock()
	}

	dockChanged := cfg.Language != old.Language || cfg.DockEnabled != old.DockEnabled ||
		cfg.DockSize != old.DockSize || cfg.DockZoom != old.DockZoom ||
		cfg.DockLabels != old.DockLabels || cfg.DockDots != old.DockDots ||
		cfg.DockRunning != old.DockRunning || cfg.DockTrash != old.DockTrash ||
		cfg.DockLaunchpad != old.DockLaunchpad || cfg.DockAutoHide != old.DockAutoHide ||
		cfg.DockGlass != old.DockGlass || cfg.DockPlates != old.DockPlates || cfg.DockPosition != old.DockPosition
	if dockChanged {
		applyDock()
	}
	applyWidgets()
	applyCorners()
	setSwitcherHook(cfg.AppSwitcher)
	applyBarAutoHide()
	capSync()
	hotTimerSync()
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
		applyWidgets()
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
	rowInfo // texte à gauche, valeur à droite (non cliquable)
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
	value          func() string
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

func rInfo(label string, value func() string) sRow {
	return sRow{kind: rowInfo, label: label, value: value}
}

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
	hotNames := []string{"—", "Mission", "Launchpad", "Spotlight", tr("Bureau", "Desktop")}
	auto := tr("Automatique", "Automatic")
	st.pages = []sPage{
		{tr("Général", "General"), []sRow{
			rChoice(tr("Langue", "Language"), &cfg.Language, config.Languages, []string{auto, "Français", "English"}),
			rToggleF(tr("Lancer à l’ouverture de session", "Open at login"), autostartEnabled, setAutostart),
			rToggle(tr("Raccourcis clavier (Ctrl+Alt+M, L, Espace et Échap)", "Keyboard shortcuts (Ctrl+Alt+M, L, Space and Esc)"), &cfg.Hotkey),
			rToggle(tr("Sélecteur d’applications (Alt+Tab façon Cmd+Tab)", "App switcher (Alt+Tab like Cmd+Tab)"), &cfg.AppSwitcher),
			rHeader(tr("Icônes et réglages", "Icons and settings")),
			rButton(tr("Ouvrir le dossier des icônes", "Open the icons folder"), openIconsFolder),
			rButton(tr("Rétablir tous les réglages…", "Reset all settings…"), resetAllSettings),
		}},
		{tr("Apparence", "Appearance"), []sRow{
			rChoice(tr("Thème", "Theme"), &cfg.Theme, config.Themes, []string{auto, tr("Clair", "Light"), tr("Sombre", "Dark")}),
			rSwatch(tr("Couleur d’accentuation", "Accent color"), &cfg.Accent),
			rSlider(tr("Opacité de la barre", "Menu bar opacity"), &cfg.BarOpacity, config.BarOpacityMin, config.BarOpacityMax, 5, " %"),
			rToggle(tr("Flou derrière la barre", "Blur behind the menu bar"), &cfg.BarBlur),
			rToggle(tr("Boutons de fenêtre façon macOS (expérimental)", "macOS-style window buttons (experimental)"), &cfg.WindowButtons),
			rToggle(tr("Widgets sur le bureau", "Desktop widgets"), &cfg.Widgets),
			rToggle(tr("Coins d’écran arrondis", "Rounded screen corners"), &cfg.ScreenCorners),
		}},
		{tr("Barre", "Menu Bar"), []sRow{
			rSlider(tr("Hauteur de la barre", "Menu bar height"), &cfg.BarHeight, config.BarHeightMin, config.BarHeightMax, 1, " px"),
			rToggle(tr("Nom de l’application active", "Active app name"), &cfg.ShowAppName),
			rToggle(tr("Menu Fichier", "File menu"), &cfg.ShowFile),
			rToggle(tr("Menu Édition", "Edit menu"), &cfg.ShowEdit),
			rToggle(tr("Menu Applications", "Applications menu"), &cfg.ShowApps),
			rToggle(tr("Menu Fenêtre", "Window menu"), &cfg.ShowWindow),
			rToggle(tr("Masquer automatiquement la barre des menus", "Automatically hide the menu bar"), &cfg.BarAutoHide),
			rToggle(tr("Bloquer la barre des tâches Windows (elle ne remonte plus)", "Block the Windows taskbar (it no longer pops up)"), &cfg.HideTaskbar),
			rHeader(tr("Horloge", "Clock")),
			rToggle(tr("Afficher la date", "Show the date"), &cfg.ShowDate),
			rToggle(tr("Afficher les secondes", "Show seconds"), &cfg.ShowSeconds),
			rChoice(tr("Format de l’heure", "Time format"), &cfg.Clock, config.ClockFormats, []string{auto, "24 h", "12 h"}),
		}},
		{tr("Icônes d’état", "Status Icons"), []sRow{
			rToggle(tr("Batterie", "Battery"), &cfg.ShowBattery),
			rToggle(tr("Pourcentage de la batterie", "Battery percentage"), &cfg.ShowBatteryPct),
			rToggle("Wi-Fi", &cfg.ShowWifi),
			rToggle("Bluetooth", &cfg.ShowBluetooth),
			rToggle(tr("Clavier (langue)", "Keyboard (input language)"), &cfg.ShowInput),
			rToggle(tr("Son", "Sound"), &cfg.ShowSound),
			rToggle(tr("Recherche", "Search"), &cfg.ShowSearch),
			rToggle(tr("Centre de contrôle", "Control Center"), &cfg.ShowControl),
		}},
		{"Dock", []sRow{
			rToggle(tr("Afficher le Dock", "Show the Dock"), &cfg.DockEnabled),
			rToggle(tr("Masquer et afficher automatiquement", "Automatically hide and show"), &cfg.DockAutoHide),
			rChoice(tr("Position à l’écran", "Position on screen"), &cfg.DockPosition, config.DockPositions, []string{tr("Bas", "Bottom"), tr("Gauche", "Left"), tr("Droite", "Right")}),
			rToggle(tr("Fond en verre dépoli", "Frosted glass background"), &cfg.DockGlass),
			rToggle(tr("Icônes en carré arrondi (style macOS)", "Rounded-square icons (macOS style)"), &cfg.DockPlates),
			rSlider(tr("Taille des icônes", "Icon size"), &cfg.DockSize, config.DockSizeMin, config.DockSizeMax, 2, " px"),
			rSlider(tr("Agrandissement", "Magnification"), &cfg.DockZoom, config.DockZoomMin, config.DockZoomMax, 5, " %"),
			rToggle(tr("Icône du Launchpad", "Launchpad icon"), &cfg.DockLaunchpad),
			rChoice("Launchpad", &cfg.LaunchpadMode, config.LaunchpadModes, []string{tr("Plein écran", "Full screen"), tr("Petit", "Small")}),
			rToggle(tr("Étiquettes au survol", "Hover labels"), &cfg.DockLabels),
			rToggle(tr("Point sous les applications ouvertes", "Dot under open apps"), &cfg.DockDots),
			rToggle(tr("Applications ouvertes non épinglées", "Open apps that aren’t pinned"), &cfg.DockRunning),
			rToggle(tr("Corbeille", "Recycle Bin"), &cfg.DockTrash),
			rButton(tr("Rétablir les applications du Dock…", "Reset the Dock apps…"), resetDockPins),
		}},
		{tr("Coins actifs", "Hot Corners"), []sRow{
			rHeader(tr("Action quand la souris reste dans un coin de l’écran", "Action when the pointer rests in a screen corner")),
			rChoice(tr("Haut gauche", "Top left"), &cfg.HotTL, config.HotActions, hotNames),
			rChoice(tr("Haut droit", "Top right"), &cfg.HotTR, config.HotActions, hotNames),
			rChoice(tr("Bas gauche", "Bottom left"), &cfg.HotBL, config.HotActions, hotNames),
			rChoice(tr("Bas droit", "Bottom right"), &cfg.HotBR, config.HotActions, hotNames),
		}},
		{updPageName(), updPageRows()},
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
	scroll      int32 // défilement du contenu, en pixels
	contentEnd  int32 // bas du contenu quand il n'est pas défilé
	hoverLights bool
	active      bool
	fontIcon    uintptr
	lights      rect
}

const settingsClass = "MenuWindowsSettings"

func inRect(r rect, x, y int32) bool { return x >= r.Left && x < r.Right && y >= r.Top && y < r.Bottom }

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

// ---------------------------------------------------------------------------
// Actions
// ---------------------------------------------------------------------------

func setSliderFromX(row int, x int32) {
	sl := slotOf(row)
	if sl == nil {
		return
	}
	r := st.pages[st.page].rows[row]
	kd := sc(20)
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
