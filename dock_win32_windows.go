//go:build windows

package main

import "syscall"

// Déclarations Win32 propres au Dock (fenêtre translucide, icônes, Corbeille).

const (
	wsExLayered = 0x00080000
	ulwAlpha    = 2

	shgfiSysIconIndex = 0x4000
	shgfiIcon         = 0x100
	shgfiLargeIcon    = 0x0
	shilExtraLarge    = 2
	shilJumbo         = 4
	shgsiSysIconIndex = 0x4000
	siidRecycler      = 31
	siidRecyclerFull  = 33

	shErbNoConfirm  = 0x1
	shErbNoProgress = 0x2
	shErbNoSound    = 0x4

	processQueryLimited = 0x1000
	gclHIcon            = -14
	gclHIconSm          = -34
	iconBig             = 1
)

type bitmapInfoHeader struct {
	Size          uint32
	Width, Height int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerM     int32
	YPelsPerM     int32
	ClrUsed       uint32
	ClrImportant  uint32
}

type iconInfo struct {
	IsIcon uint32
	XHot   uint32
	YHot   uint32
	Mask   uintptr
	Color  uintptr
}

type bitmapObj struct {
	Type       int32
	Width      int32
	Height     int32
	WidthBytes int32
	Planes     uint16
	BitsPixel  uint16
	Bits       uintptr
}

type shFileInfo struct {
	Icon        uintptr
	IconIndex   int32
	Attributes  uint32
	DisplayName [260]uint16
	TypeName    [80]uint16
}

type shStockIconInfo struct {
	Size          uint32
	Icon          uintptr
	SysImageIndex int32
	IconIdx       int32
	Path          [260]uint16
}

// SHQUERYRBINFO est déclarée « packée » (sans alignement) dans le SDK Windows :
// on utilise des tableaux d'octets pour avoir la même disposition en 32 et 64 bits.
type shQueryRBInfo struct {
	Size     uint32
	SizeB    [8]byte
	NumItems [8]byte
}

type blendFunction struct {
	BlendOp             byte
	BlendFlags          byte
	SourceConstantAlpha byte
	AlphaFormat         byte
}

var (
	version = syscall.NewLazyDLL("version.dll")

	pUpdateLayeredWindow = user32.NewProc("UpdateLayeredWindow")
	pGetIconInfo         = user32.NewProc("GetIconInfo")
	pDestroyIcon         = user32.NewProc("DestroyIcon")

	pOpenProcess               = kernel32.NewProc("OpenProcess")
	pQueryFullProcessImageName = kernel32.NewProc("QueryFullProcessImageNameW")
	pCloseHandle               = kernel32.NewProc("CloseHandle")

	pGetObject        = gdi32.NewProc("GetObjectW")
	pGetDIBits        = gdi32.NewProc("GetDIBits")
	pCreateDIBSection = gdi32.NewProc("CreateDIBSection")

	pSHGetFileInfo      = shell32.NewProc("SHGetFileInfoW")
	pSHGetImageList     = shell32.NewProc("SHGetImageList")
	pSHGetStockIconInfo = shell32.NewProc("SHGetStockIconInfo")
	pSHQueryRecycleBin  = shell32.NewProc("SHQueryRecycleBinW")
	pSHEmptyRecycleBin  = shell32.NewProc("SHEmptyRecycleBinW")

	pGetFileVersionInfoSize = version.NewProc("GetFileVersionInfoSizeW")
	pGetFileVersionInfo     = version.NewProc("GetFileVersionInfoW")
	pVerQueryValue          = version.NewProc("VerQueryValueW")
)
