package config

import (
	"strings"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	c := Default()
	c.Theme = "dark"
	c.Accent = "pink"
	c.BarHeight = 31
	c.ShowSeconds = true
	c.DockEnabled = false
	c.DockZoom = 120
	got := Parse(c.Marshal())
	if got != c {
		t.Fatalf("aller-retour différent:\n%+v\n%+v", got, c)
	}
}

func TestDefaultsWhenEmpty(t *testing.T) {
	if Parse("") != Default() {
		t.Fatal("un fichier vide doit donner les réglages par défaut")
	}
}

func TestClampAndInvalid(t *testing.T) {
	c := Parse(strings.Join([]string{
		"bar_height=999",
		"dock_size=1",
		"dock_zoom=abc",
		"theme=violet",
		"accent=RED",
		"dock_trash=maybe",
		"hotkey=non",
		"inconnu=1",
		"# commentaire",
		"ligne sans egal",
	}, "\r\n"))
	d := Default()
	if c.BarHeight != BarHeightMax {
		t.Errorf("bar_height = %d", c.BarHeight)
	}
	if c.DockSize != DockSizeMin {
		t.Errorf("dock_size = %d", c.DockSize)
	}
	if c.DockZoom != d.DockZoom {
		t.Errorf("dock_zoom invalide doit rester par défaut, a %d", c.DockZoom)
	}
	if c.Theme != d.Theme {
		t.Errorf("theme invalide doit rester par défaut, a %q", c.Theme)
	}
	if c.Accent != "red" {
		t.Errorf("accent = %q (la casse doit être ignorée)", c.Accent)
	}
	if c.DockTrash != d.DockTrash {
		t.Error("dock_trash invalide doit rester par défaut")
	}
	if c.Hotkey {
		t.Error("hotkey=non doit désactiver le raccourci")
	}
}

func TestDefaultsWithinBounds(t *testing.T) {
	d := Default()
	if d.BarHeight < BarHeightMin || d.BarHeight > BarHeightMax ||
		d.BarOpacity < BarOpacityMin || d.BarOpacity > BarOpacityMax ||
		d.DockSize < DockSizeMin || d.DockSize > DockSizeMax ||
		d.DockZoom < DockZoomMin || d.DockZoom > DockZoomMax {
		t.Fatalf("valeur par défaut hors bornes: %+v", d)
	}
}

func TestMarshalHasEveryKeyOnce(t *testing.T) {
	c := Default()
	out := c.Marshal()
	for _, f := range c.fields() {
		if n := strings.Count(out, "\n"+f.key+"="); n != 1 {
			t.Errorf("clé %q écrite %d fois", f.key, n)
		}
	}
}

func TestTaskbarBlockedAndAutoUpdateByDefault(t *testing.T) {
	d := Default()
	if !d.HideTaskbar {
		t.Error("la barre des tâches Windows doit être bloquée par défaut")
	}
	if !d.AutoUpdate {
		t.Error("la recherche de mises à jour doit être automatique par défaut")
	}
	// L'ancienne clé « hide_taskbar=false » est ignorée : le blocage reste actif.
	if c := Parse("hide_taskbar=false\n"); !c.HideTaskbar {
		t.Error("l'ancienne clé hide_taskbar ne doit plus compter")
	}
	if c := Parse("block_taskbar=false\nauto_update=false\n"); c.HideTaskbar || c.AutoUpdate {
		t.Error("block_taskbar et auto_update doivent pouvoir être désactivés")
	}
}
