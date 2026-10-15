//go:build windows

package main

// Icônes personnalisées du Dock.
//
// Le dossier %APPDATA%\MenuWindows\icons contient les icônes de remplacement
// (formats .png ou .ico). Le nom du fichier est celui du programme, sans
// l'extension : notepad.png remplace l'icône de notepad.exe.
// Pour la Corbeille : trash.png (vide) et trash-full.png (pleine).
// Les changements sont pris en compte au prochain lancement du programme.
//
// Détection automatique : l'icône n'a pas besoin de porter exactement le nom du
// .exe. On la retrouve aussi d'après les MOTS du nom du programme, de sa
// description (« Google Chrome ») ou du titre de sa fenêtre : chrome.png, ou
// google-chrome.png, remplace l'icône de chrome.exe, et « bloc-notes.png »
// celle de « Bloc-notes ».

import (
	"image"
	"image/color"
	_ "image/png"
	"os"
	"path/filepath"
	"sort"
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

// ---------------------------------------------------------------------------
// Détection automatique d'après les mots
// ---------------------------------------------------------------------------

// userIcon est une icône personnalisée trouvée dans le dossier, avec ses mots.
type userIcon struct {
	file  string   // chemin complet
	name  string   // nom sans extension, normalisé
	words []string // mots du nom
}

var (
	userIcons       []userIcon
	userIconsLoaded bool
)

var accentFold = strings.NewReplacer(
	"à", "a", "â", "a", "ä", "a", "á", "a", "ã", "a", "å", "a",
	"ç", "c",
	"é", "e", "è", "e", "ê", "e", "ë", "e",
	"î", "i", "ï", "i", "í", "i", "ì", "i",
	"ñ", "n",
	"ô", "o", "ö", "o", "ó", "o", "ò", "o", "õ", "o", "œ", "oe",
	"ù", "u", "û", "u", "ü", "u", "ú", "u",
	"ÿ", "y", "ý", "y",
)

// splitWords met en minuscules, retire les accents et coupe sur tout ce qui
// n'est pas une lettre ou un chiffre : « Bloc-notes_2 » -> [bloc notes 2].
func splitWords(s string) []string {
	s = accentFold.Replace(strings.ToLower(s))
	return strings.FieldsFunc(s, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9')
	})
}

// noiseWords sont ignorés : ils ne désignent pas un programme en particulier.
var noiseWords = map[string]bool{
	"exe": true, "app": true, "the": true, "le": true, "la": true, "les": true,
	"de": true, "du": true, "des": true, "microsoft": true, "windows": true,
}

// loadUserIcons lit une fois le dossier des icônes.
func loadUserIcons() {
	if userIconsLoaded {
		return
	}
	userIconsLoaded = true
	entries, err := os.ReadDir(iconsDir())
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if ext != ".png" && ext != ".ico" {
			continue
		}
		base := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
		words := splitWords(base)
		if len(words) == 0 || base == "trash" || base == "trash-full" {
			continue // réservées à la Corbeille
		}
		userIcons = append(userIcons, userIcon{
			file:  filepath.Join(iconsDir(), e.Name()),
			name:  strings.Join(words, " "),
			words: words,
		})
	}
	// Les noms les plus précis (le plus de mots, puis le plus longs) passent d'abord.
	sort.SliceStable(userIcons, func(i, j int) bool {
		if len(userIcons[i].words) != len(userIcons[j].words) {
			return len(userIcons[i].words) > len(userIcons[j].words)
		}
		return len(userIcons[i].name) > len(userIcons[j].name)
	})
}

// wordMatch : le mot de l'icône correspond-il à un mot du programme ?
// Égalité exacte, ou préfixe d'au moins 4 lettres (« calc » ~ « calculatrice »).
func wordMatch(iconWord string, progWords []string) bool {
	for _, w := range progWords {
		if w == iconWord {
			return true
		}
		if len(iconWord) >= 4 && strings.HasPrefix(w, iconWord) {
			return true
		}
	}
	return false
}

// findUserIcon cherche, parmi les icônes du dossier, celle qui correspond le
// mieux aux textes donnés (nom du .exe, description, titre de fenêtre…).
func findUserIcon(texts ...string) string {
	loadUserIcons()
	if len(userIcons) == 0 {
		return ""
	}
	var all []string
	var joined []string
	for _, t := range texts {
		ws := splitWords(t)
		if len(ws) == 0 {
			continue
		}
		joined = append(joined, strings.Join(ws, " "))
		for _, w := range ws {
			if !noiseWords[w] {
				all = append(all, w)
			}
		}
	}
	// 1) nom identique à l'un des textes (mots dans le même ordre)
	for _, ic := range userIcons {
		for _, j := range joined {
			if ic.name == j {
				return ic.file
			}
		}
	}
	// 2) tous les mots de l'icône se retrouvent dans les textes
	for _, ic := range userIcons {
		ok, useful := true, false
		for _, w := range ic.words {
			if !wordMatch(w, all) {
				ok = false
				break
			}
			if !noiseWords[w] {
				useful = true
			}
		}
		if ok && useful {
			return ic.file
		}
	}
	return ""
}

// loadIconFile lit une image .png ou .ico.
func loadIconFile(p string) *dockgfx.Sprite {
	var s *dockgfx.Sprite
	if strings.EqualFold(filepath.Ext(p), ".png") {
		s = pngSprite(p)
	} else {
		s = icoSprite(p)
	}
	if s != nil {
		return fitSprite(s)
	}
	return nil
}

// customAppSprite : icône personnalisée d'un programme. On essaie d'abord le nom
// exact du fichier (notepad.png), puis la détection d'après les mots du nom du
// programme, de sa description (label) et, au besoin, du titre de sa fenêtre.
func customAppSprite(path, label string) *dockgfx.Sprite {
	base := filepath.Base(path)
	if s := customSprite(strings.TrimSuffix(base, filepath.Ext(base))); s != nil {
		return s
	}
	if f := findUserIcon(base, label); f != "" {
		return loadIconFile(f)
	}
	return nil
}

// customTextSprite : détection d'après un simple texte (titre d'une fenêtre du Store).
func customTextSprite(text string) *dockgfx.Sprite {
	if f := findUserIcon(text); f != "" {
		return loadIconFile(f)
	}
	return nil
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
