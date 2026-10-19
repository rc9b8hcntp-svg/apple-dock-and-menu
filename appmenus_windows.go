//go:build windows

package main

// Menus Fichier et Édition, comme dans la barre de macOS. Windows ne permet pas de lire les
// menus d'une autre application : chaque entrée envoie donc à l'application active le
// raccourci clavier équivalent (Ctrl+N, Ctrl+C…). Elles sont grisées quand aucune
// application n'est active (bureau).

import "time"

// sendCtrl simule Ctrl(+Maj)+touche dans l'application active.
func sendCtrl(vk byte, shift bool) func() {
	return func() {
		if active != 0 {
			if fg, _, _ := pGetForeground.Call(); fg != active {
				pSetForeground.Call(active)
				time.Sleep(60 * time.Millisecond)
			}
		}
		const ctrl, shiftK, keyUp = 0x11, 0x10, 2
		pKeybdEvent.Call(ctrl, 0, 0, 0)
		if shift {
			pKeybdEvent.Call(shiftK, 0, 0, 0)
		}
		pKeybdEvent.Call(uintptr(vk), 0, 0, 0)
		pKeybdEvent.Call(uintptr(vk), 0, keyUp, 0)
		if shift {
			pKeybdEvent.Call(shiftK, 0, keyUp, 0)
		}
		pKeybdEvent.Call(ctrl, 0, keyUp, 0)
	}
}

func appItem(label, key string, vk byte, shift bool) mItem {
	return mItem{text: label, key: key, act: sendCtrl(vk, shift), disabled: active == 0}
}

func showFileMenu(hwnd uintptr, x, y int32) {
	items := []mItem{
		appItem(tr("Nouveau", "New"), "Ctrl+N", 'N', false),
		appItem(tr("Ouvrir…", "Open…"), "Ctrl+O", 'O', false),
		msep(),
		appItem(tr("Fermer l’onglet", "Close Tab"), "Ctrl+W", 'W', false),
		appItem(tr("Enregistrer", "Save"), "Ctrl+S", 'S', false),
		appItem(tr("Enregistrer sous…", "Save As…"), "Ctrl+Maj+S", 'S', true),
		msep(),
		appItem(tr("Imprimer…", "Print…"), "Ctrl+P", 'P', false),
	}
	trackMenu(hwnd, items, x, y)
}

func showEditMenu(hwnd uintptr, x, y int32) {
	items := []mItem{
		appItem(tr("Annuler", "Undo"), "Ctrl+Z", 'Z', false),
		appItem(tr("Rétablir", "Redo"), "Ctrl+Y", 'Y', false),
		msep(),
		appItem(tr("Couper", "Cut"), "Ctrl+X", 'X', false),
		appItem(tr("Copier", "Copy"), "Ctrl+C", 'C', false),
		appItem(tr("Coller", "Paste"), "Ctrl+V", 'V', false),
		appItem(tr("Tout sélectionner", "Select All"), "Ctrl+A", 'A', false),
		msep(),
		appItem(tr("Rechercher…", "Find…"), "Ctrl+F", 'F', false),
		msep(),
		mItem{text: tr("Émojis et symboles", "Emoji & Symbols"), key: "Win+.", act: func() { sendWinKey(0xBE) }},
	}
	trackMenu(hwnd, items, x, y)
}
