//go:build windows

// The Windows half of the system file icons (feature_icons.go): the icons Explorer shows, from SHGetFileInfo, turned
// into a PNG. A file type is asked by a made-up name with that extension (SHGFI_USEFILEATTRIBUTES, so nothing is read
// from disk); a file or folder itself by its path.
package main

import (
	"bytes"
	"image"
	"image/png"
	"syscall"
	"unsafe"
)

const sysIconsSupported = true

var (
	icShell32          = syscall.NewLazyDLL("shell32.dll")
	icUser32           = syscall.NewLazyDLL("user32.dll")
	icGdi32            = syscall.NewLazyDLL("gdi32.dll")
	procSHGetFileInfoW = icShell32.NewProc("SHGetFileInfoW")
	procDestroyIcon    = icUser32.NewProc("DestroyIcon")
	procGetIconInfo    = icUser32.NewProc("GetIconInfo")
	procGetDC          = icUser32.NewProc("GetDC")
	procReleaseDC      = icUser32.NewProc("ReleaseDC")
	procGetDIBits      = icGdi32.NewProc("GetDIBits")
	procGetObjectW     = icGdi32.NewProc("GetObjectW")
	procDeleteObject   = icGdi32.NewProc("DeleteObject")
)

const (
	shgfiIcon              = 0x000000100
	shgfiLargeIcon         = 0x000000000
	shgfiSmallIcon         = 0x000000001
	shgfiUseFileAttributes = 0x000000010
	fileAttributeDirectory = 0x10
	fileAttributeNormal    = 0x80
)

type shFileInfo struct {
	hIcon         uintptr
	iIcon         int32
	dwAttributes  uint32
	szDisplayName [260]uint16
	szTypeName    [80]uint16
}

type iconInfo struct {
	fIcon    int32
	xHotspot uint32
	yHotspot uint32
	hbmMask  uintptr
	hbmColor uintptr
}

type bitmap struct {
	bmType       int32
	bmWidth      int32
	bmHeight     int32
	bmWidthBytes int32
	bmPlanes     uint16
	bmBitsPixel  uint16
	bmBits       uintptr
}

type bitmapInfoHeader struct {
	biSize          uint32
	biWidth         int32
	biHeight        int32
	biPlanes        uint16
	biBitCount      uint16
	biCompression   uint32
	biSizeImage     uint32
	biXPelsPerMeter int32
	biYPelsPerMeter int32
	biClrUsed       uint32
	biClrImportant  uint32
}

func sysIconForType(ext string, px int) []byte {
	name, attr := "file", uint32(fileAttributeNormal)
	switch ext {
	case "/":
		name, attr = "folder", fileAttributeDirectory
	case "":
	default:
		name = "file." + ext
	}
	return shellIcon(name, attr, shgfiUseFileAttributes, px)
}

func sysIconForPath(path string, px int) []byte { return shellIcon(path, 0, 0, px) }

func shellIcon(name string, attr, flags uint32, px int) []byte {
	p, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return nil
	}
	size := uint32(shgfiLargeIcon)
	if px <= 16 {
		size = shgfiSmallIcon
	}
	var fi shFileInfo
	r, _, _ := procSHGetFileInfoW.Call(uintptr(unsafe.Pointer(p)), uintptr(attr), uintptr(unsafe.Pointer(&fi)), unsafe.Sizeof(fi), uintptr(shgfiIcon|size|flags))
	if r == 0 || fi.hIcon == 0 {
		return nil
	}
	defer procDestroyIcon.Call(fi.hIcon)
	img := iconImage(fi.hIcon)
	if img == nil {
		return nil
	}
	var buf bytes.Buffer
	if png.Encode(&buf, img) != nil {
		return nil
	}
	return buf.Bytes()
}

// iconImage reads an icon's colour bitmap as 32-bit pixels; an icon without alpha takes its transparency from the mask.
func iconImage(h uintptr) image.Image {
	var ii iconInfo
	if r, _, _ := procGetIconInfo.Call(h, uintptr(unsafe.Pointer(&ii))); r == 0 {
		return nil
	}
	if ii.hbmMask != 0 {
		defer procDeleteObject.Call(ii.hbmMask)
	}
	if ii.hbmColor == 0 {
		return nil
	}
	defer procDeleteObject.Call(ii.hbmColor)
	var bm bitmap
	if r, _, _ := procGetObjectW.Call(ii.hbmColor, unsafe.Sizeof(bm), uintptr(unsafe.Pointer(&bm))); r == 0 {
		return nil
	}
	w, hgt := int(bm.bmWidth), int(bm.bmHeight)
	if w <= 0 || hgt <= 0 || w > 256 || hgt > 256 {
		return nil
	}
	dc, _, _ := procGetDC.Call(0)
	defer procReleaseDC.Call(0, dc)
	bits := func(hbm uintptr) []byte {
		bi := bitmapInfoHeader{biWidth: int32(w), biHeight: -int32(hgt), biPlanes: 1, biBitCount: 32}
		bi.biSize = uint32(unsafe.Sizeof(bi))
		out := make([]byte, w*hgt*4)
		if r, _, _ := procGetDIBits.Call(dc, hbm, 0, uintptr(hgt), uintptr(unsafe.Pointer(&out[0])), uintptr(unsafe.Pointer(&bi)), 0); r == 0 {
			return nil
		}
		return out
	}
	col := bits(ii.hbmColor)
	if col == nil {
		return nil
	}
	alpha := false
	for i := 3; i < len(col); i += 4 {
		if col[i] != 0 {
			alpha = true
			break
		}
	}
	var mask []byte
	if !alpha && ii.hbmMask != 0 {
		mask = bits(ii.hbmMask)
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, hgt))
	for i := 0; i < w*hgt; i++ {
		b, g, r, a := col[i*4], col[i*4+1], col[i*4+2], col[i*4+3]
		if !alpha {
			a = 255
			if mask != nil && mask[i*4] != 0 {
				a = 0
			}
		}
		img.Pix[i*4], img.Pix[i*4+1], img.Pix[i*4+2], img.Pix[i*4+3] = r, g, b, a
	}
	return img
}
