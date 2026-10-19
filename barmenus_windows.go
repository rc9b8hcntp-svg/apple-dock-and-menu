//go:build windows

package main

// Les menus des icônes d'état de la barre, comme sur macOS : batterie, Wi-Fi,
// son, recherche, centre de contrôle et horloge s'ouvrent en menus déroulants.
// Les cases à cocher et les curseurs modifient directement les réglages du
// programme (settings.ini) : la fenêtre de réglages se met à jour en même temps.

import (
	"strconv"
	"syscall"
	"unsafe"

	"menuwin/internal/config"
)

// cfgDirty : un réglage a été modifié avec un curseur ; il est enregistré à la fermeture du menu.
var cfgDirty bool

func cfgCheck(label string, p *bool) mItem {
	return mcheck(label, *p, func() {
		*p = !*p
		settingChanged(true)
	})
}

func cfgChoice(label string, p *string, v string) mItem {
	return mcheck(label, *p == v, func() {
		*p = v
		settingChanged(true)
	})
}

func cfgSlider(label string, p *int, lo, hi int, unit string) mItem {
	return mslider(label, lo, hi, unit, func() int { return *p }, func(v int) {
		*p = v
		applyConfig() // appliqué tout de suite ; enregistré à la fermeture du menu
		cfgDirty = true
	})
}

func openSettingsURI(uri string) func() { return func() { openURI(uri) } }

// ---------------------------------------------------------------------------
// Batterie
// ---------------------------------------------------------------------------

func showBatteryMenu(hwnd uintptr, x, y int32) {
	build := func(string) []mItem {
		var sp systemPowerStatus
		pGetSystemPowerStatus.Call(uintptr(unsafe.Pointer(&sp)))
		items := []mItem{}
		if battPct >= 0 {
			items = append(items, mItem{text: tr("Batterie", "Battery") + "  " + strconv.Itoa(battPct) + " %", bold: true, disabled: true})
			src := tr("Batterie", "Battery")
			if sp.ACLineStatus == 1 {
				src = tr("Adaptateur secteur", "Power Adapter")
			}
			items = append(items, mheader(tr("Source d’alimentation : ", "Power Source: ")+src))
			switch {
			case battCharging:
				items = append(items, minfo(tr("En charge", "Charging")))
			case sp.BatteryLifeTime != 0xFFFFFFFF && sp.BatteryLifeTime > 0:
				m := int(sp.BatteryLifeTime / 60)
				items = append(items, minfo(tr("Temps restant : ", "Time Remaining: ")+strconv.Itoa(m/60)+" h "+strconv.Itoa(m%60)+" min"))
			}
			items = append(items, msep())
		}
		items = append(items,
			cfgCheck(tr("Afficher le pourcentage", "Show Percentage"), &cfg.ShowBatteryPct),
			cfgCheck(tr("Afficher la batterie dans la barre", "Show Battery in Menu Bar"), &cfg.ShowBattery),
			msep(),
			mact(tr("Économiseur de batterie…", "Battery Saver…"), openSettingsURI("ms-settings:batterysaver")),
			mact(tr("Réglages de la batterie…", "Battery Settings…"), openSettingsURI("ms-settings:powersleep")),
		)
		return items
	}
	trackMenuEx(hwnd, build(""), mOpts{rebuild: build}, x, y)
}

// ---------------------------------------------------------------------------
// Wi-Fi
// ---------------------------------------------------------------------------

// toggleWifi allume ou coupe la radio ; si Windows refuse, on ouvre ses réglages.
func toggleWifi(on bool) bool {
	if !setWifiRadio(on) {
		openURI("ms-settings:network-wifi")
		return false
	}
	statusTick = 0
	updateStatus()
	invalidate(barHwnd)
	return true
}

func wifiRadioState() bool {
	if on, ok := wifiRadioOn(); ok {
		return on
	}
	return wifiState != wifiOff
}

func showWifiMenu(hwnd uintptr, x, y int32) {
	on := wifiRadioState()
	var nets []wifiNet
	if on {
		nets = wifiNetworks()
	}
	build := func(string) []mItem {
		items := []mItem{mswitch("Wi-Fi", on, func() {
			if toggleWifi(!on) {
				on = !on
				if !on {
					nets = nil
				}
			}
		})}
		if on {
			if len(nets) == 0 {
				items = append(items, msep(), minfo(tr("Aucun réseau trouvé", "No networks found")))
			}
			netItem := func(n wifiNet) mItem {
				it := mItem{text: n.ssid, checkable: true, check: n.cur}
				if !n.cur {
					it.act = func() {
						if n.known {
							run("netsh", "wlan", "connect", "name="+n.ssid)
						} else {
							openURI("ms-settings:network-wifi") // nouveau réseau : mot de passe à saisir dans Windows
						}
					}
				}
				return it
			}
			// Comme sur macOS : « Réseaux préférés » (déjà connus), puis « Autres réseaux ».
			var preferred, others []mItem
			for _, n := range nets {
				if n.known || n.cur {
					preferred = append(preferred, netItem(n))
				} else {
					others = append(others, netItem(n))
				}
			}
			if len(preferred) > 0 {
				items = append(items, msep(), mheader(tr("Réseaux préférés", "Preferred Network")))
				items = append(items, preferred...)
			}
			if len(others) > 0 {
				items = append(items, msep(), mheader(tr("Autres réseaux", "Other Networks")))
				items = append(items, others...)
			}
		}
		items = append(items, msep(),
			cfgCheck(tr("Afficher le Wi-Fi dans la barre", "Show Wi-Fi in Menu Bar"), &cfg.ShowWifi),
			mact(tr("Réglages Réseau…", "Network Settings…"), openSettingsURI("ms-settings:network-wifi")),
		)
		return items
	}
	trackMenuEx(hwnd, build(""), mOpts{rebuild: build, minW: sc(230)}, x, y)
}

// ---------------------------------------------------------------------------
// Bluetooth
// ---------------------------------------------------------------------------

func showBluetoothMenu(hwnd uintptr, x, y int32) {
	btRefresh() // relecture en arrière-plan : le prochain affichage sera à jour
	present, on, devs := btState()
	build := func(string) []mItem {
		items := []mItem{mswitch("Bluetooth", on, func() {
			on = !on
			btSetAsync(on)
		})}
		if !present {
			items = append(items, msep(), minfo(tr("Bluetooth indisponible", "Bluetooth Unavailable")))
		} else if on {
			if len(devs) == 0 {
				items = append(items, msep(), minfo(tr("Aucun appareil", "No Devices")))
			} else {
				items = append(items, msep(), mheader(tr("Appareils", "Devices")))
				for _, d := range devs {
					d := d
					label := d.name
					action := tr("Connecter", "Connect")
					if d.connected {
						action = tr("Déconnecter", "Disconnect")
					}
					if btIsPending(d.name) {
						label += tr("  (en cours…)", "  (working…)")
						action = tr("Connexion en cours…", "Working…")
					}
					sub := []mItem{mact(action, func() { btConnect(d, !d.connected) })}
					if btIsPending(d.name) {
						sub = []mItem{minfo(action)}
					}
					sub = append(sub, msep(), mact(tr("Réglages Bluetooth…", "Bluetooth Settings…"), openSettingsURI("ms-settings:bluetooth")))
					it := msub(label, sub)
					it.checkable, it.check = true, d.connected
					items = append(items, it)
				}
			}
		}
		return append(items, msep(),
			cfgCheck(tr("Afficher le Bluetooth dans la barre", "Show Bluetooth in Menu Bar"), &cfg.ShowBluetooth),
			mact(tr("Réglages Bluetooth…", "Bluetooth Settings…"), openSettingsURI("ms-settings:bluetooth")),
		)
	}
	trackMenuEx(hwnd, build(""), mOpts{rebuild: build, minW: sc(230)}, x, y)
}

// ---------------------------------------------------------------------------
// Son
// ---------------------------------------------------------------------------

func volumeItems(a *audioEP) []mItem {
	if a != nil {
		return []mItem{
			mslider(tr("Volume", "Volume"), 0, 100, " %", a.percent, a.setPercent),
			mcheck(tr("Muet", "Mute"), a.muted(), func() { a.setMuted(!a.muted()) }),
		}
	}
	key := func(label string, vk byte) mItem {
		return mItem{text: label, stay: true, act: func() { sendMediaKey(vk) }}
	}
	return []mItem{
		key(tr("Monter le volume", "Volume Up"), 0xAF),
		key(tr("Baisser le volume", "Volume Down"), 0xAE),
		key(tr("Muet", "Mute"), 0xAD),
	}
}

func showSoundMenu(hwnd uintptr, x, y int32) {
	a := openAudio()
	defer a.close()
	build := func(string) []mItem {
		items := append([]mItem{mItem{text: tr("Son", "Sound"), bold: true, disabled: true}}, volumeItems(a)...)
		return append(items, msep(),
			cfgCheck(tr("Afficher le son dans la barre", "Show Sound in Menu Bar"), &cfg.ShowSound),
			mact(tr("Réglages du son…", "Sound Settings…"), openSettingsURI("ms-settings:sound")),
		)
	}
	trackMenuEx(hwnd, build(""), mOpts{rebuild: build, minW: sc(230)}, x, y)
}

// ---------------------------------------------------------------------------
// Horloge
// ---------------------------------------------------------------------------

func longDate() string {
	var d [64]uint16
	pGetDateFormat.Call(0x0400, 0, 0, ptr(utf16("dddd d MMMM yyyy")), uintptr(unsafe.Pointer(&d[0])), 64)
	return syscall.UTF16ToString(d[:])
}

func clockItems() []mItem {
	return []mItem{
		cfgCheck(tr("Afficher la date", "Show the Date"), &cfg.ShowDate),
		cfgCheck(tr("Afficher les secondes", "Show Seconds"), &cfg.ShowSeconds),
		msep(),
		cfgChoice(tr("Heure du système", "System Time Format"), &cfg.Clock, "auto"),
		cfgChoice("24 h", &cfg.Clock, "24"),
		cfgChoice("12 h", &cfg.Clock, "12"),
	}
}

func showClockMenu(hwnd uintptr, x, y int32) {
	build := func(string) []mItem {
		items := []mItem{minfo(longDate()), msep()}
		items = append(items, clockItems()...)
		return append(items, msep(),
			mact(tr("Ouvrir le calendrier", "Open Calendar"), func() { sendWinKey('N') }),
			mact(tr("Réglages de date et d’heure…", "Date & Time Settings…"), openSettingsURI("ms-settings:dateandtime")),
		)
	}
	trackMenuEx(hwnd, build(""), mOpts{rebuild: build, minW: sc(230)}, x, y)
}

// ---------------------------------------------------------------------------
// Centre de contrôle
// ---------------------------------------------------------------------------

var (
	accentIDs = config.Accents
	accentFR  = []string{"Bleu", "Violet", "Rose", "Rouge", "Orange", "Jaune", "Vert", "Graphite"}
	accentEN  = []string{"Blue", "Purple", "Pink", "Red", "Orange", "Yellow", "Green", "Graphite"}
)

func appearanceItems() []mItem {
	items := []mItem{
		cfgChoice(tr("Automatique", "Automatic"), &cfg.Theme, "auto"),
		cfgChoice(tr("Clair", "Light"), &cfg.Theme, "light"),
		cfgChoice(tr("Sombre", "Dark"), &cfg.Theme, "dark"),
		msep(),
	}
	for i, id := range accentIDs {
		names := accentEN
		if langFR {
			names = accentFR
		}
		items = append(items, cfgChoice(names[i], &cfg.Accent, id))
	}
	return items
}

func barIconItems() []mItem {
	return []mItem{
		cfgCheck(tr("Nom de l’application", "App Name"), &cfg.ShowAppName),
		cfgCheck(tr("Menu Applications", "Applications Menu"), &cfg.ShowApps),
		cfgCheck(tr("Menu Fenêtre", "Window Menu"), &cfg.ShowWindow),
		msep(),
		cfgCheck(tr("Batterie", "Battery"), &cfg.ShowBattery),
		cfgCheck(tr("Pourcentage de la batterie", "Battery Percentage"), &cfg.ShowBatteryPct),
		cfgCheck("Wi-Fi", &cfg.ShowWifi),
		cfgCheck("Bluetooth", &cfg.ShowBluetooth),
		cfgCheck(tr("Son", "Sound"), &cfg.ShowSound),
		cfgCheck(tr("Recherche", "Search"), &cfg.ShowSearch),
		cfgCheck(tr("Centre de contrôle", "Control Center"), &cfg.ShowControl),
	}
}
