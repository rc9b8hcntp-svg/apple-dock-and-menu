//go:build windows

package main

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"unicode"
	"unsafe"
)

// ---------------------------------------------------------------------------
// Textes (français / anglais selon la langue de Windows)
// ---------------------------------------------------------------------------

type labels struct {
	about, settings, store, recent, forceQuit, sleep string
	restart, shutdown, lock, logoff, quit            string
	confirmRestart, confirmShutdown, confirmLogoff   string
	apps, allApps, restoreWin, minimizeWin, closeWin string
	appTitle, tip, desktop                           string
	cancel, confirmHint                              string
	restartBtn, shutdownBtn, logoffBtn               string
	open, pin, unpin, bin, openBin, emptyBin         string
	confirmEmpty, emptyHint, emptyBtn                string
	launchpad, search, noResult, prefs               string
}

var fr = labels{
	about: "À propos de ce PC", settings: "Paramètres système…", store: "Microsoft Store…",
	recent: "Éléments récents", forceQuit: "Forcer à quitter…\tCtrl+Alt+Échap",
	sleep: "Mettre en veille", restart: "Redémarrer…", shutdown: "Éteindre…",
	lock: "Verrouiller l’écran\tWin+L", logoff: "Fermer la session…", quit: "Quitter Lumen",
	confirmRestart:  "Voulez-vous vraiment redémarrer votre PC ?",
	confirmShutdown: "Voulez-vous vraiment éteindre votre PC ?",
	confirmLogoff:   "Voulez-vous vraiment fermer votre session ?",
	apps:            "Applications", allApps: "Toutes les applications…",
	restoreWin: "Afficher", minimizeWin: "Réduire", closeWin: "Fermer",
	appTitle: "Lumen",
	tip:      "Lumen (Ctrl+Alt+M)",
	desktop:  "Bureau",
	cancel:   "Annuler", confirmHint: "Les documents non enregistrés seront perdus.",
	restartBtn: "Redémarrer", shutdownBtn: "Éteindre", logoffBtn: "Fermer la session",
	open: "Ouvrir", pin: "Garder dans le Dock", unpin: "Retirer du Dock", bin: "Corbeille",
	openBin: "Ouvrir la Corbeille", emptyBin: "Vider la Corbeille…",
	confirmEmpty: "Vider la Corbeille ?", emptyHint: "Les éléments seront supprimés définitivement.", emptyBtn: "Vider",
	launchpad: "Launchpad", search: "Rechercher", noResult: "Aucun résultat", prefs: "Réglages de Lumen…",
}

var en = labels{
	about: "About This PC", settings: "System Settings…", store: "Microsoft Store…",
	recent: "Recent Items", forceQuit: "Force Quit…\tCtrl+Alt+Esc",
	sleep: "Sleep", restart: "Restart…", shutdown: "Shut Down…",
	lock: "Lock Screen\tWin+L", logoff: "Log Out…", quit: "Quit Lumen",
	confirmRestart:  "Are you sure you want to restart your PC?",
	confirmShutdown: "Are you sure you want to shut down your PC?",
	confirmLogoff:   "Are you sure you want to log out?",
	apps:            "Applications", allApps: "All Apps…",
	restoreWin: "Show", minimizeWin: "Minimize", closeWin: "Close",
	appTitle: "Lumen",
	tip:      "Lumen (Ctrl+Alt+M)",
	desktop:  "Desktop",
	cancel:   "Cancel", confirmHint: "Unsaved changes will be lost.",
	restartBtn: "Restart", shutdownBtn: "Shut Down", logoffBtn: "Log Out",
	open: "Open", pin: "Keep in Dock", unpin: "Remove from Dock", bin: "Recycle Bin",
	openBin: "Open Recycle Bin", emptyBin: "Empty Recycle Bin…",
	confirmEmpty: "Empty the Recycle Bin?", emptyHint: "Items will be permanently deleted.", emptyBtn: "Empty",
	launchpad: "Launchpad", search: "Search", noResult: "No Results", prefs: "Lumen Settings…",
}

var t labels

// Identifiants des commandes.
const (
	cmdAbout = 1 + iota
	cmdSettings
	cmdStore
	cmdRecent
	cmdForceQuit
	cmdSleep
	cmdRestart
	cmdShutdown
	cmdLock
	cmdLogoff
	cmdQuitApp
	cmdAllApps
	cmdLaunchpad
	cmdPrefs
	cmdAppHide
	cmdAppClose
	cmdWinMin
	cmdWinZoom
	cmdWinClose
)

const (
	cmdAppBase = 1000 // les applications du menu Démarrer commencent ici
	cmdWinBase = 5000 // les fenêtres ouvertes du menu Fenêtre commencent ici
)

// ---------------------------------------------------------------------------
// Utilitaires
// ---------------------------------------------------------------------------

func openURI(uri string) {
	pShellExecute.Call(0, ptr(utf16("open")), ptr(utf16(uri)), 0, 0, 1)
}

func run(name string, args ...string) {
	c := exec.Command(name, args...)
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWin}
	_ = c.Start()
}

// ---------------------------------------------------------------------------
// Menu principal (symbole)
// ---------------------------------------------------------------------------

func showMainMenu(hwnd uintptr, x, y int32) {
	items := []mItem{
		mi(cmdAbout, t.about),
		msep(),
		mi(cmdPrefs, t.prefs),
		mi(cmdSettings, t.settings),
		mi(cmdStore, t.store),
		mi(cmdLaunchpad, t.launchpad+"\tCtrl+Alt+L"),
		msep(),
		msub(t.recent, recentItems()),
		msep(),
		mi(cmdForceQuit, t.forceQuit),
		msep(),
		mi(cmdSleep, t.sleep),
		mi(cmdRestart, t.restart),
		mi(cmdShutdown, t.shutdown),
		msep(),
		mi(cmdLock, t.lock),
		mi(cmdLogoff, logoffLabel()),
		msep(),
		mi(cmdQuitApp, t.quit),
	}
	if cmd := trackMenu(hwnd, items, x, y); cmd != 0 {
		execute(hwnd, cmd)
	}
}

// ---------------------------------------------------------------------------
// Menu Applications (raccourcis du menu Démarrer, regroupés par lettre)
// ---------------------------------------------------------------------------

type appEntry struct{ name, path string }

func scanApps() []appEntry {
	roots := []string{
		filepath.Join(os.Getenv("ProgramData"), `Microsoft\Windows\Start Menu\Programs`),
		filepath.Join(os.Getenv("APPDATA"), `Microsoft\Windows\Start Menu\Programs`),
	}
	seen := map[string]bool{}
	var list []appEntry
	for _, root := range roots {
		if root == "" || strings.HasPrefix(root, `Microsoft`) {
			continue
		}
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.EqualFold(filepath.Ext(p), ".lnk") {
				return nil
			}
			name := strings.TrimSuffix(d.Name(), filepath.Ext(d.Name()))
			low := strings.ToLower(name)
			if strings.Contains(low, "uninstall") || strings.Contains(low, "désinstall") || strings.Contains(low, "desinstall") {
				return nil
			}
			if seen[low] {
				return nil
			}
			seen[low] = true
			list = append(list, appEntry{name: name, path: p})
			return nil
		})
	}
	// Les noms qui ne commencent pas par une lettre (groupe « # ») passent en premier.
	sort.Slice(list, func(i, j int) bool {
		si, sj := groupKey(list[i].name) == "#", groupKey(list[j].name) == "#"
		if si != sj {
			return si
		}
		return strings.ToLower(list[i].name) < strings.ToLower(list[j].name)
	})
	return list
}

func groupKey(name string) string {
	for _, r := range name {
		if unicode.IsLetter(r) {
			return strings.ToUpper(string(r))
		}
		break
	}
	return "#"
}

func showAppsMenu(hwnd uintptr, x, y int32) {
	apps := scanApps()
	items := []mItem{mi(cmdAllApps, t.allApps)}
	if len(apps) > 0 {
		items = append(items, msep())
	}

	// Un sous-menu par lettre.
	var group []mItem
	cur := ""
	flush := func() {
		if len(group) > 0 {
			items = append(items, msub(cur, group))
			group = nil
		}
	}
	for i, a := range apps {
		if k := groupKey(a.name); k != cur {
			flush()
			cur = k
		}
		group = append(group, mi(cmdAppBase+i, a.name))
	}
	flush()

	cmd := trackMenu(hwnd, items, x, y)
	switch {
	case cmd == cmdAllApps:
		openURI("shell:AppsFolder")
	case cmd >= cmdAppBase && cmd-cmdAppBase < len(apps):
		openURI(apps[cmd-cmdAppBase].path)
	}
}

// ---------------------------------------------------------------------------
// Actions du menu principal
// ---------------------------------------------------------------------------

func execute(hwnd uintptr, cmd int) {
	switch cmd {
	case cmdAbout:
		showAbout()
	case cmdSettings:
		openURI("ms-settings:")
	case cmdStore:
		openURI("ms-windows-store:")
	case cmdLaunchpad:
		showLaunchpad()
	case cmdPrefs:
		showSettings()
	case cmdRecent:
		openURI("shell:recent")
	case cmdForceQuit:
		showForceQuit()
	case cmdSleep:
		// Veille (et non hibernation) : hibernate=false, force=false.
		pSetSuspendState.Call(0, 0, 0)
	case cmdRestart:
		if confirm(hwnd, t.confirmRestart, t.restartBtn) {
			run("shutdown.exe", "/r", "/t", "0")
		}
	case cmdShutdown:
		if confirm(hwnd, t.confirmShutdown, t.shutdownBtn) {
			run("shutdown.exe", "/s", "/t", "0")
		}
	case cmdLock:
		pLockWorkStation.Call()
	case cmdLogoff:
		if confirm(hwnd, t.confirmLogoff, t.logoffBtn) {
			run("shutdown.exe", "/l")
		}
	case cmdQuitApp:
		pPostMessage.Call(hwnd, wmClose, 0, 0)
	}
}

// ---------------------------------------------------------------------------
// Menu de l'application active (comme le menu « Finder » de macOS)
// ---------------------------------------------------------------------------

// activeAppWindows donne les fenêtres de l'application active (même programme).
func activeAppWindows() []uintptr {
	if active == 0 {
		return nil
	}
	p := cachedProcPath(active)
	if p == "" || strings.EqualFold(filepath.Base(p), "ApplicationFrameHost.exe") {
		return []uintptr{active} // appli du Store : une fenêtre = une appli
	}
	var out []uintptr
	for _, w := range wins {
		if w.hwnd == active || cachedProcPath(w.hwnd) == p {
			out = append(out, w.hwnd)
		}
	}
	return out
}

func showAppMenu(hwnd uintptr, x, y int32) {
	targets := activeAppWindows()
	exe := cachedProcPath(active)
	about := mact(tr("À propos de ", "About ")+activeName, func() {
		if exe != "" {
			pSHObjectProperties.Call(0, 2 /*SHOP_FILEPATH*/, ptr(utf16(exe)), 0)
		}
	})
	about.disabled = exe == ""
	isTarget := func(h uintptr) bool {
		for _, t := range targets {
			if t == h {
				return true
			}
		}
		return false
	}
	items := []mItem{
		about,
		msep(),
		mact(tr("Masquer ", "Hide ")+activeName, func() {
			for _, h := range targets {
				pShowWindow.Call(h, swMinimize)
			}
		}),
		mact(tr("Masquer les autres", "Hide Others"), func() {
			for _, w := range wins {
				if !isTarget(w.hwnd) {
					pShowWindow.Call(w.hwnd, swMinimize)
				}
			}
		}),
		mact(tr("Tout afficher", "Show All"), func() {
			for _, w := range wins {
				if ic, _, _ := pIsIconic.Call(w.hwnd); ic != 0 {
					pShowWindow.Call(w.hwnd, swRestore)
				}
			}
		}),
		msep(),
		mItem{text: tr("Quitter ", "Quit ") + activeName, key: "Alt+F4", act: func() {
			for _, h := range targets {
				pPostMessage.Call(h, wmClose, 0, 0)
			}
		}},
	}
	trackMenu(hwnd, items, x, y)
}

// ---------------------------------------------------------------------------
// Menu Fenêtre (comme sur macOS : Réduire, Zoom, Centrer, placer à gauche / à droite,
// tout ramener au premier plan, puis la liste des fenêtres ouvertes)
// ---------------------------------------------------------------------------

func clipTitle(s string, max int) string {
	r := []rune(s)
	if len(r) > max {
		return string(r[:max-1]) + "…"
	}
	return s
}

// placeWindow déplace une fenêtre dans la zone de travail : "left", "right" ou "center".
func placeWindow(h uintptr, where string) {
	if z, _, _ := pIsZoomed.Call(h); z != 0 {
		pShowWindow.Call(h, swRestore)
	}
	var wr rect
	pGetWindowRect.Call(h, uintptr(unsafe.Pointer(&wr)))
	wa := workArea(wr.Left, wr.Top)
	w, ht := wr.Right-wr.Left, wr.Bottom-wr.Top
	x, y, nw, nh := wr.Left, wr.Top, w, ht
	switch where {
	case "left":
		x, y, nw, nh = wa.Left, wa.Top, (wa.Right-wa.Left)/2, wa.Bottom-wa.Top
	case "right":
		x, y, nw, nh = wa.Left+(wa.Right-wa.Left)/2, wa.Top, (wa.Right-wa.Left)/2, wa.Bottom-wa.Top
	case "center":
		x, y = wa.Left+(wa.Right-wa.Left-w)/2, wa.Top+(wa.Bottom-wa.Top-ht)/2
	}
	pSetWindowPos.Call(h, 0, uintptr(x), uintptr(y), uintptr(nw), uintptr(nh), swpNoZOrder)
	bringToFront(h)
}

func showWindowMenu(hwnd uintptr, x, y int32) {
	cur := active
	zoomLabel := tr("Zoom", "Zoom")
	if cur != 0 {
		if z, _, _ := pIsZoomed.Call(cur); z != 0 {
			zoomLabel = tr("Restaurer", "Restore")
		}
	}
	on := func(label, key string, f func()) mItem {
		it := mItem{text: label, key: key, act: f}
		it.disabled = cur == 0
		return it
	}
	items := []mItem{
		on(tr("Réduire", "Minimize"), "Win+↓", func() { pShowWindow.Call(cur, swMinimize) }),
		on(zoomLabel, "", func() {
			if z, _, _ := pIsZoomed.Call(cur); z != 0 {
				pShowWindow.Call(cur, swRestore)
			} else {
				pShowWindow.Call(cur, 3 /*SW_MAXIMIZE*/)
			}
		}),
		on(tr("Centrer", "Center"), "", func() { placeWindow(cur, "center") }),
		msep(),
		on(tr("Placer à gauche de l’écran", "Move Window to Left Half"), "Win+←", func() { placeWindow(cur, "left") }),
		on(tr("Placer à droite de l’écran", "Move Window to Right Half"), "Win+→", func() { placeWindow(cur, "right") }),
		msep(),
		on(tr("Fermer la fenêtre", "Close Window"), "Alt+F4", func() { pPostMessage.Call(cur, wmClose, 0, 0) }),
		msep(),
		mact(tr("Tout ramener au premier plan", "Bring All to Front"), func() {
			for i := len(wins) - 1; i >= 0; i-- {
				if wins[i].hwnd != cur {
					bringToFront(wins[i].hwnd)
				}
			}
			if cur != 0 {
				bringToFront(cur)
			}
		}),
	}
	if len(wins) > 0 {
		items = append(items, msep())
	}
	for _, w := range wins {
		h := w.hwnd
		items = append(items, mItem{text: clipTitle(w.title, 48), checkable: true, check: h == cur, act: func() { bringToFront(h) }})
	}
	trackMenu(hwnd, items, x, y)
}

// ---------------------------------------------------------------------------
// Éléments récents (sous-menu du menu principal) et nom de la session
// ---------------------------------------------------------------------------

func recentItems() []mItem {
	dir := filepath.Join(os.Getenv("APPDATA"), `Microsoft\Windows\Recent`)
	entries, _ := os.ReadDir(dir)
	type rec struct {
		name, path string
		mod        int64
	}
	var list []rec
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".lnk") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		list = append(list, rec{strings.TrimSuffix(e.Name(), filepath.Ext(e.Name())), filepath.Join(dir, e.Name()), info.ModTime().UnixNano()})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].mod > list[j].mod })
	if len(list) > 10 {
		list = list[:10]
	}
	var items []mItem
	if len(list) == 0 {
		items = append(items, minfo(tr("Aucun élément récent", "No Recent Items")))
	}
	for _, r := range list {
		path := r.path
		items = append(items, mact(clipTitle(r.name, 40), func() { openURI(path) }))
	}
	return append(items, msep(), mact(tr("Ouvrir le dossier Récents", "Open Recent Folder"), func() { openURI("shell:recent") }))
}

func logoffLabel() string {
	if u := os.Getenv("USERNAME"); u != "" {
		return tr("Fermer la session de ", "Log Out ") + u + "…"
	}
	return t.logoff
}
