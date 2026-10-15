//go:build windows && (amd64 || arm64)

package main

import "unsafe"

// Vérifie à la compilation que nos structures ont la taille attendue par
// Windows (64 bits). Une erreur ici veut dire qu'un champ est mal déclaré.
var (
	_ = [48 - unsafe.Sizeof(msg{})]struct{}{}
	_ = [unsafe.Sizeof(msg{}) - 48]struct{}{}
	_ = [80 - unsafe.Sizeof(wndClassEx{})]struct{}{}
	_ = [unsafe.Sizeof(wndClassEx{}) - 80]struct{}{}
	_ = [72 - unsafe.Sizeof(paintStruct{})]struct{}{}
	_ = [unsafe.Sizeof(paintStruct{}) - 72]struct{}{}
	_ = [976 - unsafe.Sizeof(notifyIconData{})]struct{}{}
	_ = [unsafe.Sizeof(notifyIconData{}) - 976]struct{}{}
	_ = [24 - unsafe.Sizeof(trackMouseEvent{})]struct{}{}
	_ = [unsafe.Sizeof(trackMouseEvent{}) - 24]struct{}{}
	_ = [48 - unsafe.Sizeof(appBarData{})]struct{}{}
	_ = [unsafe.Sizeof(appBarData{}) - 48]struct{}{}
)
