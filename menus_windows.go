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
	appTitle, tip                                    string
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

func confirm(hwnd uintptr, text string) bool {
	r, _, _ := pMessageBox.Call(hwnd, ptr(utf16(text)), ptr(utf16(t.appTitle)), mbYesNo|mbIconQuest|mbTopmost)
	return r == idYes
}

func addItem(menu uintptr, id int, text string) {
	pAppendMenu.Call(menu, mfString, uintptr(id), ptr(utf16(text)))
}

func addSep(menu uintptr) { pAppendMenu.Call(menu, mfSeparator, 0, 0) }

// escapeMnemonic double les '&' pour qu'ils s'affichent tels quels.
func escapeMnemonic(s string) string { return strings.ReplaceAll(s, "&", "&&") }

// popup affiche le menu et renvoie la commande choisie (0 si aucune).
func popup(hwnd, menu uintptr, x, y int32) int {
	pSetForeground.Call(hwnd)
	cmd, _, _ := pTrackPopupMenu.Call(menu, tpmReturnCmd|tpmRightBtn|tpmNoNotify,
		uintptr(x), uintptr(y), 0, hwnd, 0)
	pPostMessage.Call(hwnd, wmNull, 0, 0)
	return int(cmd)
}

// ---------------------------------------------------------------------------
// Menu principal (symbole)
// ---------------------------------------------------------------------------

func showMainMenu(hwnd uintptr, x, y int32) {
	menu, _, _ := pCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	defer pDestroyMenu.Call(menu)

	addItem(menu, cmdAbout, t.about)
	addSep(menu)
	addItem(menu, cmdSettings, t.settings)
	addItem(menu, cmdStore, t.store)
	addSep(menu)
	addItem(menu, cmdRecent, t.recent)
	addSep(menu)
	addItem(menu, cmdForceQuit, t.forceQuit)
	addSep(menu)
	addItem(menu, cmdSleep, t.sleep)
	addItem(menu, cmdRestart, t.restart)
	addItem(menu, cmdShutdown, t.shutdown)
	addSep(menu)
	addItem(menu, cmdLock, t.lock)
	addItem(menu, cmdLogoff, t.logoff)
	addSep(menu)
	addItem(menu, cmdQuitApp, t.quit)

	if cmd := popup(hwnd, menu, x, y); cmd != 0 {
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
	sort.Slice(list, func(i, j int) bool { return strings.ToLower(list[i].name) < strings.ToLower(list[j].name) })
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
	menu, _, _ := pCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	defer pDestroyMenu.Call(menu) // détruit aussi les sous-menus

	addItem(menu, cmdAllApps, t.allApps)
	if len(apps) > 0 {
		addSep(menu)
	}

	// Un sous-menu par lettre.
	var sub uintptr
	cur := ""
	for i, a := range apps {
		k := groupKey(a.name)
		if k != cur {
			if sub != 0 {
				pAppend(menu, cur, sub)
			}
			sub, _, _ = pCreatePopupMenu.Call()
			cur = k
		}
		addItem(sub, cmdAppBase+i, escapeMnemonic(a.name))
	}
	if sub != 0 {
		pAppend(menu, cur, sub)
	}

	cmd := popup(hwnd, menu, x, y)
	switch {
	case cmd == cmdAllApps:
		openURI("shell:AppsFolder")
	case cmd >= cmdAppBase && cmd-cmdAppBase < len(apps):
		openURI(apps[cmd-cmdAppBase].path)
	}
}

// pAppend ajoute un sous-menu nommé au menu parent.
func pAppend(parent uintptr, label string, sub uintptr) {
	pAppendMenu.Call(parent, mfPopup, sub, ptr(utf16(label)))
}

// ---------------------------------------------------------------------------
// Menu contextuel d'une fenêtre de la barre
// ---------------------------------------------------------------------------

func showWindowMenu(hwnd, target uintptr, x, y int32) {
	menu, _, _ := pCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	defer pDestroyMenu.Call(menu)
	addItem(menu, cmdWinShow, t.restoreWin)
	addItem(menu, cmdWinMin, t.minimizeWin)
	addSep(menu)
	addItem(menu, cmdWinClose, t.closeWin)

	switch popup(hwnd, menu, x, y) {
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
		if confirm(hwnd, t.confirmRestart) {
			run("shutdown.exe", "/r", "/t", "0")
		}
	case cmdShutdown:
		if confirm(hwnd, t.confirmShutdown) {
			run("shutdown.exe", "/s", "/t", "0")
		}
	case cmdLock:
		pLockWorkStation.Call()
	case cmdLogoff:
		if confirm(hwnd, t.confirmLogoff) {
			run("shutdown.exe", "/l")
		}
	case cmdQuitApp:
		pPostMessage.Call(hwnd, wmClose, 0, 0)
	}
}

var _ = unsafe.Pointer(nil)
