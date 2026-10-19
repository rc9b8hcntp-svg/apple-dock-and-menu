//go:build windows

package main

// Spotlight : un champ de recherche flottant, au centre en haut de l'écran, comme
// sur macOS. On tape, les résultats apparaissent groupés (calcul, applications,
// réglages Windows, dossiers, documents, PDF, images) avec leurs icônes ; les
// flèches choisissent, Entrée ouvre, Échap efface puis ferme. Il s'ouvre avec
// l'icône de recherche de la barre ou Ctrl+Alt+Espace.

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"menuwin/internal/dockgfx"
	"menuwin/internal/launchpad"
	"menuwin/internal/spotlight"
)

const spClass = "MenuWindowsSpotlight"

type spItem struct {
	title, detail, section string
	path                   string
	kind                   spotlight.Kind
	calc, web              bool
	copyText               string
}

type spRow struct {
	header string // en-tête de section (item = -1)
	item   int
	r      rect
}

var sp struct {
	hwnd    uintptr
	prev    uintptr
	x, y    int32
	w, h    int32
	query   []rune
	items   []spItem
	rows    []spRow
	sel     int
	entries []spotlight.Entry
	sprites map[string]*dockgfx.Sprite
	listBot int32
}

var (
	spClassOK bool
	spFiles   struct {
		sync.Mutex
		list []spotlight.Entry
	}
)

var spSettings = []struct{ fr, en, uri string }{
	{"Wi-Fi", "Wi-Fi", "ms-settings:network-wifi"},
	{"Bluetooth et appareils", "Bluetooth & devices", "ms-settings:bluetooth"},
	{"Écran", "Display", "ms-settings:display"},
	{"Son", "Sound", "ms-settings:sound"},
	{"Batterie", "Battery", "ms-settings:batterysaver"},
	{"Alimentation et veille", "Power & sleep", "ms-settings:powersleep"},
	{"Notifications", "Notifications", "ms-settings:notifications"},
	{"Applications installées", "Installed apps", "ms-settings:appsfeatures"},
	{"Comptes", "Accounts", "ms-settings:yourinfo"},
	{"Date et heure", "Date & time", "ms-settings:dateandtime"},
	{"Langue et région", "Language & region", "ms-settings:regionlanguage"},
	{"Confidentialité", "Privacy", "ms-settings:privacy"},
	{"Windows Update", "Windows Update", "ms-settings:windowsupdate"},
	{"Personnalisation", "Personalization", "ms-settings:personalization"},
	{"Fond d’écran", "Wallpaper", "ms-settings:personalization-background"},
	{"Couleurs et mode sombre", "Colors & dark mode", "ms-settings:colors"},
	{"Réseau et Internet", "Network & internet", "ms-settings:network"},
	{"VPN", "VPN", "ms-settings:network-vpn"},
	{"Stockage", "Storage", "ms-settings:storagesense"},
	{"Souris", "Mouse", "ms-settings:mousetouchpad"},
	{"Clavier", "Keyboard", "ms-settings:easeofaccess-keyboard"},
	{"Imprimantes et scanners", "Printers & scanners", "ms-settings:printers"},
	{"Accessibilité", "Accessibility", "ms-settings:easeofaccess"},
	{"À propos de ce PC", "About this PC", "ms-settings:about"},
}

// startFileIndex indexe en arrière-plan les dossiers personnels (Bureau,
// Documents, Téléchargements, Images…).
func startFileIndex() {
	go func() {
		var roots []string
		for _, base := range []string{os.Getenv("USERPROFILE"), os.Getenv("OneDrive")} {
			if base == "" {
				continue
			}
			for _, d := range []string{"Desktop", "Bureau", "Documents", "Downloads", "Téléchargements", "Pictures", "Images", "Music", "Musique", "Videos", "Vidéos"} {
				p := filepath.Join(base, d)
				if st, err := os.Stat(p); err == nil && st.IsDir() {
					roots = append(roots, p)
				}
			}
		}
		list := spotlight.Walk(roots, 4, 40000)
		spFiles.Lock()
		spFiles.list = list
		spFiles.Unlock()
	}()
}

// ---------------------------------------------------------------------------
// Résultats
// ---------------------------------------------------------------------------

func spSectionTitle(k spotlight.Kind) string {
	switch k {
	case spotlight.App:
		return tr("Applications", "Applications")
	case spotlight.Setting:
		return tr("Réglages Windows", "Windows Settings")
	case spotlight.Folder:
		return tr("Dossiers", "Folders")
	case spotlight.PDF:
		return tr("Documents PDF", "PDF Documents")
	case spotlight.Image:
		return tr("Images", "Images")
	}
	return tr("Documents", "Documents")
}

func spBaseEntries() []spotlight.Entry {
	var es []spotlight.Entry
	for _, a := range scanApps() {
		es = append(es, spotlight.Entry{Name: a.name, Path: a.path, Kind: spotlight.App})
	}
	es = append(es,
		spotlight.Entry{Name: t.launchpad, Path: "builtin:launchpad", Kind: spotlight.App},
		spotlight.Entry{Name: t.prefs[:len(t.prefs)-len("…")], Path: "builtin:prefs", Kind: spotlight.App})
	for _, s := range spSettings {
		es = append(es, spotlight.Entry{Name: tr(s.fr, s.en), Path: s.uri, Kind: spotlight.Setting})
	}
	return es
}

// spBuild recalcule la liste des résultats d'après le texte tapé.
func spBuild() {
	sp.items, sp.rows, sp.sel = nil, nil, -1
	q := strings.TrimSpace(string(sp.query))
	if q != "" {
		var items []spItem
		if v, ok := spotlight.Eval(q); ok {
			s := spotlight.Format(v, langFR)
			items = append(items, spItem{title: "= " + s, detail: tr("Entrée pour copier", "Press Return to copy"),
				section: tr("Calculatrice", "Calculator"), calc: true, copyText: s})
		}
		entries := sp.entries
		spFiles.Lock()
		entries = append(append([]spotlight.Entry(nil), entries...), spFiles.list...)
		spFiles.Unlock()
		rs := spotlight.Search(entries, q)
		best := -1
		if len(items) == 0 {
			best = spotlight.Best(rs)
		}
		toItem := func(r spotlight.Result, section string) spItem {
			return spItem{title: r.Name, detail: r.Detail, section: section, path: r.Path, kind: r.Kind}
		}
		if best >= 0 {
			items = append(items, toItem(rs[best], tr("Meilleur résultat", "Top Hit")))
		}
		for i, r := range rs {
			if i != best {
				items = append(items, toItem(r, spSectionTitle(r.Kind)))
			}
		}
		items = append(items, spItem{title: tr("Rechercher « ", "Search the Web for “") + q + tr(" » sur le Web", "”"),
			section: tr("Recherche sur le Web", "Web Search"), web: true, path: "https://www.bing.com/search?q=" + url.QueryEscape(q)})
		sp.items = items
		if len(items) > 0 {
			sp.sel = 0
		}
	}
	spLayout()
}

// ---------------------------------------------------------------------------
// Mise en page
// ---------------------------------------------------------------------------

func spFieldH() int32 { return sc(60) }
func spRowH() int32   { return sc(42) }
func spHeadH() int32  { return sc(28) }

// spLayout place les lignes et fixe la hauteur de la fenêtre.
func spLayout() {
	sp.rows = sp.rows[:0]
	y := spFieldH()
	if len(sp.items) > 0 {
		y += sc(4)
		last := ""
		for i, it := range sp.items {
			if it.section != last {
				last = it.section
				sp.rows = append(sp.rows, spRow{header: it.section, item: -1, r: rect{0, y, sp.w, y + spHeadH()}})
				y += spHeadH()
			}
			sp.rows = append(sp.rows, spRow{item: i, r: rect{sc(8), y, sp.w - sc(8), y + spRowH()}})
			y += spRowH()
		}
		y += sc(8)
	}
	sp.listBot = y
	if y != sp.h && sp.hwnd != 0 {
		sp.h = y
		pSetWindowPos.Call(sp.hwnd, 0, uintptr(sp.x), uintptr(sp.y), uintptr(sp.w), uintptr(sp.h), swpNoZOrder|swpNoActivate)
		styleWindow(sp.hwnd, sp.w, sp.h)
	}
	if sp.hwnd != 0 {
		pInvalidateRect.Call(sp.hwnd, 0, 0)
	}
}

// ---------------------------------------------------------------------------
// Icônes
// ---------------------------------------------------------------------------

func spIcon(it spItem) *dockgfx.Sprite {
	if it.calc || it.web || it.kind == spotlight.Setting {
		return nil
	}
	if s, ok := sp.sprites[it.path]; ok {
		return s
	}
	var s *dockgfx.Sprite
	switch it.path {
	case "builtin:launchpad":
		s = launchpad.Icon(64)
	case "builtin:prefs":
		s = nil
	default:
		if it.kind == spotlight.App {
			s = customTextSprite(it.title)
		}
		if s == nil {
			s = spriteForPath(it.path)
		}
	}
	if s == nil {
		s = defaultSprite()
	}
	sp.sprites[it.path] = s
	return s
}

// ---------------------------------------------------------------------------
// Dessin
// ---------------------------------------------------------------------------

func drawSpotlight(hdc uintptr, w, h int32) {
	full := rect{0, 0, w, h}
	pFillRect.Call(hdc, uintptr(unsafe.Pointer(&full)), brushMenuBg)
	pSetBkMode.Call(hdc, transparent)

	// Champ de recherche : loupe, texte en grand, curseur.
	drawGlyphIn(hdc, panelFont.glyphBig, "", rect{sc(14), 0, sc(54), spFieldH()}, pal.menuKey)
	pSelectObject.Call(hdc, panelFont.big)
	area := rect{sc(62), 0, w - sc(20), spFieldH()}
	caret := area.Left
	if len(sp.query) == 0 {
		pSetTextColor.Call(hdc, pal.menuKey)
		drawText(hdc, tr("Recherche Spotlight", "Spotlight Search"), area, dtLeft|dtEllipsis)
	} else {
		q := string(sp.query)
		pSetTextColor.Call(hdc, pal.menuText)
		drawText(hdc, q, area, dtLeft|dtEllipsis)
		caret = area.Left + textWidth(hdc, q) + sc(2)
		if caret > area.Right {
			caret = area.Right
		}
	}
	bar := rect{caret, sc(15), caret + sc(1) + 1, spFieldH() - sc(15)}
	pFillRect.Call(hdc, uintptr(unsafe.Pointer(&bar)), brushGlyph)

	if len(sp.rows) == 0 {
		return
	}
	line := rect{0, spFieldH(), w, spFieldH() + 1}
	pFillRect.Call(hdc, uintptr(unsafe.Pointer(&line)), brushSep)

	for _, row := range sp.rows {
		if row.item < 0 {
			pSelectObject.Call(hdc, fontBold)
			pSetTextColor.Call(hdc, pal.menuKey)
			drawText(hdc, row.header, rect{sc(20), row.r.Top, w - sc(20), row.r.Bottom}, dtLeft|dtEllipsis)
			continue
		}
		it := sp.items[row.item]
		selected := row.item == sp.sel
		textCol, keyCol := pal.menuText, pal.menuKey
		if selected {
			fillRR(hdc, row.r, sc(12), pal.accent)
			textCol, keyCol = rgb(255, 255, 255), rgb(225, 235, 255)
		}
		icon := rect{sc(20), row.r.Top + (spRowH()-sc(30))/2, sc(20) + sc(30), row.r.Top + (spRowH()-sc(30))/2 + sc(30)}
		switch {
		case it.calc:
			glyphDisc(hdc, icon, "", rgb(255, 159, 10), rgb(255, 255, 255))
		case it.web:
			glyphDisc(hdc, icon, "", pal.accent, rgb(255, 255, 255))
			if selected {
				glyphDisc(hdc, icon, "", rgb(255, 255, 255), pal.accent)
			}
		case it.kind == spotlight.Setting || it.path == "builtin:prefs":
			glyphDisc(hdc, icon, "", rgb(142, 142, 147), rgb(255, 255, 255))
		default:
			blitSprite(hdc, spIcon(it), icon.Left, icon.Top, sc(30))
		}
		pSelectObject.Call(hdc, panelFont.row)
		right := w - sc(24)
		if it.detail != "" {
			pSelectObject.Call(hdc, panelFont.small)
			dw := textWidth(hdc, it.detail)
			if max := (w - sc(80)) / 3; dw > max {
				dw = max
			}
			pSetTextColor.Call(hdc, keyCol)
			drawText(hdc, it.detail, rect{right - dw, row.r.Top, right, row.r.Bottom}, dtRight|dtEllipsis)
			right -= dw + sc(12)
			pSelectObject.Call(hdc, panelFont.row)
		}
		pSetTextColor.Call(hdc, textCol)
		drawText(hdc, it.title, rect{sc(64), row.r.Top, right, row.r.Bottom}, dtLeft|dtEllipsis)
	}
	pSelectObject.Call(hdc, fontReg)
}

// ---------------------------------------------------------------------------
// Actions
// ---------------------------------------------------------------------------

func spOpenItem(i int) {
	if i < 0 || i >= len(sp.items) {
		return
	}
	it := sp.items[i]
	closeSpotlight(true)
	switch {
	case it.calc:
		setClipboardText(it.copyText)
	case it.path == "builtin:launchpad":
		showLaunchpad()
	case it.path == "builtin:prefs":
		showSettings()
	default:
		openURI(it.path)
	}
}

func spMove(d int) {
	n := len(sp.items)
	if n == 0 {
		return
	}
	sp.sel = (sp.sel + d + n) % n
	pInvalidateRect.Call(sp.hwnd, 0, 0)
}

func spItemAt(x, y int32) int {
	for _, row := range sp.rows {
		if row.item >= 0 && x >= row.r.Left && x < row.r.Right && y >= row.r.Top && y < row.r.Bottom {
			return row.item
		}
	}
	return -1
}

// ---------------------------------------------------------------------------
// Fenêtre
// ---------------------------------------------------------------------------

func spProc(hwnd, message, wparam, lparam uintptr) uintptr {
	if hwnd != sp.hwnd {
		r, _, _ := pDefWindowProc.Call(hwnd, message, wparam, lparam)
		return r
	}
	switch uint32(message) {
	case wmEraseBkgnd:
		return 1
	case wmPaint:
		paintBuffered(hwnd, func(mem uintptr, w, h int32) { drawSpotlight(mem, w, h) })
		return 0
	case wmActivate:
		if wparam&0xFFFF == waInactive {
			closeSpotlight(false)
		}
		return 0
	case wmMouseMove:
		x, y := lparamXY(lparam)
		if i := spItemAt(x, y); i >= 0 && i != sp.sel {
			sp.sel = i
			pInvalidateRect.Call(hwnd, 0, 0)
		}
		return 0
	case wmLButtonUp:
		x, y := lparamXY(lparam)
		if i := spItemAt(x, y); i >= 0 {
			spOpenItem(i)
		}
		return 0
	case wmChar:
		c := rune(wparam)
		switch {
		case c == 8:
			if n := len(sp.query); n > 0 {
				sp.query = sp.query[:n-1]
				spBuild()
			}
		case c >= 32 && c != 127 && !(c >= 0xD800 && c <= 0xDFFF):
			if len(sp.query) < 80 {
				sp.query = append(sp.query, c)
				spBuild()
			}
		}
		return 0
	case wmKeyDown:
		switch wparam {
		case vkEscape:
			if len(sp.query) > 0 {
				sp.query = nil
				spBuild()
			} else {
				closeSpotlight(true)
			}
		case vkReturn:
			spOpenItem(sp.sel)
		case vkDown, 0x09 /*Tab*/ :
			spMove(1)
		case vkUp:
			spMove(-1)
		}
		return 0
	case wmClose:
		closeSpotlight(true)
		return 0
	}
	r, _, _ := pDefWindowProc.Call(hwnd, message, wparam, lparam)
	return r
}

func registerSpotlightClass() {
	if spClassOK {
		return
	}
	hInst, _, _ := pGetModuleHandle.Call(0)
	cursor, _, _ := pLoadCursor.Call(0, idcArrow)
	wc := wndClassEx{WndProc: syscall.NewCallback(spProc), Instance: hInst, Cursor: cursor, ClassName: utf16(spClass)}
	wc.Size = uint32(unsafe.Sizeof(wc))
	pRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
	spClassOK = true
}

// showSpotlight ouvre Spotlight, ou le referme s'il est déjà ouvert.
func showSpotlight() {
	if sp.hwnd != 0 {
		closeSpotlight(true)
		return
	}
	if menuOpen || dialogOpen {
		return
	}
	registerSpotlightClass()
	initPanelFonts()
	hideLabel()
	if lp.hwnd != 0 {
		closeLaunchpad(false)
	}

	cur := cursorPt()
	mr := monitorRect(cur.X, cur.Y)
	sp.w = sc(680)
	if max := mr.Right - mr.Left - sc(40); sp.w > max {
		sp.w = max
	}
	sp.h = spFieldH()
	sp.x = mr.Left + (mr.Right-mr.Left-sp.w)/2
	sp.y = mr.Top + (mr.Bottom-mr.Top)/6
	sp.query, sp.items, sp.sel = nil, nil, -1
	sp.sprites = map[string]*dockgfx.Sprite{}
	sp.entries = spBaseEntries()
	spLayout()

	sp.prev, _, _ = pGetForeground.Call()
	hInst, _, _ := pGetModuleHandle.Call(0)
	hwnd, _, _ := pCreateWindowEx.Call(wsExTopmost|wsExToolwin, ptr(utf16(spClass)), ptr(utf16("Spotlight")), wsPopup,
		uintptr(sp.x), uintptr(sp.y), uintptr(sp.w), uintptr(sp.h), 0, 0, hInst, 0)
	if hwnd == 0 {
		return
	}
	sp.hwnd = hwnd
	styleWindow(hwnd, sp.w, sp.h)
	pShowWindow.Call(hwnd, swShow)
	pSetForeground.Call(hwnd)
	pSetFocus.Call(hwnd)
	invalidate(barHwnd)
}

// closeSpotlight ferme la fenêtre ; si restore est vrai, le focus revient à la
// fenêtre qui l'avait avant.
func closeSpotlight(restore bool) {
	h := sp.hwnd
	if h == 0 {
		return
	}
	sp.hwnd = 0
	pDestroyWindow.Call(h)
	sp.items, sp.rows, sp.sprites, sp.entries = nil, nil, nil, nil
	if barHwnd != 0 {
		invalidate(barHwnd)
	}
	if restore && sp.prev != 0 && sp.prev != barHwnd {
		if ok, _, _ := pIsWindow.Call(sp.prev); ok != 0 {
			pSetForeground.Call(sp.prev)
		}
	}
}
