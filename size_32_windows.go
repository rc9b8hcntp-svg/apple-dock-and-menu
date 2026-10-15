//go:build windows && 386

package main

import "unsafe"

// Vérifie à la compilation que nos structures ont la taille attendue par
// Windows (32 bits). Une erreur ici veut dire qu'un champ est mal déclaré.
var (
	_ = [28 - unsafe.Sizeof(msg{})]struct{}{}
	_ = [unsafe.Sizeof(msg{}) - 28]struct{}{}
	_ = [48 - unsafe.Sizeof(wndClassEx{})]struct{}{}
	_ = [unsafe.Sizeof(wndClassEx{}) - 48]struct{}{}
	_ = [64 - unsafe.Sizeof(paintStruct{})]struct{}{}
	_ = [unsafe.Sizeof(paintStruct{}) - 64]struct{}{}
	_ = [956 - unsafe.Sizeof(notifyIconData{})]struct{}{}
	_ = [unsafe.Sizeof(notifyIconData{}) - 956]struct{}{}
	_ = [16 - unsafe.Sizeof(trackMouseEvent{})]struct{}{}
	_ = [unsafe.Sizeof(trackMouseEvent{}) - 16]struct{}{}
	_ = [36 - unsafe.Sizeof(appBarData{})]struct{}{}
	_ = [unsafe.Sizeof(appBarData{}) - 36]struct{}{}
)
