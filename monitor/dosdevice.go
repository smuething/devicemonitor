// +build windows

package monitor

import (
	"path/filepath"
	"strings"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"

	log "github.com/sirupsen/logrus"
)

const (
	DDD_RAW_TARGET_PATH       = 0x00000001
	DDD_REMOVE_DEFINITION     = 0x00000002
	DDD_EXACT_MATCH_ON_REMOVE = 0x00000004
	DDD_NO_BROADCAST_SYSTEM   = 0x00000008
)

var (
	modkernel32 = windows.NewLazySystemDLL("kernel32.dll")

	procDefineDosDeviceW = modkernel32.NewProc("DefineDosDeviceW")
	procQueryDosDevice   = modkernel32.NewProc("QueryDosDeviceW")
)

func fixLongPath(name string) (absolutePath string, err error) {
	absolutePath, err = filepath.Abs(name)

	if err != nil {
		return
	}

	if strings.HasPrefix(absolutePath, `\\?\`) {
		return
	}

	if strings.HasPrefix(absolutePath, `\\`) {
		absolutePath = strings.Replace(absolutePath, `\\`, `\\?\UNC\`, 1)
	} else {
		absolutePath = `\\?\` + absolutePath
	}

	return
}

func StringToUTF16Ptr(str string) *uint16 {
	wchars := utf16.Encode([]rune(str + "\x00"))
	return &wchars[0]
}

func QueryDosDevice(device string) (targets []string, err error) {

	log.Tracef("Querying DOS device: %s", device)

	var BUFSIZE uint32 = 2048
	var buf []uint16 = make([]uint16, BUFSIZE)
	var buffer *uint16 = &buf[0]

	r, _, err := procQueryDosDevice.Call(
		uintptr(unsafe.Pointer(StringToUTF16Ptr(device))),
		uintptr(unsafe.Pointer(buffer)),
		uintptr(BUFSIZE),
	)

	if err != windows.ERROR_SUCCESS {
		return
	}

	err = nil

	start := 0
	for i, v := range buf[:int(r)] {
		if v == 0 {
			targets = append(targets, windows.UTF16ToString(buf[start:i]))
			start = i + 1
		}
	}

	log.Tracef("DOS device %s -> %v", device, targets)
	return

}

func DefineDosDevice(device string, target string, normalizeTarget bool, remove bool, broadcast bool) (err error) {

	if normalizeTarget {
		target, err = fixLongPath(target)
		if err != nil {
			return
		}
	}

	var flags uint32 = DDD_RAW_TARGET_PATH
	if remove {
		flags |= DDD_REMOVE_DEFINITION | DDD_EXACT_MATCH_ON_REMOVE
	}
	if !broadcast {
		flags |= DDD_NO_BROADCAST_SYSTEM
	}

	_, _, err = procDefineDosDeviceW.Call(
		uintptr(flags),
		uintptr(unsafe.Pointer(StringToUTF16Ptr(device))),
		uintptr(unsafe.Pointer(StringToUTF16Ptr(target))),
	)

	if err == windows.ERROR_SUCCESS {
		err = nil
	}

	return
}
