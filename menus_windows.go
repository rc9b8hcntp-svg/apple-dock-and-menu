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
)

// ---------------------------------------------------------------------------
// Textes (français / anglais selon la langue de Windows)
// ---------------------------------------------------------------------------

type labels struct {
	about, settings, store, recent, forceQuit, sleep string
	restart, shutdown, lock, logoff, quit            string
	confirmRestart, confirmShutdown, confirmLogoff   string
	apps, allApps, restoreWin, minimizeWin, closeWin string
	appTitle, tip                                    string
	cancel, confirmHint                              string
	restartBtn, shutdownBtn, logoffBtn               string
}

var fr = labels{
	about: "À propos de ce PC", settings: "Paramètres système…", store: "Microsoft Store…",
	recent: "Éléments récents", forceQuit: "Forcer à quitter…\tCtrl+Maj+Échap",
	sleep: "Mettre en veille", restart: "Redémarrer…", shutdown: "Éteindre…",
	lock: "Verrouiller l’écran\tWin+L", logoff: "Fermer la session…", quit: "Quitter Menu Windows",
	confirmRestart:  "Voulez-vous vraiment redémarrer votre PC ?",
	confirmShutdown: "Voulez-vous vraiment éteindre votre PC ?",
	confirmLogoff:   "Voulez-vous vraiment fermer votre session ?",
	apps:            "Applications", allApps: "Toutes les applications…",
	restoreWin: "Afficher", minimizeWin: "Réduire", closeWin: "Fermer",
	appTitle: "Menu Windows",
	tip:      "Menu Windows (Ctrl+Alt+M)",
	cancel:   "Annuler", confirmHint: "Les documents non enregistrés seront perdus.",
	restartBtn: "Redémarrer", shutdownBtn: "Éteindre", logoffBtn: "Fermer la session",
}

var en = labels{
	about: "About This PC", settings: "System Settings…", store: "Microsoft Store…",
	recent: "Recent Items", forceQuit: "Force Quit…\tCtrl+Shift+Esc",
	sleep: "Sleep", restart: "Restart…", shutdown: "Shut Down…",
	lock: "Lock Screen\tWin+L", logoff: "Log Out…", quit: "Quit Menu Windows",
	confirmRestart:  "Are you sure you want to restart your PC?",
	confirmShutdown: "Are you sure you want to shut down your PC?",
	confirmLogoff:   "Are you sure you want to log out?",
	apps:            "Applications", allApps: "All Apps…",
	restoreWin: "Show", minimizeWin: "Minimize", closeWin: "Close",
	appTitle: "Menu Windows",
	tip:      "Menu Windows (Ctrl+Alt+M)",
	cancel:   "Cancel", confirmHint: "Unsaved changes will be lost.",
	restartBtn: "Restart", shutdownBtn: "Shut Down", logoffBtn: "Log Out",
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
	cmdWinShow
	cmdWinMin
	cmdWinClose
)

const cmdAppBase = 1000 // les applications du menu Démarrer commencent ici

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
		mi(cmdSettings, t.settings),
		mi(cmdStore, t.store),
		msep(),
		mi(cmdRecent, t.recent),
		msep(),
		mi(cmdForceQuit, t.forceQuit),
		msep(),
		mi(cmdSleep, t.sleep),
		mi(cmdRestart, t.restart),
		mi(cmdShutdown, t.shutdown),
		msep(),
		mi(cmdLock, t.lock),
		mi(cmdLogoff, t.logoff),
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
// Menu contextuel d'une fenêtre de la barre
// ---------------------------------------------------------------------------

func showWindowMenu(hwnd, target uintptr, x, y int32) {
	items := []mItem{
		mi(cmdWinShow, t.restoreWin),
		mi(cmdWinMin, t.minimizeWin),
		msep(),
		mi(cmdWinClose, t.closeWin),
	}
	switch trackMenu(hwnd, items, x, y) {
	case cmdWinShow:
		pShowWindow.Call(target, swRestore)
		pSetForeground.Call(target)
	case cmdWinMin:
		pShowWindow.Call(target, swMinimize)
	case cmdWinClose:
		pPostMessage.Call(target, wmClose, 0, 0)
	}
}

// ---------------------------------------------------------------------------
// Actions du menu principal
// ---------------------------------------------------------------------------

func execute(hwnd uintptr, cmd int) {
	switch cmd {
	case cmdAbout:
		openURI("ms-settings:about")
	case cmdSettings:
		openURI("ms-settings:")
	case cmdStore:
		openURI("ms-windows-store:")
	case cmdRecent:
		openURI("shell:recent")
	case cmdForceQuit:
		run("taskmgr.exe")
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
