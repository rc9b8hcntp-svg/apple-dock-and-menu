//go:build windows

package main

import (
	"encoding/binary"
	"fmt"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"menuwin/internal/dockgfx"
)

// ---------------------------------------------------------------------------
// Icônes : HICON -> sprite
// ---------------------------------------------------------------------------

// hiconSprite lit les pixels d'une icône Windows.
func hiconSprite(hicon uintptr) *dockgfx.Sprite {
	if hicon == 0 {
		return nil
	}
	var ii iconInfo
	if ok, _, _ := pGetIconInfo.Call(hicon, uintptr(unsafe.Pointer(&ii))); ok == 0 {
		return nil
	}
	defer func() {
		if ii.Color != 0 {
			pDeleteObject.Call(ii.Color)
		}
		if ii.Mask != 0 {
			pDeleteObject.Call(ii.Mask)
		}
	}()
	if ii.Color == 0 {
		return nil // icône monochrome : rare, on laisse l'icône par défaut
	}

	var bm bitmapObj
	pGetObject.Call(ii.Color, unsafe.Sizeof(bm), uintptr(unsafe.Pointer(&bm)))
	w, h := int(bm.Width), int(bm.Height)
	if w <= 0 || h <= 0 || w > 1024 || h > 1024 {
		return nil
	}

	dc, _, _ := pGetDC.Call(0)
	defer pReleaseDC.Call(0, dc)

	read := func(bmp uintptr) []byte {
		bi := bitmapInfoHeader{Width: int32(w), Height: -int32(h), Planes: 1, BitCount: 32}
		bi.Size = uint32(unsafe.Sizeof(bi))
		px := make([]byte, w*h*4)
		r, _, _ := pGetDIBits.Call(dc, bmp, 0, uintptr(h), uintptr(unsafe.Pointer(&px[0])), uintptr(unsafe.Pointer(&bi)), 0)
		if r == 0 {
			return nil
		}
		return px
	}

	px := read(ii.Color)
	if px == nil {
		return nil
	}
	hasAlpha := false
	for i := 3; i < len(px); i += 4 {
		if px[i] != 0 {
			hasAlpha = true
			break
		}
	}
	if !hasAlpha {
		// Ancienne icône sans canal alpha : la transparence est dans le masque
		// (noir = opaque, blanc = transparent).
		mask := read(ii.Mask)
		for i := 0; i < w*h; i++ {
			if mask == nil || mask[i*4] == 0 {
				px[i*4+3] = 255
			} else {
				px[i*4+3] = 0
			}
		}
	}
	return dockgfx.FromBGRA(w, h, px, false)
}

// ---------------------------------------------------------------------------
// Liste d'images du système (icônes jusqu'à 256 px)
// ---------------------------------------------------------------------------

var (
	iidImageList = guid{0x46EB5926, 0x582E, 0x4017, [8]byte{0x9F, 0xDF, 0xE8, 0x99, 0x8D, 0xAA, 0x09, 0x50}}
	imageLists   = map[uintptr]unsafe.Pointer{}
)

func imageList(which uintptr) unsafe.Pointer {
	if il, ok := imageLists[which]; ok {
		return il
	}
	var ppv unsafe.Pointer
	hr, _, _ := pSHGetImageList.Call(which, uintptr(unsafe.Pointer(&iidImageList)), uintptr(unsafe.Pointer(&ppv)))
	if int32(hr) != 0 {
		ppv = nil
	}
	imageLists[which] = ppv
	return ppv
}

// imageListIcon appelle IImageList::GetIcon (méthode n° 10 de l'interface).
func imageListIcon(il unsafe.Pointer, index int32) uintptr {
	if il == nil {
		return 0
	}
	vtbl := *(*unsafe.Pointer)(il)
	fn := *(*uintptr)(unsafe.Add(vtbl, 10*unsafe.Sizeof(uintptr(0))))
	var hicon uintptr
	hr, _, _ := syscall.SyscallN(fn, uintptr(il), uintptr(index), 1 /*ILD_TRANSPARENT*/, uintptr(unsafe.Pointer(&hicon)))
	if int32(hr) != 0 {
		return 0
	}
	return hicon
}

// systemIconSprite renvoie la plus grande icône disponible pour un index du
// système, réduite à la taille de travail du Dock.
func systemIconSprite(index int32) *dockgfx.Sprite {
	for _, which := range []uintptr{shilJumbo, shilExtraLarge} {
		hicon := imageListIcon(imageList(which), index)
		if hicon == 0 {
			continue
		}
		s := hiconSprite(hicon)
		pDestroyIcon.Call(hicon)
		if s != nil {
			return fitSprite(s)
		}
	}
	return nil
}

// fitSprite réduit un sprite trop grand : une taille de travail d'environ une
// fois et demie la taille maximale suffit pour un rendu net.
func fitSprite(s *dockgfx.Sprite) *dockgfx.Sprite {
	target := int(dockParams.Max * 1.5)
	if target < 64 {
		target = 64
	}
	if s.W > target {
		return dockgfx.ResizeArea(s, target, target*s.H/s.W)
	}
	return s
}

// spriteForPath donne l'icône d'un fichier (exécutable, raccourci…).
func spriteForPath(path string) *dockgfx.Sprite {
	var fi shFileInfo
	r, _, _ := pSHGetFileInfo.Call(ptr(utf16(path)), 0, uintptr(unsafe.Pointer(&fi)), unsafe.Sizeof(fi), shgfiSysIconIndex)
	if r != 0 {
		if s := systemIconSprite(fi.IconIndex); s != nil {
			return s
		}
	}
	// Repli : l'icône 32 px habituelle.
	var fi2 shFileInfo
	r, _, _ = pSHGetFileInfo.Call(ptr(utf16(path)), 0, uintptr(unsafe.Pointer(&fi2)), unsafe.Sizeof(fi2), shgfiIcon|shgfiLargeIcon)
	if r != 0 && fi2.Icon != 0 {
		s := hiconSprite(fi2.Icon)
		pDestroyIcon.Call(fi2.Icon)
		return s
	}
	return nil
}

// trashSprite donne l'icône de la Corbeille (vide ou pleine).
func trashSprite(full bool) *dockgfx.Sprite {
	id := uintptr(siidRecycler)
	if full {
		id = siidRecyclerFull
	}
	var si shStockIconInfo
	si.Size = uint32(unsafe.Sizeof(si))
	if hr, _, _ := pSHGetStockIconInfo.Call(id, shgsiSysIconIndex, uintptr(unsafe.Pointer(&si))); int32(hr) == 0 {
		return systemIconSprite(si.SysImageIndex)
	}
	return nil
}

// windowSprite lit l'icône d'une fenêtre (pour les applications du Store, dont
// toutes les fenêtres appartiennent au même processus hôte).
func windowSprite(hwnd uintptr) *dockgfx.Sprite {
	var res uintptr
	for _, kind := range []uintptr{iconBig, iconSmall2} {
		res = 0
		pSendMsgTimeout.Call(hwnd, wmGetIcon, kind, 0, smtoAbort, 40, uintptr(unsafe.Pointer(&res)))
		if res != 0 {
			if s := hiconSprite(res); s != nil {
				return s
			}
		}
	}
	for _, idx := range []int{gclHIcon, gclHIconSm} {
		if r, _, _ := pGetClassLongPtr.Call(hwnd, uintptr(idx)); r != 0 {
			if s := hiconSprite(r); s != nil {
				return s
			}
		}
	}
	return nil
}

var defaultSpriteCache *dockgfx.Sprite

func defaultSprite() *dockgfx.Sprite {
	if defaultSpriteCache == nil {
		icon, _, _ := pLoadIcon.Call(0, idiApp)
		defaultSpriteCache = hiconSprite(icon)
	}
	if defaultSpriteCache == nil {
		// Dernier recours : un carré gris arrondi.
		const n = 64
		px := make([]uint32, n*n)
		for i := range px {
			px[i] = 0xFF808088
		}
		defaultSpriteCache = &dockgfx.Sprite{W: n, H: n, Pix: px}
	}
	return defaultSpriteCache
}

// ---------------------------------------------------------------------------
// Processus : chemin de l'exécutable et nom de l'application
// ---------------------------------------------------------------------------

func procPath(hwnd uintptr) string {
	var pid uint32
	pGetWindowThreadPI.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if pid == 0 {
		return ""
	}
	h, _, _ := pOpenProcess.Call(processQueryLimited, 0, uintptr(pid))
	if h == 0 {
		return ""
	}
	defer pCloseHandle.Call(h)
	buf := make([]uint16, 1024)
	n := uint32(len(buf))
	if r, _, _ := pQueryFullProcessImageName.Call(h, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&n))); r == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:n])
}

// normKey identifie une application par son chemin. Un programme 32 bits voit
// System32 sous le nom SysWOW64 : on les confond pour retrouver les applis épinglées.
func normKey(p string) string {
	p = strings.ToLower(filepath.Clean(p))
	return strings.ReplaceAll(p, `\syswow64\`, `\system32\`)
}

// fileDescription lit le nom lisible d'un programme (« Bloc-notes », « Calculatrice »…).
func fileDescription(path string) string {
	var h uint32
	n, _, _ := pGetFileVersionInfoSize.Call(ptr(utf16(path)), uintptr(unsafe.Pointer(&h)))
	if n == 0 {
		return ""
	}
	buf := make([]byte, n)
	if ok, _, _ := pGetFileVersionInfo.Call(ptr(utf16(path)), 0, n, uintptr(unsafe.Pointer(&buf[0]))); ok == 0 {
		return ""
	}
	query := func(sub string) (unsafe.Pointer, uint32) {
		var p unsafe.Pointer
		var l uint32
		ok, _, _ := pVerQueryValue.Call(uintptr(unsafe.Pointer(&buf[0])), ptr(utf16(sub)),
			uintptr(unsafe.Pointer(&p)), uintptr(unsafe.Pointer(&l)))
		if ok == 0 {
			return nil, 0
		}
		return p, l
	}
	tr, l := query(`\VarFileInfo\Translation`)
	if tr == nil || l < 4 {
		return ""
	}
	lang := binary.LittleEndian.Uint16(unsafe.Slice((*byte)(tr), 4)[0:2])
	cp := binary.LittleEndian.Uint16(unsafe.Slice((*byte)(tr), 4)[2:4])
	val, vl := query(fmt.Sprintf(`\StringFileInfo\%04x%04x\FileDescription`, lang, cp))
	if val == nil || vl == 0 {
		return ""
	}
	return strings.TrimSpace(syscall.UTF16ToString(unsafe.Slice((*uint16)(val), int(vl))))
}

// recycleBinFull indique si la Corbeille contient des éléments.
func recycleBinFull() bool {
	var info shQueryRBInfo
	info.Size = uint32(unsafe.Sizeof(info))
	if hr, _, _ := pSHQueryRecycleBin.Call(0, uintptr(unsafe.Pointer(&info))); int32(hr) != 0 {
		return false
	}
	return binary.LittleEndian.Uint64(info.NumItems[:]) > 0
}
