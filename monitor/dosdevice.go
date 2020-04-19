// +build windows

package monitor

import (
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"

	log "github.com/sirupsen/logrus"
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

func QueryDosDevice(device string) (targets []string, err error) {

	log.Tracef("Querying DOS device: %s", device)

	bufSize := uint32(2048)
	buf := make([]uint16, bufSize)
	device_, err := windows.UTF16PtrFromString(device)
	if err != nil {
		return
	}

	n, err := windows.QueryDosDevice(device_, &buf[0], bufSize)

	if err != nil {
		return
	}

	start := 0
	for i, v := range buf[:n] {
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

	var flags uint32 = windows.DDD_RAW_TARGET_PATH
	if remove {
		flags |= windows.DDD_REMOVE_DEFINITION | windows.DDD_EXACT_MATCH_ON_REMOVE
	}
	if !broadcast {
		flags |= windows.DDD_NO_BROADCAST_SYSTEM
	}

	device_, err := windows.UTF16PtrFromString(device)
	if err != nil {
		return err
	}
	target_, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return err
	}

	return windows.DefineDosDevice(flags, device_, target_)
}
