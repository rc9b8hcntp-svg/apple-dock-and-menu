//go:build windows

package main

// Menus et boîte de confirmation dessinés par le programme.
//
// Aucun menu natif de Windows (TrackPopupMenu, MessageBox) n'est utilisé :
// chaque menu est une petite fenêtre sombre aux coins arrondis, dessinée ici
// avec le même style que la barre (surbrillance bleue, sous-menus, raccourcis
// alignés à droite, défilement des longues listes, clavier complet).

import (
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

func (it mItem) selectable() bool { return !it.sep && !it.disabled }

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
)

func makeFont(px int32, weight int) uintptr {
	f, _, _ := pCreateFont.Call(uintptr(-px), 0, 0, 0, uintptr(weight), 0, 0, 0,
		1 /*DEFAULT_CHARSET*/, 0, 0, 5 /*CLEARTYPE*/, 0, ptr(utf16("Segoe UI")))
	return f
}

func initMenuResources() {
	brushMenuBg, _, _ = pCreateSolidBrush.Call(rgb(44, 44, 48))
	brushBtn, _, _ = pCreateSolidBrush.Call(rgb(84, 84, 90))
	brushBtnHover, _, _ = pCreateSolidBrush.Call(rgb(100, 100, 106))
	brushAccentHi, _, _ = pCreateSolidBrush.Call(rgb(98, 157, 248))
	penBorder, _, _ = pCreatePen.Call(0 /*PS_SOLID*/, 1, rgb(78, 78, 84))
	cw := int32(1.6*scale + 0.5)
	if cw < 1 {
		cw = 1
	}
	penChev, _, _ = pCreatePen.Call(0, uintptr(cw), rgb(170, 170, 178))
	penChevSel, _, _ = pCreatePen.Call(0, uintptr(cw), rgb(255, 255, 255))
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
			col := uint32(rgb(78, 78, 84))
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
}

var (
	mStack  []*mLevel
	mResult int
	mDone   bool
	mLost   bool // le menu s'est fermé parce qu'une autre fenêtre a pris le focus
)

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
		if it.sep {
			continue
		}
		if w := textWidth(dc, it.text); w > maxText {
			maxText = w
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

	L.keyW, L.subW = 0, 0
	w := sc(12) + maxText + sc(14)
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
	L.w = w

	L.rects = L.rects[:0]
	var y int32
	for _, it := range L.items {
		h := rowH()
		if it.sep {
			h = sepH()
		}
		L.rects = append(L.rects, rect{padOuter(), y, w - padOuter(), y + h})
		y += h
	}
	L.contentH = y
}

// fitHeight fixe la hauteur de la fenêtre ; au-delà de maxH la liste défile.
func fitHeight(L *mLevel, maxH int32) {
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
		selected := i == L.sel && !it.disabled
		if selected {
			fillRound(hdc, r, sc(9), brushAccent)
		}
		textCol, keyCol := rgb(238, 238, 242), rgb(150, 150, 158)
		if selected {
			textCol, keyCol = rgb(255, 255, 255), rgb(225, 235, 255)
		} else if it.disabled {
			textCol, keyCol = rgb(110, 110, 116), rgb(110, 110, 116)
		}

		right := r.Right - sc(12) - L.subW
		if it.key != "" {
			pSetTextColor.Call(hdc, keyCol)
			drawText(hdc, it.key, rect{r.Left, r.Top, right, r.Bottom}, dtRight)
		}
		textRight := r.Right - sc(12) - L.subW
		if L.keyW > 0 {
			textRight -= L.keyW + sc(28)
		}
		pSetTextColor.Call(hdc, textCol)
		drawText(hdc, it.text, rect{r.Left + sc(12), r.Top, textRight, r.Bottom}, dtLeft|dtEllipsis)

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
	mResult = it.id
	mDone = true
}

func menuHover() {
	p := cursorPt()
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
	if levelAt(p) < 0 {
		// Clic en dehors : on ferme sans transmettre le clic.
		if inBar(p) {
			ignoreNextUp = true
		}
		cancelMenu()
	}
}

func menuRelease() {
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
	if inBar(p) {
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
		activate(d, d.sel)
	}
}

// menuChar : taper une lettre saute à la prochaine entrée qui commence par elle.
func menuChar(ch rune) {
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
	if menuOpen || dialogOpen || len(items) == 0 {
		return 0
	}
	root := &mLevel{items: items, sel: -1, parentIdx: -1}
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
	pSetTextColor.Call(hdc, rgb(255, 255, 255))
	tr := dlg.titleR
	pDrawText.Call(hdc, ptr(utf16(dlg.title)), ^uintptr(0), uintptr(unsafe.Pointer(&tr)), dtLeft|dtWordBreak|dtNoPrefix)

	pSelectObject.Call(hdc, fontReg)
	pSetTextColor.Call(hdc, rgb(160, 160, 168))
	hr := dlg.hintR
	pDrawText.Call(hdc, ptr(utf16(dlg.hint)), ^uintptr(0), uintptr(unsafe.Pointer(&hr)), dtLeft|dtWordBreak|dtNoPrefix)

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
		pSetTextColor.Call(hdc, rgb(255, 255, 255))
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

	dlg.title, dlg.hint, dlg.ok, dlg.cancel = title, t.confirmHint, okLabel, t.cancel
	dlg.hover, dlg.press, dlg.result, dlg.done, dlg.tracking = -1, -1, false, false, false

	// Mesures
	w, pad := sc(360), sc(22)
	tw := w - 2*pad
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
	bh := sc(30)
	h := pad + th + sc(6) + hh + sc(22) + bh + pad

	dlg.w, dlg.h = w, h
	dlg.titleR = rect{pad, pad, w - pad, pad + th}
	dlg.hintR = rect{pad, pad + th + sc(6), w - pad, pad + th + sc(6) + hh}
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
