//go:build windows

package main

// Menus et boîte de confirmation dessinés par le programme.
//
// Aucun menu natif de Windows (TrackPopupMenu, MessageBox) n'est utilisé :
// chaque menu est une petite fenêtre sombre aux coins arrondis, dessinée ici
// avec le même style que la barre (surbrillance bleue, sous-menus, raccourcis
// alignés à droite, défilement des longues listes, clavier complet).

import (
	"strconv"
	"strings"
	"syscall"
	"unicode"
	"unicode/utf8"
	"unsafe"
)

// ---------------------------------------------------------------------------
// Description d'un menu
// ---------------------------------------------------------------------------

type mItem struct {
	id       int
	text     string
	key      string // raccourci affiché à droite
	sep      bool
	disabled bool
	sub      []mItem

	// Entrées « vivantes », utilisées par les menus des icônes de la barre.
	act       func() // exécuté à l'activation (une fois le menu fermé, sauf si stay)
	stay      bool   // le menu reste ouvert après l'activation (cases à cocher, choix)
	checkable bool   // une colonne de coches est réservée à gauche
	check     bool   // la coche est affichée
	field     bool   // champ de recherche (non sélectionnable), son texte est celui du menu
	header    bool   // titre de section : petit texte gras grisé, non sélectionnable
	sw        bool   // interrupteur à droite (état dans check)
	bold      bool   // texte en gras
	slider    bool
	value     func() int // curseur : valeur courante, entre lo et hi
	setValue  func(int)
	lo, hi    int
	unit      string
}

// mi crée une entrée. Un éventuel "\t" sépare le texte du raccourci.
func mi(id int, label string) mItem {
	text, key := label, ""
	if i := strings.IndexByte(label, '\t'); i >= 0 {
		text, key = label[:i], label[i+1:]
	}
	return mItem{id: id, text: text, key: key}
}

func msep() mItem { return mItem{sep: true} }

func msub(label string, sub []mItem) mItem { return mItem{text: label, sub: sub} }

func (it mItem) selectable() bool { return !it.sep && !it.disabled && !it.field }

// mact : une entrée qui exécute une action une fois le menu fermé.
func mact(label string, act func()) mItem { return mItem{text: label, act: act} }

// mcheck : une case à cocher ; le menu reste ouvert pour voir le résultat.
func mcheck(label string, on bool, act func()) mItem {
	return mItem{text: label, checkable: true, check: on, stay: true, act: act}
}

// minfo : une ligne d'information grisée.
func minfo(label string) mItem { return mItem{text: label, disabled: true} }

// mslider : un curseur horizontal (valeur entre lo et hi).
func mslider(label string, lo, hi int, unit string, get func() int, set func(int)) mItem {
	return mItem{text: label, slider: true, lo: lo, hi: hi, unit: unit, value: get, setValue: set}
}

// mheader : un titre de section (« Réseaux préférés »).
func mheader(label string) mItem { return mItem{text: label, header: true, disabled: true} }

// mswitch : une ligne en gras avec un interrupteur à droite, comme « Wi-Fi » sur macOS.
func mswitch(label string, on bool, act func()) mItem {
	return mItem{text: label, sw: true, check: on, bold: true, stay: true, act: act}
}

// mfield : le champ de recherche en haut du menu.
func mfield() mItem { return mItem{field: true} }

// ---------------------------------------------------------------------------
// Ressources et état global
// ---------------------------------------------------------------------------

const (
	menuClass   = "MenuWindowsPopup"
	dialogClass = "MenuWindowsDialog"
	menuTimerID = 7
)

var (
	brushMenuBg, brushBtn, brushBtnHover, brushAccentHi uintptr
	penBorder, penChev, penChevSel                      uintptr
	fontTitle                                           uintptr

	menuClassesOK bool
	menuOpen      bool // un menu est affiché
	dialogOpen    bool // une boîte de confirmation est affichée
	ignoreNextUp  bool // le clic qui a fermé le menu ne doit pas rouvrir quoi que ce soit

	// Remplace le texte d'avertissement de la boîte de confirmation (vide = texte par défaut).
	confirmHintOverride string
)

func makeFont(px int32, weight int) uintptr {
	f, _, _ := pCreateFont.Call(uintptr(-px), 0, 0, 0, uintptr(weight), 0, 0, 0,
		1 /*DEFAULT_CHARSET*/, 0, 0, 5 /*CLEARTYPE*/, 0, ptr(utf16(uiFace())))
	return f
}

func initMenuResources() {
	fontTitle = makeFont(sc(15), 700)
}

func freeMenuResources() {
	for _, o := range []uintptr{brushMenuBg, brushBtn, brushBtnHover, brushAccentHi, penBorder, penChev, penChevSel, fontTitle} {
		pDeleteObject.Call(o)
	}
}

func registerMenuClasses() {
	if menuClassesOK {
		return
	}
	hInst, _, _ := pGetModuleHandle.Call(0)
	cursor, _, _ := pLoadCursor.Call(0, idcArrow)
	reg := func(name string, proc uintptr) {
		wc := wndClassEx{Style: csDropShadow, WndProc: proc, Instance: hInst, Cursor: cursor, ClassName: utf16(name)}
		wc.Size = uint32(unsafe.Sizeof(wc))
		pRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc)))
	}
	reg(menuClass, syscall.NewCallback(menuProc))
	reg(dialogClass, syscall.NewCallback(dialogProc))
	menuClassesOK = true
}

// ---------------------------------------------------------------------------
// Utilitaires de mesure et de géométrie
// ---------------------------------------------------------------------------

func textWidth(hdc uintptr, s string) int32 {
	u, _ := syscall.UTF16FromString(s)
	if len(u) <= 1 {
		return 0
	}
	var sz size
	pGetTextExtent.Call(hdc, uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)-1), uintptr(unsafe.Pointer(&sz)))
	return sz.CX
}

func textHeight(hdc uintptr, s string, width int32) int32 {
	r := rect{0, 0, width, 0}
	pDrawText.Call(hdc, ptr(utf16(s)), ^uintptr(0), uintptr(unsafe.Pointer(&r)), dtCalcRect|dtWordBreak|dtNoPrefix)
	return r.Bottom
}

// workArea renvoie la zone de travail de l'écran le plus proche du point.
func workArea(x, y int32) rect {
	r := rect{x, y, x + 1, y + 1}
	hm, _, _ := pMonitorFromRect.Call(uintptr(unsafe.Pointer(&r)), 2 /*MONITOR_DEFAULTTONEAREST*/)
	if hm != 0 {
		var info monitorInfo
		info.Size = uint32(unsafe.Sizeof(info))
		if ok, _, _ := pGetMonitorInfo.Call(hm, uintptr(unsafe.Pointer(&info))); ok != 0 {
			return info.Work
		}
	}
	sw, _, _ := pGetSystemMetrics.Call(smCxScreen)
	sh, _, _ := pGetSystemMetrics.Call(smCyScreen)
	return rect{0, 0, int32(sw), int32(sh)}
}

func cursorPt() point {
	var p point
	pGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
	return p
}

func inBar(p point) bool {
	var wr rect
	pGetWindowRect.Call(barHwnd, uintptr(unsafe.Pointer(&wr)))
	return p.X >= wr.Left && p.X < wr.Right && p.Y >= wr.Top && p.Y < wr.Bottom
}

// styleWindow arrondit les coins : DWM sur Windows 11, une région sur Windows 10.
// Renvoie true quand les coins sont lissés par le système.
func styleWindow(hwnd uintptr, w, h int32) bool {
	if pDwmSetWindowAttr.Find() == nil {
		pref := uint32(2) // DWMWCP_ROUND
		if hr, _, _ := pDwmSetWindowAttr.Call(hwnd, 33 /*DWMWA_WINDOW_CORNER_PREFERENCE*/, uintptr(unsafe.Pointer(&pref)), 4); hr == 0 {
			col := uint32(pal.menuBorder)
			pDwmSetWindowAttr.Call(hwnd, 34 /*DWMWA_BORDER_COLOR*/, uintptr(unsafe.Pointer(&col)), 4)
			return true
		}
	}
	rgn, _, _ := pCreateRoundRgn.Call(0, 0, uintptr(w+1), uintptr(h+1), uintptr(sc(14)), uintptr(sc(14)))
	pSetWindowRgn.Call(hwnd, rgn, 1) // le système prend possession de la région
	return false
}

func drawChevron(hdc uintptr, cx, cy int32, dir int, pen uintptr) {
	a := sc(2)
	var pts [3][2]int32
	switch dir {
	case 0: // droite
		pts = [3][2]int32{{cx - a, cy - 2*a}, {cx + a, cy}, {cx - a, cy + 2*a}}
	case 1: // haut
		pts = [3][2]int32{{cx - 2*a, cy + a}, {cx, cy - a}, {cx + 2*a, cy + a}}
	default: // bas
		pts = [3][2]int32{{cx - 2*a, cy - a}, {cx, cy + a}, {cx + 2*a, cy - a}}
	}
	pSelectObject.Call(hdc, pen)
	pMoveToEx.Call(hdc, uintptr(pts[0][0]), uintptr(pts[0][1]), 0)
	pLineTo.Call(hdc, uintptr(pts[1][0]), uintptr(pts[1][1]))
	pLineTo.Call(hdc, uintptr(pts[2][0]), uintptr(pts[2][1]))
}

// ---------------------------------------------------------------------------
// Un niveau de menu = une fenêtre
// ---------------------------------------------------------------------------

type mLevel struct {
	hwnd       uintptr
	items      []mItem
	rects      []rect // position de chaque entrée, dans l'espace du contenu
	x, y, w, h int32  // position écran et taille
	top        int32  // marge (ou bandeau de défilement) en haut et en bas
	contentH   int32
	scroll     int32
	scrolls    bool
	keyW, subW int32
	sel        int
	parentIdx  int
	depth      int
	smooth     bool

	checkW  int32 // largeur de la colonne des coches (0 si aucune entrée à cocher)
	minW    int32 // largeur minimale (garde la largeur stable quand le menu se met à jour)
	maxH    int32 // hauteur maximale disponible
	rebuild func(query string) []mItem
	search  bool // les lettres tapées alimentent le champ de recherche
	query   string
}

var (
	mStack   []*mLevel
	mResult  int
	mDone    bool
	mLost    bool    // le menu s'est fermé parce qu'une autre fenêtre a pris le focus
	mSwitch  int     // titre de la barre vers lequel on a glissé : son menu doit s'ouvrir ensuite
	mAfter   func()  // action à lancer une fois le menu fermé
	mDragL   *mLevel // niveau dont un curseur est en train d'être déplacé
	mDragIdx int
)

const swpNoZOrder = 0x0004

func padOuter() int32 { return sc(6) }
func rowH() int32     { return sc(26) }
func sepH() int32     { return sc(9) }

// measureLevel calcule la largeur et la position des entrées.
func measureLevel(L *mLevel) {
	dc, _, _ := pGetDC.Call(0)
	old, _, _ := pSelectObject.Call(dc, fontReg)
	var maxText, maxKey int32
	anySub := false
	for _, it := range L.items {
		if it.sep || it.field {
			continue
		}
		tw := textWidth(dc, it.text)
		if it.bold || it.header {
			tw += sc(8)
		}
		if it.sw {
			tw += sc(46)
		}
		if tw > maxText {
			maxText = tw
		}
		if w := textWidth(dc, it.key); w > maxKey {
			maxKey = w
		}
		if it.sub != nil {
			anySub = true
		}
	}
	pSelectObject.Call(dc, old)
	pReleaseDC.Call(0, dc)

	L.keyW, L.subW, L.checkW = 0, 0, 0
	for _, it := range L.items {
		if it.checkable {
			L.checkW = sc(18)
		}
	}
	w := sc(12) + L.checkW + maxText + sc(14)
	if maxKey > 0 {
		L.keyW = maxKey
		w += sc(28) + maxKey
	}
	if anySub {
		L.subW = sc(16)
		w += L.subW
	}
	w += 2 * padOuter()
	if min := sc(190); w < min {
		w = min
	}
	if w < L.minW {
		w = L.minW
	}
	L.w = w

	L.rects = L.rects[:0]
	var y int32
	for _, it := range L.items {
		h := rowH()
		switch {
		case it.sep:
			h = sepH()
		case it.header:
			h = sc(24)
		case it.slider:
			h = sc(48)
		case it.field:
			h = sc(38)
		}
		L.rects = append(L.rects, rect{padOuter(), y, w - padOuter(), y + h})
		y += h
	}
	L.contentH = y
}

// fitHeight fixe la hauteur de la fenêtre ; au-delà de maxH la liste défile.
func fitHeight(L *mLevel, maxH int32) {
	L.maxH = maxH
	full := L.contentH + 2*padOuter()
	if full <= maxH {
		L.h, L.top, L.scrolls, L.scroll = full, padOuter(), false, 0
		return
	}
	L.h, L.top, L.scrolls = maxH, sc(16), true
	if ms := L.maxScroll(); L.scroll > ms {
		L.scroll = ms
	}
}

func (L *mLevel) maxScroll() int32 {
	ms := L.contentH - (L.h - 2*L.top)
	if ms < 0 {
		return 0
	}
	return ms
}

func placeRoot(L *mLevel, x, y int32) {
	wa := workArea(x, y)
	margin := sc(8)
	below := wa.Bottom - y - margin
	by := y // bas du menu quand on ouvre vers le haut : jamais sous la zone de travail
	if by > wa.Bottom {
		by = wa.Bottom
	}
	above := by - wa.Top - margin
	full := L.contentH + 2*padOuter()
	switch {
	case full <= below:
		fitHeight(L, below)
		L.y = y
	case y > (wa.Top+wa.Bottom)/2 && above > below:
		// ancré dans le bas de l'écran (icône de la zone de notification) : on ouvre vers le haut
		fitHeight(L, above)
		L.y = by - L.h
	default:
		fitHeight(L, below)
		L.y = y
	}
	L.x = x
	if L.x+L.w > wa.Right {
		L.x = wa.Right - L.w
	}
	if L.x < wa.Left {
		L.x = wa.Left
	}
}

func placeSub(L, parent *mLevel, idx int) {
	r := parent.rects[idx]
	itemTop := parent.y + parent.top + r.Top - parent.scroll
	wa := workArea(parent.x+parent.w, itemTop)
	margin := sc(8)
	fitHeight(L, wa.Bottom-wa.Top-2*margin)
	L.y = itemTop - L.top
	if L.y+L.h > wa.Bottom-margin {
		L.y = wa.Bottom - margin - L.h
	}
	if L.y < wa.Top {
		L.y = wa.Top
	}
	L.x = parent.x + parent.w - sc(4)
	if L.x+L.w > wa.Right {
		L.x = parent.x - L.w + sc(4)
	}
	if L.x < wa.Left {
		L.x = wa.Left
	}
}

func createLevelWindow(L *mLevel, root bool) bool {
	registerMenuClasses()
	hInst, _, _ := pGetModuleHandle.Call(0)
	ex := uintptr(wsExTopmost | wsExToolwin)
	if !root {
		ex |= wsExNoActive
	}
	hwnd, _, _ := pCreateWindowEx.Call(ex, ptr(utf16(menuClass)), 0, wsPopup,
		uintptr(L.x), uintptr(L.y), uintptr(L.w), uintptr(L.h), 0, 0, hInst, 0)
	if hwnd == 0 {
		return false
	}
	L.hwnd = hwnd
	L.smooth = styleWindow(hwnd, L.w, L.h)
	mStack = append(mStack, L) // avant l'affichage : le premier WM_PAINT retrouve le niveau
	if root {
		pShowWindow.Call(hwnd, swShow)
	} else {
		pShowWindow.Call(hwnd, swShowNoActivate)
	}
	return true
}

func closeFrom(n int) {
	for i := len(mStack) - 1; i >= n; i-- {
		pDestroyWindow.Call(mStack[i].hwnd)
	}
	mStack = mStack[:n]
}

func levelOf(hwnd uintptr) *mLevel {
	for _, L := range mStack {
		if L.hwnd == hwnd {
			return L
		}
	}
	return nil
}

func levelAt(p point) int {
	for i := len(mStack) - 1; i >= 0; i-- {
		L := mStack[i]
		if p.X >= L.x && p.X < L.x+L.w && p.Y >= L.y && p.Y < L.y+L.h {
			return i
		}
	}
	return -1
}

// itemAt renvoie l'entrée sous le point (coordonnées écran), ou -1.
func (L *mLevel) itemAt(p point) int {
	cy := p.Y - L.y
	cx := p.X - L.x
	if cy < L.top || cy >= L.h-L.top || cx < padOuter() || cx >= L.w-padOuter() {
		return -1
	}
	c := cy - L.top + L.scroll
	for i, r := range L.rects {
		if c >= r.Top && c < r.Bottom {
			return i
		}
	}
	return -1
}

func invalidateLevel(L *mLevel) { pInvalidateRect.Call(L.hwnd, 0, 0) }

func ensureVisible(L *mLevel, idx int) {
	if !L.scrolls || idx < 0 || idx >= len(L.rects) {
		return
	}
	view := L.h - 2*L.top
	r := L.rects[idx]
	if r.Top < L.scroll {
		L.scroll = r.Top
	} else if r.Bottom > L.scroll+view {
		L.scroll = r.Bottom - view
	}
	if L.scroll < 0 {
		L.scroll = 0
	}
	if ms := L.maxScroll(); L.scroll > ms {
		L.scroll = ms
	}
}

func scrollBy(L *mLevel, d int32) {
	if !L.scrolls {
		return
	}
	ns := L.scroll + d
	if ns < 0 {
		ns = 0
	}
	if ms := L.maxScroll(); ns > ms {
		ns = ms
	}
	if ns != L.scroll {
		L.scroll = ns
		invalidateLevel(L)
	}
}

// ---------------------------------------------------------------------------
// Dessin d'un niveau
// ---------------------------------------------------------------------------

func drawLevel(hdc uintptr, L *mLevel) {
	full := rect{0, 0, L.w, L.h}
	pFillRect.Call(hdc, uintptr(unsafe.Pointer(&full)), brushMenuBg)
	pSetBkMode.Call(hdc, transparent)
	pSelectObject.Call(hdc, fontReg)

	for i, it := range L.items {
		r := L.rects[i]
		r.Top += L.top - L.scroll
		r.Bottom += L.top - L.scroll
		if r.Bottom <= 0 || r.Top >= L.h {
			continue
		}
		if it.sep {
			mid := r.Top + (r.Bottom-r.Top)/2
			line := rect{r.Left + sc(6), mid, r.Right - sc(6), mid + 1}
			pFillRect.Call(hdc, uintptr(unsafe.Pointer(&line)), brushSep)
			continue
		}
		if it.field {
			drawSearchField(hdc, r, L.query)
			continue
		}
		selected := i == L.sel && !it.disabled
		if it.slider {
			drawSliderRow(hdc, r, it, selected)
			continue
		}
		if it.header {
			pSelectObject.Call(hdc, fontBold)
			pSetTextColor.Call(hdc, pal.menuKey)
			drawText(hdc, it.text, rect{r.Left + sc(12), r.Top + sc(3), r.Right - sc(12), r.Bottom}, dtLeft|dtEllipsis)
			pSelectObject.Call(hdc, fontReg)
			continue
		}
		if selected {
			fillRR(hdc, r, sc(9), pal.accent)
		}
		textCol, keyCol := pal.menuText, pal.menuKey
		if selected {
			textCol, keyCol = rgb(255, 255, 255), rgb(225, 235, 255)
		} else if it.disabled {
			textCol, keyCol = pal.menuOff, pal.menuOff
		}

		right := r.Right - sc(12) - L.subW
		if it.key != "" {
			pSetTextColor.Call(hdc, keyCol)
			drawText(hdc, it.key, rect{r.Left, r.Top, right, r.Bottom}, dtRight)
		}
		textRight := r.Right - sc(12) - L.subW
		if it.sw {
			textRight -= sc(46)
		}
		if L.keyW > 0 {
			textRight -= L.keyW + sc(28)
		}
		if it.checkable && it.check {
			drawIconGlyph(hdc, "\uE73E", rect{r.Left + sc(2), r.Top, r.Left + sc(24), r.Bottom}, textCol)
		}
		pSetTextColor.Call(hdc, textCol)
		if it.bold {
			pSelectObject.Call(hdc, fontBold)
		}
		drawText(hdc, it.text, rect{r.Left + sc(12) + L.checkW, r.Top, textRight, r.Bottom}, dtLeft|dtEllipsis)
		pSelectObject.Call(hdc, fontReg)
		if it.sw {
			drawSwitch(hdc, rect{r.Right - sc(12) - sc(34), r.Top + (r.Bottom-r.Top-sc(20))/2, r.Right - sc(12), r.Top + (r.Bottom-r.Top-sc(20))/2 + sc(20)}, it.check, selected)
		}

		if it.sub != nil {
			pen := penChev
			if selected {
				pen = penChevSel
			}
			drawChevron(hdc, r.Right-sc(14), r.Top+(r.Bottom-r.Top)/2, 0, pen)
		}
	}

	if L.scrolls {
		topStrip := rect{0, 0, L.w, L.top}
		botStrip := rect{0, L.h - L.top, L.w, L.h}
		pFillRect.Call(hdc, uintptr(unsafe.Pointer(&topStrip)), brushMenuBg)
		pFillRect.Call(hdc, uintptr(unsafe.Pointer(&botStrip)), brushMenuBg)
		if L.scroll > 0 {
			drawChevron(hdc, L.w/2, L.top/2, 1, penChev)
		}
		if L.scroll < L.maxScroll() {
			drawChevron(hdc, L.w/2, L.h-L.top/2, 2, penChev)
		}
	}

	if !L.smooth {
		pSelectObject.Call(hdc, penBorder)
		nb, _, _ := pGetStockObject.Call(5 /*NULL_BRUSH*/)
		pSelectObject.Call(hdc, nb)
		pRoundRect.Call(hdc, 0, 0, uintptr(L.w), uintptr(L.h), uintptr(sc(14)), uintptr(sc(14)))
	}
}

// drawSwitch dessine un interrupteur (piste arrondie et bouton rond).
func drawSwitch(hdc uintptr, r rect, on, selected bool) {
	track := blendColor(pal.menuBg, pal.menuText, 0.30)
	knob := rgb(255, 255, 255)
	switch {
	case on && selected:
		track, knob = rgb(255, 255, 255), pal.accent
	case on:
		track = pal.accent
	case selected:
		track = rgb(120, 170, 255)
	}
	fillRR(hdc, r, r.Bottom-r.Top, track)
	d := r.Bottom - r.Top - sc(4)
	x := r.Left + sc(2)
	if on {
		x = r.Right - sc(2) - d
	}
	fillEllipse(hdc, rect{x, r.Top + sc(2), x + d, r.Top + sc(2) + d}, knob)
}

// drawSearchField dessine le champ de recherche d'un menu (loupe, texte, curseur).
func drawSearchField(hdc uintptr, r rect, query string) {
	box := rect{r.Left + sc(4), r.Top + sc(4), r.Right - sc(4), r.Bottom - sc(4)}
	fillRR(hdc, box, sc(10), blendColor(pal.menuBg, pal.menuText, 0.10))
	drawIconGlyph(hdc, "\uE721", rect{box.Left + sc(4), box.Top, box.Left + sc(28), box.Bottom}, pal.menuKey)
	pSelectObject.Call(hdc, fontReg)
	tx := box.Left + sc(32)
	area := rect{tx, box.Top, box.Right - sc(10), box.Bottom}
	caret := tx
	if query == "" {
		pSetTextColor.Call(hdc, pal.menuKey)
		drawText(hdc, tr("Rechercher une application", "Search for an app"), area, dtLeft|dtEllipsis)
	} else {
		pSetTextColor.Call(hdc, pal.menuText)
		drawText(hdc, query, area, dtLeft|dtEllipsis)
		caret = tx + textWidth(hdc, query) + sc(1)
		if caret > area.Right {
			caret = area.Right
		}
	}
	bar := rect{caret, box.Top + sc(7), caret + sc(1) + 1, box.Bottom - sc(7)}
	fillColor(hdc, bar, pal.menuText)
}

// sliderSpan donne le début et la fin de la rainure d'un curseur, en coordonnées écran.
func sliderSpan(L *mLevel) (int32, int32) {
	return L.x + padOuter() + sc(12), L.x + L.w - padOuter() - sc(12)
}

// drawSliderRow dessine une ligne « libellé, valeur, rainure et bouton ».
func drawSliderRow(hdc uintptr, r rect, it mItem, selected bool) {
	if selected {
		fillRR(hdc, r, sc(9), blendColor(pal.menuBg, pal.menuText, 0.08))
	}
	v := it.value()
	if v < it.lo {
		v = it.lo
	}
	if v > it.hi {
		v = it.hi
	}
	pSelectObject.Call(hdc, fontReg)
	pSetTextColor.Call(hdc, pal.menuText)
	drawText(hdc, it.text, rect{r.Left + sc(12), r.Top + sc(3), r.Right - sc(70), r.Top + sc(27)}, dtLeft|dtEllipsis)
	pSetTextColor.Call(hdc, pal.menuKey)
	drawText(hdc, strconv.Itoa(v)+it.unit, rect{r.Left, r.Top + sc(3), r.Right - sc(12), r.Top + sc(27)}, dtRight)

	x0, x1 := r.Left+sc(12), r.Right-sc(12)
	cy := r.Bottom - sc(14)
	fillRR(hdc, rect{x0, cy - sc(3), x1, cy + sc(3)}, sc(6), blendColor(pal.menuBg, pal.menuText, 0.22))
	span := it.hi - it.lo
	if span <= 0 {
		span = 1
	}
	kx := x0 + int32(float64(v-it.lo)/float64(span)*float64(x1-x0))
	if kx > x0 {
		fillRR(hdc, rect{x0, cy - sc(3), kx, cy + sc(3)}, sc(6), pal.accent)
	}
	k := sc(14)
	ring := rect{kx - k/2 - 1, cy - k/2 - 1, kx - k/2 + k + 1, cy - k/2 + k + 1}
	fillEllipse(hdc, ring, blendColor(pal.menuBg, pal.menuText, 0.35))
	fillEllipse(hdc, rect{kx - k/2, cy - k/2, kx - k/2 + k, cy - k/2 + k}, rgb(255, 255, 255))
}

// paintBuffered dessine dans un tampon puis recopie à l'écran (pas de scintillement).
func paintBuffered(hwnd uintptr, draw func(mem uintptr, w, h int32)) {
	var ps paintStruct
	hdc, _, _ := pBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	var cr rect
	pGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&cr)))
	w, h := cr.Right, cr.Bottom
	if w > 0 && h > 0 {
		mem, _, _ := pCreateCompatDC.Call(hdc)
		bmp, _, _ := pCreateCompatBmp.Call(hdc, uintptr(w), uintptr(h))
		old, _, _ := pSelectObject.Call(mem, bmp)
		draw(mem, w, h)
		pBitBlt.Call(hdc, 0, 0, uintptr(w), uintptr(h), mem, 0, 0, srcCopy)
		pSelectObject.Call(mem, old)
		pDeleteObject.Call(bmp)
		pDeleteDC.Call(mem)
	}
	pEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
}

// ---------------------------------------------------------------------------
// Logique du menu : souris
// ---------------------------------------------------------------------------

func cancelMenu() {
	mResult = 0
	mDone = true
}

func openSub(parent *mLevel, idx int, selectFirst bool) {
	it := parent.items[idx]
	if len(it.sub) == 0 {
		return
	}
	nl := &mLevel{items: it.sub, sel: -1, parentIdx: idx, depth: parent.depth + 1}
	measureLevel(nl)
	placeSub(nl, parent, idx)
	if !createLevelWindow(nl, false) {
		return
	}
	if selectFirst {
		moveSel(nl, +1)
	}
}

// activate exécute une entrée (ou ouvre son sous-menu).
func activate(L *mLevel, idx int) {
	if idx < 0 || idx >= len(L.items) {
		return
	}
	it := L.items[idx]
	if !it.selectable() {
		return
	}
	if it.sub != nil {
		if len(mStack) == L.depth+1 {
			openSub(L, idx, true)
		}
		return
	}
	if it.slider {
		return // se règle à la souris (ou avec ← →)
	}
	if it.act != nil {
		if it.stay {
			it.act()
			reloadMenus()
			return
		}
		mAfter = it.act
		mDone = true
		return
	}
	mResult = it.id
	mDone = true
}

// slideTo règle un curseur d'après la position horizontale de la souris.
func slideTo(L *mLevel, idx int, x int32) {
	it := L.items[idx]
	x0, x1 := sliderSpan(L)
	if x1 <= x0 || it.setValue == nil {
		return
	}
	f := float64(x-x0) / float64(x1-x0)
	if f < 0 {
		f = 0
	}
	if f > 1 {
		f = 1
	}
	v := it.lo + int(f*float64(it.hi-it.lo)+0.5)
	if it.value() != v {
		it.setValue(v)
	}
	invalidateLevel(L)
}

// relayout recalcule la taille d'un niveau dont le contenu a changé, sans le déplacer.
func relayout(L, parent *mLevel) {
	if L.sel >= len(L.items) || (L.sel >= 0 && !L.items[L.sel].selectable()) {
		L.sel = -1
	}
	L.minW = L.w // la largeur ne rétrécit pas : le menu ne « saute » pas
	measureLevel(L)
	if parent == nil {
		fitHeight(L, L.maxH)
	} else {
		placeSub(L, parent, L.parentIdx)
	}
	pSetWindowPos.Call(L.hwnd, 0, uintptr(L.x), uintptr(L.y), uintptr(L.w), uintptr(L.h), swpNoZOrder|swpNoActivate)
	L.smooth = styleWindow(L.hwnd, L.w, L.h)
	invalidateLevel(L)
}

// reloadMenus redemande le contenu du menu (coches, valeurs, résultats de recherche)
// et met à jour tous les niveaux ouverts.
func reloadMenus() {
	if len(mStack) == 0 {
		return
	}
	root := mStack[0]
	if root.rebuild == nil {
		for _, L := range mStack {
			invalidateLevel(L)
		}
		return
	}
	root.items = root.rebuild(root.query)
	relayout(root, nil)
	for i := 1; i < len(mStack); i++ {
		par, L := mStack[i-1], mStack[i]
		if L.parentIdx < 0 || L.parentIdx >= len(par.items) || len(par.items[L.parentIdx].sub) == 0 {
			closeFrom(i)
			break
		}
		L.items = par.items[L.parentIdx].sub
		relayout(L, par)
	}
}

func menuHover() {
	p := cursorPt()
	if mDragL != nil {
		if l, _, _ := pGetAsyncKeyState.Call(0x01); l&0x8000 != 0 {
			slideTo(mDragL, mDragIdx, p.X)
			return
		}
		mDragL = nil
	}
	// Menu ouvert depuis la barre : en glissant sur un autre titre, on passe à son menu (comme sur macOS).
	if pressed != 0 {
		if k := barMenuKindAt(p); k != 0 && k != pressed {
			mSwitch = k
			cancelMenu()
			return
		}
	}
	li := levelAt(p)
	if li < 0 {
		// Le pointeur est sorti : on retire la surbrillance d'une entrée sans sous-menu.
		d := mStack[len(mStack)-1]
		if d.sel >= 0 && d.items[d.sel].sub == nil {
			d.sel = -1
			invalidateLevel(d)
		}
		return
	}
	L := mStack[li]
	idx := L.itemAt(p)
	if idx < 0 || !L.items[idx].selectable() {
		return
	}
	it := L.items[idx]
	if L.sel != idx {
		L.sel = idx
		invalidateLevel(L)
	}
	if len(mStack) > li+1 && !(it.sub != nil && mStack[li+1].parentIdx == idx) {
		closeFrom(li + 1)
	}
	if it.sub != nil && len(mStack) == li+1 {
		openSub(L, idx, false)
	}
}

func menuPress() {
	p := cursorPt()
	if li := levelAt(p); li >= 0 {
		L := mStack[li]
		if idx := L.itemAt(p); idx >= 0 && L.items[idx].slider && L.items[idx].selectable() {
			if l, _, _ := pGetAsyncKeyState.Call(0x01); l&0x8000 != 0 {
				mDragL, mDragIdx = L, idx
				slideTo(L, idx, p.X)
			}
			return
		}
	}
	if levelAt(p) < 0 {
		// Clic en dehors : on ferme sans transmettre le clic.
		if inBar(p) || inDock(p) {
			ignoreNextUp = true
		}
		cancelMenu()
	}
}

func menuRelease() {
	if mDragL != nil {
		mDragL = nil
		reloadMenus()
		return
	}
	p := cursorPt()
	li := levelAt(p)
	if li < 0 {
		return
	}
	L := mStack[li]
	activate(L, L.itemAt(p))
}

func menuWheel(delta int16) {
	p := cursorPt()
	li := levelAt(p)
	if li < 0 {
		li = len(mStack) - 1
	}
	scrollBy(mStack[li], int32(float64(-delta)/120*float64(rowH()*3)))
}

func menuAutoScroll() {
	p := cursorPt()
	for _, L := range mStack {
		if !L.scrolls || p.X < L.x || p.X >= L.x+L.w {
			continue
		}
		cy := p.Y - L.y
		switch {
		case cy >= 0 && cy < L.top:
			scrollBy(L, -sc(8))
		case cy >= L.h-L.top && cy < L.h:
			scrollBy(L, sc(8))
		}
	}
}

// menuOutsideClickGuard : si Windows a refusé de donner le premier plan au menu
// (donc pas de capture souris fiable, pas de WM_ACTIVATE), un clic en dehors du
// menu doit quand même le fermer.
func menuOutsideClickGuard() {
	if len(mStack) == 0 {
		return
	}
	if fg, _, _ := pGetForeground.Call(); fg == mStack[0].hwnd {
		return // cas normal : la capture et WM_ACTIVATE s'en chargent
	}
	l, _, _ := pGetAsyncKeyState.Call(0x01) // VK_LBUTTON
	r, _, _ := pGetAsyncKeyState.Call(0x02) // VK_RBUTTON
	if (l|r)&0x8000 == 0 {
		return
	}
	p := cursorPt()
	if levelAt(p) >= 0 {
		return
	}
	if inBar(p) || inDock(p) {
		ignoreNextUp = true
	} else {
		mLost = true // le clic va à une autre fenêtre : on ne reprend pas le focus
	}
	cancelMenu()
}

// ---------------------------------------------------------------------------
// Logique du menu : clavier
// ---------------------------------------------------------------------------

func moveSel(L *mLevel, dir int) {
	n := len(L.items)
	if n == 0 {
		return
	}
	cur := L.sel
	if cur < 0 {
		if dir > 0 {
			cur = -1
		} else {
			cur = n
		}
	}
	for k := 0; k < n; k++ {
		cur = (cur + dir + n) % n
		if L.items[cur].selectable() {
			L.sel = cur
			ensureVisible(L, cur)
			invalidateLevel(L)
			return
		}
	}
}

func menuKey(vk uintptr) {
	d := mStack[len(mStack)-1]
	if (vk == vkLeft || vk == vkRight) && d.sel >= 0 && d.items[d.sel].slider {
		it := d.items[d.sel]
		step := (it.hi - it.lo) / 20
		if step < 1 {
			step = 1
		}
		v := it.value()
		if vk == vkLeft {
			v -= step
		} else {
			v += step
		}
		if v < it.lo {
			v = it.lo
		}
		if v > it.hi {
			v = it.hi
		}
		it.setValue(v)
		invalidateLevel(d)
		return
	}
	switch vk {
	case vkEscape:
		if len(mStack) > 1 {
			closeFrom(len(mStack) - 1)
		} else {
			cancelMenu()
		}
	case vkDown:
		moveSel(d, +1)
	case vkUp:
		moveSel(d, -1)
	case vkHome:
		d.sel = -1
		moveSel(d, +1)
	case vkEnd:
		d.sel = -1
		moveSel(d, -1)
	case vkPrior:
		scrollBy(d, -d.h)
	case vkNext:
		scrollBy(d, d.h)
	case vkRight:
		if d.sel >= 0 && d.items[d.sel].selectable() && d.items[d.sel].sub != nil {
			openSub(d, d.sel, true)
		}
	case vkLeft:
		if len(mStack) > 1 {
			closeFrom(len(mStack) - 1)
		}
	case vkReturn, vkSpace:
		if vk == vkSpace && mStack[0].search {
			break // l'espace se tape dans le champ de recherche
		}
		activate(d, d.sel)
	}
}

// menuChar : taper une lettre saute à la prochaine entrée qui commence par elle.
func menuChar(ch rune) {
	if root := mStack[0]; root.search && root.rebuild != nil {
		switch {
		case ch == 8: // retour arrière
			r := []rune(root.query)
			if len(r) == 0 {
				return
			}
			root.query = string(r[:len(r)-1])
		case ch >= 32 && ch != 127:
			root.query += string(ch)
		default:
			return
		}
		reloadMenus()
		root.sel = -1
		if strings.TrimSpace(root.query) != "" {
			moveSel(root, +1) // le premier résultat est sélectionné : Entrée l'ouvre
		}
		invalidateLevel(root)
		return
	}
	if ch < 32 {
		return
	}
	d := mStack[len(mStack)-1]
	c := unicode.ToLower(ch)
	n := len(d.items)
	for k := 1; k <= n; k++ {
		i := (d.sel + k + n) % n
		it := d.items[i]
		if !it.selectable() || it.text == "" {
			continue
		}
		r, _ := utf8.DecodeRuneInString(it.text)
		if unicode.ToLower(r) == c {
			d.sel = i
			ensureVisible(d, i)
			invalidateLevel(d)
			return
		}
	}
}

// ---------------------------------------------------------------------------
// Fenêtre d'un niveau de menu
// ---------------------------------------------------------------------------

func menuProc(hwnd, message, wparam, lparam uintptr) uintptr {
	switch uint32(message) {
	case wmEraseBkgnd:
		return 1
	case wmPaint:
		L := levelOf(hwnd)
		paintBuffered(hwnd, func(mem uintptr, w, h int32) {
			if L != nil {
				drawLevel(mem, L)
			}
		})
		return 0
	}
	if menuOpen && len(mStack) > 0 {
		switch uint32(message) {
		case wmMouseMove:
			menuHover()
			return 0
		case wmLButtonDown, wmRButtonDown:
			menuPress()
			return 0
		case wmLButtonUp, wmRButtonUp:
			menuRelease()
			return 0
		case wmMouseWheel:
			menuWheel(int16(wparam >> 16))
			return 0
		case wmTimer:
			if wparam == menuTimerID {
				menuAutoScroll()
				menuOutsideClickGuard()
			}
			return 0
		case wmKeyDown:
			menuKey(wparam)
			return 0
		case wmChar:
			menuChar(rune(wparam))
			return 0
		case wmActivate:
			if wparam&0xFFFF == 0 { // WA_INACTIVE : un autre programme a pris la main
				mLost = true
				cancelMenu()
			}
			return 0
		case wmCancelMode, wmClose:
			cancelMenu()
			return 0
		}
	}
	r, _, _ := pDefWindowProc.Call(hwnd, message, wparam, lparam)
	return r
}

// pump fait tourner la boucle de messages jusqu'à ce que done() renvoie true.
func pump(done func() bool) {
	var m msg
	for !done() {
		r, _, _ := pGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			if int32(r) == 0 {
				pPostQuitMessage.Call(m.WParam) // laisse la boucle principale quitter aussi
			}
			return
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		pDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
	}
}

// trackMenu affiche le menu sous (x, y) et renvoie l'identifiant choisi (0 si aucun).
func trackMenu(owner uintptr, items []mItem, x, y int32) int {
	return trackMenuEx(owner, items, mOpts{}, x, y)
}

// mOpts : options d'un menu « vivant » (celui d'une icône de la barre).
type mOpts struct {
	rebuild func(query string) []mItem // redonne le contenu après un changement
	search  bool                       // les lettres tapées vont dans le champ de recherche
	minW    int32                      // largeur minimale
}

func trackMenuEx(owner uintptr, items []mItem, o mOpts, x, y int32) int {
	if menuOpen || dialogOpen || len(items) == 0 {
		return 0
	}
	root := &mLevel{items: items, sel: -1, parentIdx: -1, rebuild: o.rebuild, search: o.search, minW: o.minW}
	mAfter, mDragL = nil, nil
	measureLevel(root)
	placeRoot(root, x, y)

	prev, _, _ := pGetForeground.Call()
	mStack = mStack[:0]
	mResult, mDone, mLost = 0, false, false
	menuOpen = true
	if !createLevelWindow(root, true) {
		menuOpen = false
		return 0
	}
	pSetForeground.Call(root.hwnd)
	pSetFocus.Call(root.hwnd)
	pSetCapture.Call(root.hwnd)
	pSetTimer.Call(root.hwnd, menuTimerID, 40, 0)

	pump(func() bool { return mDone })

	menuOpen = false
	pReleaseCapture.Call()
	closeFrom(0)
	if pressed != 0 {
		pressed = 0
		invalidate(barHwnd)
	}
	if !mLost && prev != 0 && prev != barHwnd {
		if ok, _, _ := pIsWindow.Call(prev); ok != 0 {
			pSetForeground.Call(prev)
		}
	}
	mDragL = nil
	if cfgDirty { // réglages modifiés avec un curseur : on les enregistre à la fermeture
		saveConfig()
		cfgDirty = false
	}
	if f := mAfter; f != nil {
		mAfter = nil
		f()
	}
	return mResult
}

// ---------------------------------------------------------------------------
// Boîte de confirmation (remplace MessageBox)
// ---------------------------------------------------------------------------

var dlg struct {
	hwnd         uintptr
	title, hint  string
	ok, cancel   string
	w, h         int32
	titleR       rect
	hintR        rect
	iconR        rect
	btn          [2]rect // 0 = annuler, 1 = confirmer
	hover, press int
	result, done bool
	smooth       bool
	tracking     bool
}

func dlgHit(x, y int32) int {
	for i, r := range dlg.btn {
		if x >= r.Left && x < r.Right && y >= r.Top && y < r.Bottom {
			return i
		}
	}
	return -1
}

func drawDialog(hdc uintptr, w, h int32) {
	full := rect{0, 0, w, h}
	pFillRect.Call(hdc, uintptr(unsafe.Pointer(&full)), brushMenuBg)
	pSetBkMode.Call(hdc, transparent)

	pSelectObject.Call(hdc, fontTitle)
	pSetTextColor.Call(hdc, pal.menuText)
	tr := dlg.titleR
	pDrawText.Call(hdc, ptr(utf16(dlg.title)), ^uintptr(0), uintptr(unsafe.Pointer(&tr)), dtLeft|dtWordBreak|dtNoPrefix)

	pSelectObject.Call(hdc, fontReg)
	pSetTextColor.Call(hdc, pal.menuKey)
	hr := dlg.hintR
	pDrawText.Call(hdc, ptr(utf16(dlg.hint)), ^uintptr(0), uintptr(unsafe.Pointer(&hr)), dtLeft|dtWordBreak|dtNoPrefix)

	// Icône de l'alerte : le symbole de l'application, comme l'icône d'une alerte macOS.
	ic := dlg.iconR
	fillRR(hdc, ic, sc(26), pal.accent)
	sq, gap := sc(11), sc(3)
	x0 := ic.Left + (ic.Right-ic.Left-2*sq-gap)/2
	y0 := ic.Top + (ic.Bottom-ic.Top-2*sq-gap)/2
	for i := int32(0); i < 2; i++ {
		for j := int32(0); j < 2; j++ {
			fillRR(hdc, rect{x0 + j*(sq+gap), y0 + i*(sq+gap), x0 + j*(sq+gap) + sq, y0 + i*(sq+gap) + sq}, sc(3), rgb(255, 255, 255))
		}
	}

	labels := [2]string{dlg.cancel, dlg.ok}
	for i, r := range dlg.btn {
		brush := brushBtn
		if i == 1 {
			brush = brushAccent
		}
		if dlg.hover == i {
			if i == 1 {
				brush = brushAccentHi
			} else {
				brush = brushBtnHover
			}
		}
		fillRound(hdc, r, sc(12), brush)
		if i == 1 {
			pSetTextColor.Call(hdc, rgb(255, 255, 255))
		} else {
			pSetTextColor.Call(hdc, pal.menuText)
		}
		if i == 1 {
			pSelectObject.Call(hdc, fontBold)
		} else {
			pSelectObject.Call(hdc, fontReg)
		}
		drawText(hdc, labels[i], r, dtCenter)
	}

	if !dlg.smooth {
		pSelectObject.Call(hdc, penBorder)
		nb, _, _ := pGetStockObject.Call(5 /*NULL_BRUSH*/)
		pSelectObject.Call(hdc, nb)
		pRoundRect.Call(hdc, 0, 0, uintptr(w), uintptr(h), uintptr(sc(14)), uintptr(sc(14)))
	}
}

func dialogProc(hwnd, message, wparam, lparam uintptr) uintptr {
	switch uint32(message) {
	case wmEraseBkgnd:
		return 1
	case wmPaint:
		paintBuffered(hwnd, drawDialog)
		return 0
	case wmMouseMove:
		x, y := lparamXY(lparam)
		if i := dlgHit(x, y); i != dlg.hover {
			dlg.hover = i
			pInvalidateRect.Call(hwnd, 0, 0)
		}
		if !dlg.tracking {
			tme := trackMouseEvent{Flags: tmeLeave, HwndTrack: hwnd}
			tme.Size = uint32(unsafe.Sizeof(tme))
			pTrackMouseEvent.Call(uintptr(unsafe.Pointer(&tme)))
			dlg.tracking = true
		}
		return 0
	case wmMouseLeave:
		dlg.hover, dlg.tracking = -1, false
		pInvalidateRect.Call(hwnd, 0, 0)
		return 0
	case wmLButtonDown:
		x, y := lparamXY(lparam)
		dlg.press = dlgHit(x, y)
		return 0
	case wmLButtonUp:
		x, y := lparamXY(lparam)
		if i := dlgHit(x, y); i >= 0 && i == dlg.press {
			dlg.result, dlg.done = i == 1, true
		}
		dlg.press = -1
		return 0
	case wmKeyDown:
		if lparam&(1<<30) != 0 {
			return 0 // répétition automatique : une touche maintenue ne doit pas confirmer
		}
		switch wparam {
		case vkReturn:
			dlg.result, dlg.done = true, true
		case vkEscape:
			dlg.result, dlg.done = false, true
		}
		return 0
	case wmClose:
		dlg.result, dlg.done = false, true
		return 0
	}
	r, _, _ := pDefWindowProc.Call(hwnd, message, wparam, lparam)
	return r
}

// confirm affiche la boîte de confirmation et renvoie true si l'utilisateur confirme.
func confirm(owner uintptr, title, okLabel string) bool {
	if dialogOpen || menuOpen {
		return false
	}
	registerMenuClasses()
	dialogOpen = true
	defer func() { dialogOpen = false }()

	hint := t.confirmHint
	if confirmHintOverride != "" {
		hint = confirmHintOverride
	}
	dlg.title, dlg.hint, dlg.ok, dlg.cancel = title, hint, okLabel, t.cancel
	dlg.hover, dlg.press, dlg.result, dlg.done, dlg.tracking = -1, -1, false, false, false

	// Mesures
	w, pad := sc(420), sc(22)
	icon := sc(56)
	tx := pad + icon + sc(16) // le texte à droite de l'icône
	tw := w - tx - pad
	dc, _, _ := pGetDC.Call(0)
	old, _, _ := pSelectObject.Call(dc, fontTitle)
	th := textHeight(dc, title, tw)
	pSelectObject.Call(dc, fontReg)
	hh := textHeight(dc, dlg.hint, tw)
	bw := textWidth(dc, dlg.cancel)
	if v := textWidth(dc, dlg.ok); v > bw {
		bw = v
	}
	pSelectObject.Call(dc, old)
	pReleaseDC.Call(0, dc)
	bw += sc(36)
	if bw < sc(100) {
		bw = sc(100)
	}
	bh := sc(28)
	body := th + sc(6) + hh
	if body < icon {
		body = icon
	}
	h := pad + body + sc(22) + bh + pad

	dlg.w, dlg.h = w, h
	dlg.iconR = rect{pad, pad, pad + icon, pad + icon}
	dlg.titleR = rect{tx, pad, w - pad, pad + th}
	dlg.hintR = rect{tx, pad + th + sc(6), w - pad, pad + th + sc(6) + hh}
	by := h - pad - bh
	dlg.btn[1] = rect{w - pad - bw, by, w - pad, by + bh}
	dlg.btn[0] = rect{w - pad - 2*bw - sc(10), by, w - pad - bw - sc(10), by + bh}

	wa := workArea(0, 0)
	x := wa.Left + (wa.Right-wa.Left-w)/2
	y := wa.Top + (wa.Bottom-wa.Top-h)*2/5

	prev, _, _ := pGetForeground.Call()
	hInst, _, _ := pGetModuleHandle.Call(0)
	hwnd, _, _ := pCreateWindowEx.Call(uintptr(wsExTopmost|wsExToolwin), ptr(utf16(dialogClass)), 0, wsPopup,
		uintptr(x), uintptr(y), uintptr(w), uintptr(h), 0, 0, hInst, 0)
	if hwnd == 0 {
		return false
	}
	dlg.hwnd = hwnd
	dlg.smooth = styleWindow(hwnd, w, h)
	pShowWindow.Call(hwnd, swShow)
	pSetForeground.Call(hwnd)
	pSetFocus.Call(hwnd)

	pump(func() bool { return dlg.done })

	pDestroyWindow.Call(hwnd)
	dlg.hwnd = 0
	if !dlg.result && prev != 0 && prev != barHwnd {
		if ok, _, _ := pIsWindow.Call(prev); ok != 0 {
			pSetForeground.Call(prev)
		}
	}
	return dlg.result
}
