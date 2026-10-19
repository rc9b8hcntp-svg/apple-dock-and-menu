//go:build windows

package main

// Nom de l'application, dossier de données et icône.
//
// L'application s'appelait « Menu Windows » : au premier lancement, on récupère les
// réglages, le Dock et les icônes de l'ancien dossier, et le démarrage automatique.

import (
	"os"
	"path/filepath"
)

const (
	appDirName    = "Lumen"
	legacyDirName = "MenuWindows"
	legacyRunName = "MenuWindows"
)

// appDataDir : %APPDATA%\Lumen
func appDataDir() string { return filepath.Join(os.Getenv("APPDATA"), appDirName) }

// migrateLegacy reprend les données de l'ancienne version (renommage de dossier).
func migrateLegacy() {
	base := os.Getenv("APPDATA")
	if base == "" {
		return
	}
	newDir, oldDir := filepath.Join(base, appDirName), filepath.Join(base, legacyDirName)
	if _, err := os.Stat(newDir); err != nil {
		if _, err := os.Stat(oldDir); err == nil {
			_ = os.Rename(oldDir, newDir)
		}
	}
	// Démarrage automatique : l'ancienne entrée est remplacée par la nouvelle.
	var buf [1024]byte
	if _, ok := regGet(runKey, legacyRunName, 0x2 /*RRF_RT_REG_SZ*/, buf[:]); ok {
		if exe, err := os.Executable(); err == nil {
			regSetString(runKey, runName, `"`+exe+`"`)
		}
		var flag [12]byte
		if n, ok := regGet(approvedKey, legacyRunName, rrfRtBinary, flag[:]); ok && n > 0 {
			regSet(approvedKey, runName, regBinary, flag[:])
		}
		regDelete(runKey, legacyRunName)
		regDelete(approvedKey, legacyRunName)
	}
}

// appIcon charge l'icône de l'application (ressource n° 1 de l'exécutable) à la taille voulue ;
// repli : l'icône standard de Windows.
func appIcon(px int32) uintptr {
	hInst, _, _ := pGetModuleHandle.Call(0)
	if h, _, _ := pLoadImage.Call(hInst, 1, 1 /*IMAGE_ICON*/, uintptr(px), uintptr(px), 0); h != 0 {
		return h
	}
	h, _, _ := pLoadIcon.Call(0, idiApp)
	return h
}

// smallIconSize : la taille de la petite icône du système (zone de notification).
func smallIconSize() int32 {
	n, _, _ := pGetSystemMetrics.Call(49 /*SM_CXSMICON*/)
	if n < 16 {
		n = 16
	}
	return int32(n)
}
