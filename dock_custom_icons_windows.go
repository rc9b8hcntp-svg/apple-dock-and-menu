//go:build windows

package main

// Icônes personnalisées du Dock.
//
// Le dossier %APPDATA%\MenuWindows\icons contient les icônes de remplacement
// (formats .png ou .ico). Le nom du fichier est celui du programme, sans
// l'extension : notepad.png remplace l'icône de notepad.exe.
// Pour la Corbeille : trash.png (vide) et trash-full.png (pleine).
// Les changements sont pris en compte au prochain lancement du programme.

import (
	"image"
	"image/color"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"

	"menuwin/internal/dockgfx"
)

const (
	imageIcon         = 1
	lrLoadFromFile    = 0x10
	customIconMaxSize = 256
)

var pLoadImage = user32.NewProc("LoadImageW")

// iconsDir est le dossier des icônes personnalisées.
func iconsDir() string {
	return filepath.Join(os.Getenv("APPDATA"), "MenuWindows", "icons")
}

// customSprite cherche l'icône personnalisée portant ce nom (sans extension).
func customSprite(name string) *dockgfx.Sprite {
	if name == "" {
		return nil
	}
	for _, ext := range []string{".png", ".ico"} {
		p := filepath.Join(iconsDir(), name+ext)
		if !fileExists(p) {
			continue
		}
		var s *dockgfx.Sprite
		if ext == ".png" {
			s = pngSprite(p)
		} else {
			s = icoSprite(p)
		}
		if s != nil {
			return fitSprite(s)
		}
	}
	return nil
}

// customAppSprite : icône personnalisée d'un programme (d'après le nom de son fichier).
func customAppSprite(path string) *dockgfx.Sprite {
	base := filepath.Base(path)
	return customSprite(strings.TrimSuffix(base, filepath.Ext(base)))
}

func pngSprite(path string) *dockgfx.Sprite {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 || w > 4096 || h > 4096 {
		return nil
	}
	px := make([]byte, w*h*4)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := color.NRGBAModel.Convert(img.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA)
			i := (y*w + x) * 4
			px[i], px[i+1], px[i+2], px[i+3] = c.B, c.G, c.R, c.A
		}
	}
	return dockgfx.FromBGRA(w, h, px, false)
}

func icoSprite(path string) *dockgfx.Sprite {
	hicon, _, _ := pLoadImage.Call(0, ptr(utf16(path)), imageIcon, customIconMaxSize, customIconMaxSize, lrLoadFromFile)
	if hicon == 0 {
		return nil
	}
	defer pDestroyIcon.Call(hicon)
	return hiconSprite(hicon)
}
