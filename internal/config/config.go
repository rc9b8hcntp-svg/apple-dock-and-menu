// Package config contient les réglages de Lumen : valeurs par défaut,
// lecture et écriture du fichier settings.ini (une ligne « clé=valeur » par
// réglage). Il ne dépend pas de Windows et est testé partout.
package config

import (
	"strconv"
	"strings"
)

// Valeurs possibles des réglages « à choix ».
var (
	Languages      = []string{"auto", "fr", "en"}
	Themes         = []string{"auto", "light", "dark"}
	Accents        = []string{"blue", "purple", "pink", "red", "orange", "yellow", "green", "graphite"}
	ClockFormats   = []string{"auto", "24", "12"}
	LaunchpadModes = []string{"full", "small"}
	DockPositions  = []string{"bottom", "left", "right"}
	HotActions     = []string{"none", "mission", "launchpad", "spotlight", "desktop"}
)

// Config regroupe tous les réglages.
type Config struct {
	// Général
	Language   string // auto, fr, en
	Hotkey     bool   // raccourcis clavier (Ctrl+Alt+M et Ctrl+Alt+L)
	AutoUpdate bool   // cherche une mise à jour au démarrage (et toutes les quelques heures)

	// Apparence
	Theme         string // auto, light, dark
	Accent        string // couleur d'accentuation
	BarOpacity    int    // 50 à 100 (%)
	BarBlur       bool   // flou (teinté) derrière la barre
	Widgets       bool   // widgets sur le bureau (horloge, calendrier, batterie)
	ScreenCorners bool   // coins d'écran arrondis
	AppSwitcher   bool   // Alt+Tab ouvre le sélecteur d'applications façon Cmd+Tab

	// Barre
	BarHeight   int // hauteur en pixels (à 96 dpi)
	ShowAppName bool
	ShowApps    bool
	ShowFile    bool
	ShowEdit    bool
	ShowInput   bool
	ShowWindow  bool
	HideTaskbar bool // bloque la barre des tâches de Windows : cachée, elle ne remonte plus (activé par défaut)

	// Coins actifs : action quand la souris touche un coin de l'écran
	HotTL, HotTR, HotBL, HotBR string // none, mission, launchpad, spotlight, desktop

	// Icônes d'état (à droite de la barre)
	ShowBattery    bool
	ShowBatteryPct bool
	ShowWifi       bool
	ShowBluetooth  bool
	ShowSound      bool
	ShowSearch     bool
	ShowControl    bool

	// Horloge
	ShowDate    bool
	ShowSeconds bool
	Clock       string // auto, 24, 12

	// Dock
	DockEnabled   bool
	LaunchpadMode string // full (plein écran) ou small (petit, au centre)
	DockPosition  string // bottom, left ou right
	BarAutoHide   bool   // la barre du haut se masque automatiquement
	WindowButtons bool   // boutons de fenêtre rouge / jaune / vert sur les autres applications
	DockGlass     bool   // Dock en verre dépoli (décor flouté), comme macOS
	DockPlates    bool   // icônes sur une plaque en carré arrondi
	DockSize      int    // taille des icônes (px à 96 dpi)
	DockZoom      int    // taille maximale sous la souris, en % de la taille au repos
	DockLabels    bool
	DockDots      bool
	DockRunning   bool // applications ouvertes mais non épinglées
	DockTrash     bool
	DockLaunchpad bool // icône du Launchpad en première case
	DockAutoHide  bool // le Dock se cache et réapparaît en touchant le bord de l'écran
}

// Default donne les réglages d'origine.
func Default() Config {
	return Config{
		Language:   "auto",
		Hotkey:     true,
		AutoUpdate: true,

		Theme:      "dark",
		Accent:     "blue",
		BarOpacity: 100,
		BarBlur:    false,
		Widgets:    false,

		HotTL: "none", HotTR: "none", HotBL: "none", HotBR: "none",

		BarHeight:   24,
		ShowAppName: true,
		ShowApps:    true,
		ShowFile:    true,
		ShowEdit:    true,
		ShowInput:   true,
		ShowWindow:  true,
		HideTaskbar: true,

		ShowBattery:    true,
		ShowBatteryPct: true,
		ShowWifi:       true,
		ShowBluetooth:  true,
		ShowSound:      false,
		ShowSearch:     true,
		ShowControl:    true,

		ShowDate:    true,
		ShowSeconds: false,
		Clock:       "auto",

		DockEnabled:   true,
		LaunchpadMode: "full",
		DockPosition:  "bottom",
		DockGlass:     true,
		DockPlates:    false,
		DockSize:      48,
		DockZoom:      165,
		DockLabels:    true,
		DockDots:      true,
		DockRunning:   true,
		DockTrash:     true,
		DockLaunchpad: true,
	}
}

// Bornes des réglages numériques.
const (
	BarHeightMin, BarHeightMax   = 20, 40
	BarOpacityMin, BarOpacityMax = 50, 100
	DockSizeMin, DockSizeMax     = 32, 96
	DockZoomMin, DockZoomMax     = 100, 200
)

type field struct {
	key      string
	s        *string
	n        *int
	b        *bool
	min, max int
	opts     []string
}

func (c *Config) fields() []field {
	return []field{
		{key: "language", s: &c.Language, opts: Languages},
		{key: "hotkey", b: &c.Hotkey},
		{key: "auto_update", b: &c.AutoUpdate},
		{key: "theme", s: &c.Theme, opts: Themes},
		{key: "accent", s: &c.Accent, opts: Accents},
		{key: "bar_opacity", n: &c.BarOpacity, min: BarOpacityMin, max: BarOpacityMax},
		{key: "bar_blur", b: &c.BarBlur},
		{key: "widgets", b: &c.Widgets},
		{key: "screen_corners", b: &c.ScreenCorners},
		{key: "app_switcher", b: &c.AppSwitcher},
		{key: "hot_corner_top_left", s: &c.HotTL, opts: HotActions},
		{key: "hot_corner_top_right", s: &c.HotTR, opts: HotActions},
		{key: "hot_corner_bottom_left", s: &c.HotBL, opts: HotActions},
		{key: "hot_corner_bottom_right", s: &c.HotBR, opts: HotActions},
		{key: "bar_height", n: &c.BarHeight, min: BarHeightMin, max: BarHeightMax},
		{key: "show_app_name", b: &c.ShowAppName},
		{key: "show_apps_menu", b: &c.ShowApps},
		{key: "show_file_menu", b: &c.ShowFile},
		{key: "show_edit_menu", b: &c.ShowEdit},
		{key: "show_input_menu", b: &c.ShowInput},
		{key: "show_window_menu", b: &c.ShowWindow},
		{key: "block_taskbar", b: &c.HideTaskbar},
		{key: "show_battery", b: &c.ShowBattery},
		{key: "show_battery_percent", b: &c.ShowBatteryPct},
		{key: "show_wifi", b: &c.ShowWifi},
		{key: "show_bluetooth", b: &c.ShowBluetooth},
		{key: "show_sound", b: &c.ShowSound},
		{key: "show_search", b: &c.ShowSearch},
		{key: "show_control_center", b: &c.ShowControl},
		{key: "show_date", b: &c.ShowDate},
		{key: "show_seconds", b: &c.ShowSeconds},
		{key: "clock_format", s: &c.Clock, opts: ClockFormats},
		{key: "dock_enabled", b: &c.DockEnabled},
		{key: "launchpad_mode", s: &c.LaunchpadMode, opts: LaunchpadModes},
		{key: "dock_position", s: &c.DockPosition, opts: DockPositions},
		{key: "bar_autohide", b: &c.BarAutoHide},
		{key: "window_buttons", b: &c.WindowButtons},
		{key: "dock_glass", b: &c.DockGlass},
		{key: "dock_plates", b: &c.DockPlates},
		{key: "dock_size", n: &c.DockSize, min: DockSizeMin, max: DockSizeMax},
		{key: "dock_zoom", n: &c.DockZoom, min: DockZoomMin, max: DockZoomMax},
		{key: "dock_labels", b: &c.DockLabels},
		{key: "dock_dots", b: &c.DockDots},
		{key: "dock_running", b: &c.DockRunning},
		{key: "dock_trash", b: &c.DockTrash},
		{key: "dock_launchpad", b: &c.DockLaunchpad},
		{key: "dock_autohide", b: &c.DockAutoHide},
	}
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func parseBool(v string) (bool, bool) {
	switch strings.ToLower(v) {
	case "1", "true", "yes", "on", "oui":
		return true, true
	case "0", "false", "no", "off", "non":
		return false, true
	}
	return false, false
}

// Parse lit le contenu d'un fichier settings.ini. Les lignes inconnues ou
// invalides sont ignorées : le réglage garde alors sa valeur par défaut.
func Parse(text string) Config {
	c := Default()
	byKey := map[string]field{}
	for _, f := range c.fields() {
		byKey[f.key] = f
	}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line[0] == '#' || line[0] == ';' {
			continue
		}
		i := strings.IndexByte(line, '=')
		if i < 0 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(line[:i]))
		val := strings.TrimSpace(line[i+1:])
		f, ok := byKey[key]
		if !ok {
			continue
		}
		switch {
		case f.s != nil:
			if v := strings.ToLower(val); contains(f.opts, v) {
				*f.s = v
			}
		case f.n != nil:
			if v, err := strconv.Atoi(val); err == nil {
				*f.n = clamp(v, f.min, f.max)
			}
		case f.b != nil:
			if v, ok := parseBool(val); ok {
				*f.b = v
			}
		}
	}
	return c
}

// Marshal produit le contenu du fichier settings.ini.
func (c Config) Marshal() string {
	var b strings.Builder
	b.WriteString("# Lumen - réglages (modifiables aussi depuis « Réglages de Lumen… »)\n")
	for _, f := range c.fields() {
		b.WriteString(f.key)
		b.WriteByte('=')
		switch {
		case f.s != nil:
			b.WriteString(*f.s)
		case f.n != nil:
			b.WriteString(strconv.Itoa(*f.n))
		case f.b != nil:
			b.WriteString(strconv.FormatBool(*f.b))
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
