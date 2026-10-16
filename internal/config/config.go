// Package config contient les réglages de Menu Windows : valeurs par défaut,
// lecture et écriture du fichier settings.ini (une ligne « clé=valeur » par
// réglage). Il ne dépend pas de Windows et est testé partout.
package config

import (
	"strconv"
	"strings"
)

// Valeurs possibles des réglages « à choix ».
var (
	Languages    = []string{"auto", "fr", "en"}
	Themes       = []string{"auto", "light", "dark"}
	Accents      = []string{"blue", "purple", "pink", "red", "orange", "yellow", "green", "graphite"}
	ClockFormats = []string{"auto", "24", "12"}
)

// Config regroupe tous les réglages.
type Config struct {
	// Général
	Language string // auto, fr, en
	Hotkey   bool   // raccourcis clavier (Ctrl+Alt+M et Ctrl+Alt+L)

	// Apparence
	Theme      string // auto, light, dark
	Accent     string // couleur d'accentuation
	BarOpacity int    // 50 à 100 (%)

	// Barre
	BarHeight   int // hauteur en pixels (à 96 dpi)
	ShowAppName bool
	ShowApps    bool
	ShowWindow  bool

	// Icônes d'état (à droite de la barre)
	ShowBattery    bool
	ShowBatteryPct bool
	ShowWifi       bool
	ShowSound      bool
	ShowSearch     bool
	ShowControl    bool

	// Horloge
	ShowDate    bool
	ShowSeconds bool
	Clock       string // auto, 24, 12

	// Dock
	DockEnabled   bool
	DockSize      int // taille des icônes (px à 96 dpi)
	DockZoom      int // taille maximale sous la souris, en % de la taille au repos
	DockLabels    bool
	DockDots      bool
	DockRunning   bool // applications ouvertes mais non épinglées
	DockTrash     bool
	DockLaunchpad bool // icône du Launchpad en première case
}

// Default donne les réglages d'origine.
func Default() Config {
	return Config{
		Language: "auto",
		Hotkey:   true,

		Theme:      "dark",
		Accent:     "blue",
		BarOpacity: 100,

		BarHeight:   24,
		ShowAppName: true,
		ShowApps:    true,
		ShowWindow:  true,

		ShowBattery:    true,
		ShowBatteryPct: true,
		ShowWifi:       true,
		ShowSound:      false,
		ShowSearch:     true,
		ShowControl:    true,

		ShowDate:    true,
		ShowSeconds: false,
		Clock:       "auto",

		DockEnabled:   true,
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
		{key: "theme", s: &c.Theme, opts: Themes},
		{key: "accent", s: &c.Accent, opts: Accents},
		{key: "bar_opacity", n: &c.BarOpacity, min: BarOpacityMin, max: BarOpacityMax},
		{key: "bar_height", n: &c.BarHeight, min: BarHeightMin, max: BarHeightMax},
		{key: "show_app_name", b: &c.ShowAppName},
		{key: "show_apps_menu", b: &c.ShowApps},
		{key: "show_window_menu", b: &c.ShowWindow},
		{key: "show_battery", b: &c.ShowBattery},
		{key: "show_battery_percent", b: &c.ShowBatteryPct},
		{key: "show_wifi", b: &c.ShowWifi},
		{key: "show_sound", b: &c.ShowSound},
		{key: "show_search", b: &c.ShowSearch},
		{key: "show_control_center", b: &c.ShowControl},
		{key: "show_date", b: &c.ShowDate},
		{key: "show_seconds", b: &c.ShowSeconds},
		{key: "clock_format", s: &c.Clock, opts: ClockFormats},
		{key: "dock_enabled", b: &c.DockEnabled},
		{key: "dock_size", n: &c.DockSize, min: DockSizeMin, max: DockSizeMax},
		{key: "dock_zoom", n: &c.DockZoom, min: DockZoomMin, max: DockZoomMax},
		{key: "dock_labels", b: &c.DockLabels},
		{key: "dock_dots", b: &c.DockDots},
		{key: "dock_running", b: &c.DockRunning},
		{key: "dock_trash", b: &c.DockTrash},
		{key: "dock_launchpad", b: &c.DockLaunchpad},
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
	b.WriteString("# Menu Windows - réglages (modifiables aussi depuis « Réglages de Menu Windows… »)\n")
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
