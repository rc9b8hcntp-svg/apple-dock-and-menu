//go:build windows && 386

package main

import "unsafe"

// Tailles attendues par Windows (32 bits) pour les structures du Dock.
var (
	_ = [40 - unsafe.Sizeof(bitmapInfoHeader{})]struct{}{}
	_ = [unsafe.Sizeof(bitmapInfoHeader{}) - 40]struct{}{}
	_ = [20 - unsafe.Sizeof(iconInfo{})]struct{}{}
	_ = [unsafe.Sizeof(iconInfo{}) - 20]struct{}{}
	_ = [24 - unsafe.Sizeof(bitmapObj{})]struct{}{}
	_ = [unsafe.Sizeof(bitmapObj{}) - 24]struct{}{}
	_ = [692 - unsafe.Sizeof(shFileInfo{})]struct{}{}
	_ = [unsafe.Sizeof(shFileInfo{}) - 692]struct{}{}
	_ = [536 - unsafe.Sizeof(shStockIconInfo{})]struct{}{}
	_ = [unsafe.Sizeof(shStockIconInfo{}) - 536]struct{}{}
	_ = [20 - unsafe.Sizeof(shQueryRBInfo{})]struct{}{}
	_ = [unsafe.Sizeof(shQueryRBInfo{}) - 20]struct{}{}
	_ = [4 - unsafe.Sizeof(blendFunction{})]struct{}{}
	_ = [unsafe.Sizeof(blendFunction{}) - 4]struct{}{}
)
