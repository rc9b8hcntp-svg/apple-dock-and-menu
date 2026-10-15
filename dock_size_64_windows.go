//go:build windows && (amd64 || arm64)

package main

import "unsafe"

// Tailles attendues par Windows (64 bits) pour les structures du Dock.
var (
	_ = [40 - unsafe.Sizeof(bitmapInfoHeader{})]struct{}{}
	_ = [unsafe.Sizeof(bitmapInfoHeader{}) - 40]struct{}{}
	_ = [32 - unsafe.Sizeof(iconInfo{})]struct{}{}
	_ = [unsafe.Sizeof(iconInfo{}) - 32]struct{}{}
	_ = [32 - unsafe.Sizeof(bitmapObj{})]struct{}{}
	_ = [unsafe.Sizeof(bitmapObj{}) - 32]struct{}{}
	_ = [696 - unsafe.Sizeof(shFileInfo{})]struct{}{}
	_ = [unsafe.Sizeof(shFileInfo{}) - 696]struct{}{}
	_ = [544 - unsafe.Sizeof(shStockIconInfo{})]struct{}{}
	_ = [unsafe.Sizeof(shStockIconInfo{}) - 544]struct{}{}
	_ = [20 - unsafe.Sizeof(shQueryRBInfo{})]struct{}{}
	_ = [unsafe.Sizeof(shQueryRBInfo{}) - 20]struct{}{}
	_ = [4 - unsafe.Sizeof(blendFunction{})]struct{}{}
	_ = [unsafe.Sizeof(blendFunction{}) - 4]struct{}{}
)
