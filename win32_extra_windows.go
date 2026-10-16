//go:build windows

package main

// Déclarations Win32 pour la fenêtre de réglages.

const (
	wsCaption     = 0x00C00000
	wsSysMenu     = 0x00080000
	wsMinimizeBox = 0x00020000
)

var (
	pAdjustWindowRectEx = user32.NewProc("AdjustWindowRectEx")
	pSetWindowText      = user32.NewProc("SetWindowTextW")
)
