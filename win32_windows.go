//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

// ---------------------------------------------------------------------------
// Constantes Win32
// ---------------------------------------------------------------------------

const (
	wmDestroy       = 0x0002
	wmPaint         = 0x000F
	wmClose         = 0x0010
	wmEraseBkgnd    = 0x0014
	wmDisplayChange = 0x007E
	wmTimer         = 0x0113
	wmHotkey        = 0x0312
	wmMouseMove     = 0x0200
	wmLButtonUp     = 0x0202
	wmRButtonUp     = 0x0205
	wmMouseLeave    = 0x02A3
	wmNull          = 0x0000
	wmGetIcon       = 0x007F
	wmApp           = 0x8000
	wmTray          = wmApp + 1
	wmAppBar        = wmApp + 2

	wsPopup       = 0x80000000
	wsExTopmost   = 0x00000008
	wsExToolwin   = 0x00000080
	wsExAppwindow = 0x00040000
	wsExNoActive  = 0x08000000

	swShowNoActivate = 4
	swMinimize       = 6
	swRestore        = 9

	idcArrow     = 32512
	idiApp       = 32512
	nimAdd       = 0
	nimDelete    = 2
	nifMessage   = 0x1
	nifIcon      = 0x2
	nifTip       = 0x4
	mfString     = 0x0
	mfPopup      = 0x10
	mfSeparator  = 0x800
	tpmReturnCmd = 0x0100
	tpmRightBtn  = 0x0002
	tpmNoNotify  = 0x0080
	modAlt       = 0x1
	modControl   = 0x2
	modNoRepeat  = 0x4000
	mbYesNo      = 0x4
	mbIconQuest  = 0x20
	mbTopmost    = 0x40000
	idYes        = 6
	tmeLeave     = 0x2
	createNoWin  = 0x08000000
	errAlready   = 183

	gwOwner      = 4
	smCxScreen   = 0
	dwmaCloaked  = 14
	smtoAbort    = 0x2
	iconSmall2   = 2
	diNormal     = 3
	srcCopy      = 0x00CC0020
	transparent  = 1
	dtLeft       = 0x0000
	dtCenter     = 0x0001
	dtRight      = 0x0002
	dtVCenter    = 0x0004
	dtSingleLine = 0x0020
	dtNoPrefix   = 0x0800
	dtEllipsis   = 0x8000

	swpNoSize     = 0x0001
	swpNoMove     = 0x0002
	swpNoActivate = 0x0010
	swpShowWindow = 0x0040

	abmNew      = 0
	abmRemove   = 1
	abmQueryPos = 2
	abmSetPos   = 3
	abeTop      = 1
	abnPosChg   = 1
	abnFullScr  = 2
)

// ---------------------------------------------------------------------------
// Structures Win32
// ---------------------------------------------------------------------------

type point struct{ X, Y int32 }
type rect struct{ Left, Top, Right, Bottom int32 }

type msg struct {
	HWnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
}

type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	IconSm     uintptr
}

type paintStruct struct {
	Hdc         uintptr
	Erase       int32
	Paint       rect
	Restore     int32
	IncUpdate   int32
	RgbReserved [32]byte
}

type guid struct {
	D1 uint32
	D2 uint16
	D3 uint16
	D4 [8]byte
}

type notifyIconData struct {
	Size             uint32
	HWnd             uintptr
	ID               uint32
	Flags            uint32
	CallbackMessage  uint32
	Icon             uintptr
	Tip              [128]uint16
	State            uint32
	StateMask        uint32
	Info             [256]uint16
	TimeoutOrVersion uint32
	InfoTitle        [64]uint16
	InfoFlags        uint32
	GuidItem         guid
	BalloonIcon      uintptr
}

type trackMouseEvent struct {
	Size      uint32
	Flags     uint32
	HwndTrack uintptr
	HoverTime uint32
}

type appBarData struct {
	Size            uint32
	HWnd            uintptr
	CallbackMessage uint32
	Edge            uint32
	Rc              rect
	LParam          uintptr
}

// ---------------------------------------------------------------------------
// Appels système
// ---------------------------------------------------------------------------

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	powrprof = syscall.NewLazyDLL("powrprof.dll")
	dwmapi   = syscall.NewLazyDLL("dwmapi.dll")

	pRegisterClassEx   = user32.NewProc("RegisterClassExW")
	pCreateWindowEx    = user32.NewProc("CreateWindowExW")
	pDestroyWindow     = user32.NewProc("DestroyWindow")
	pDefWindowProc     = user32.NewProc("DefWindowProcW")
	pGetMessage        = user32.NewProc("GetMessageW")
	pTranslateMessage  = user32.NewProc("TranslateMessage")
	pDispatchMessage   = user32.NewProc("DispatchMessageW")
	pPostQuitMessage   = user32.NewProc("PostQuitMessage")
	pPostMessage       = user32.NewProc("PostMessageW")
	pShowWindow        = user32.NewProc("ShowWindow")
	pSetWindowPos      = user32.NewProc("SetWindowPos")
	pGetWindowRect     = user32.NewProc("GetWindowRect")
	pGetClientRect     = user32.NewProc("GetClientRect")
	pLoadCursor        = user32.NewProc("LoadCursorW")
	pLoadIcon          = user32.NewProc("LoadIconW")
	pBeginPaint        = user32.NewProc("BeginPaint")
	pEndPaint          = user32.NewProc("EndPaint")
	pFillRect          = user32.NewProc("FillRect")
	pInvalidateRect    = user32.NewProc("InvalidateRect")
	pGetCursorPos      = user32.NewProc("GetCursorPos")
	pSetForeground     = user32.NewProc("SetForegroundWindow")
	pGetForeground     = user32.NewProc("GetForegroundWindow")
	pCreatePopupMenu   = user32.NewProc("CreatePopupMenu")
	pAppendMenu        = user32.NewProc("AppendMenuW")
	pTrackPopupMenu    = user32.NewProc("TrackPopupMenu")
	pDestroyMenu       = user32.NewProc("DestroyMenu")
	pMessageBox        = user32.NewProc("MessageBoxW")
	pRegisterHotKey    = user32.NewProc("RegisterHotKey")
	pUnregisterHotKey  = user32.NewProc("UnregisterHotKey")
	pLockWorkStation   = user32.NewProc("LockWorkStation")
	pSetProcessDPIAw   = user32.NewProc("SetProcessDPIAware")
	pGetDpiForSystem   = user32.NewProc("GetDpiForSystem")
	pGetSystemMetrics  = user32.NewProc("GetSystemMetrics")
	pTrackMouseEvent   = user32.NewProc("TrackMouseEvent")
	pSetTimer          = user32.NewProc("SetTimer")
	pKillTimer         = user32.NewProc("KillTimer")
	pEnumWindows       = user32.NewProc("EnumWindows")
	pIsWindowVisible   = user32.NewProc("IsWindowVisible")
	pIsIconic          = user32.NewProc("IsIconic")
	pGetWindow         = user32.NewProc("GetWindow")
	pGetWindowLongPtr  = user32.NewProc(longProc("GetWindowLong"))
	pGetWindowText     = user32.NewProc("GetWindowTextW")
	pGetWindowTextLen  = user32.NewProc("GetWindowTextLengthW")
	pGetWindowThreadPI = user32.NewProc("GetWindowThreadProcessId")
	pSendMsgTimeout    = user32.NewProc("SendMessageTimeoutW")
	pGetClassLongPtr   = user32.NewProc(longProc("GetClassLong"))
	pDrawIconEx        = user32.NewProc("DrawIconEx")
	pDrawText          = user32.NewProc("DrawTextW")

	pGetModuleHandle  = kernel32.NewProc("GetModuleHandleW")
	pCreateMutex      = kernel32.NewProc("CreateMutexW")
	pGetLastError     = kernel32.NewProc("GetLastError")
	pGetUILanguage    = kernel32.NewProc("GetUserDefaultUILanguage")
	pGetCurrentPID    = kernel32.NewProc("GetCurrentProcessId")
	pGetDateFormat    = kernel32.NewProc("GetDateFormatW")
	pGetTimeFormat    = kernel32.NewProc("GetTimeFormatW")
	pShellExecute     = shell32.NewProc("ShellExecuteW")
	pShellNotifyIcon  = shell32.NewProc("Shell_NotifyIconW")
	pSHAppBarMessage  = shell32.NewProc("SHAppBarMessage")
	pCreateSolidBrush = gdi32.NewProc("CreateSolidBrush")
	pDeleteObject     = gdi32.NewProc("DeleteObject")
	pDeleteDC         = gdi32.NewProc("DeleteDC")
	pCreateRoundRgn   = gdi32.NewProc("CreateRoundRectRgn")
	pFillRgn          = gdi32.NewProc("FillRgn")
	pCreateFont       = gdi32.NewProc("CreateFontW")
	pSelectObject     = gdi32.NewProc("SelectObject")
	pSetBkMode        = gdi32.NewProc("SetBkMode")
	pSetTextColor     = gdi32.NewProc("SetTextColor")
	pCreateCompatDC   = gdi32.NewProc("CreateCompatibleDC")
	pCreateCompatBmp  = gdi32.NewProc("CreateCompatibleBitmap")
	pBitBlt           = gdi32.NewProc("BitBlt")
	pSetSuspendState  = powrprof.NewProc("SetSuspendState")
	pDwmGetWindowAttr = dwmapi.NewProc("DwmGetWindowAttribute")
)

// ---------------------------------------------------------------------------
// Petits utilitaires
// ---------------------------------------------------------------------------

func utf16(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(s)
	return p
}

// Sur Windows 32 bits, GetWindowLongPtrW et GetClassLongPtrW n'existent pas
// (ce sont des macros) : on appelle GetWindowLongW et GetClassLongW.
func longProc(base string) string {
	if unsafe.Sizeof(uintptr(0)) == 8 {
		return base + "PtrW"
	}
	return base + "W"
}

func ptr(p *uint16) uintptr { return uintptr(unsafe.Pointer(p)) }

func rgb(r, g, b uint8) uintptr {
	return uintptr(uint32(r) | uint32(g)<<8 | uint32(b)<<16)
}

// getLongPtr lit GetWindowLongPtr avec un index négatif (GWL_*).
func getLongPtr(hwnd uintptr, index int) uintptr {
	r, _, _ := pGetWindowLongPtr.Call(hwnd, uintptr(index))
	return r
}
