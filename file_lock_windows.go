//go:build windows

package main

import (
	"fmt"
	"os"
	"syscall"
	"time"
	"unsafe"
)

const (
	lockfileFailImmediately = 0x00000001
	lockfileExclusiveLock   = 0x00000002
	errorLockViolation      = syscall.Errno(0x21)
)

var (
	kernel32          = syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx    = kernel32.NewProc("LockFileEx")
	procUnlockFileEx  = kernel32.NewProc("UnlockFileEx")
	cacheLockRangeLow = ^uint32(0)
)

type cacheFileUnlock func() error

func acquireCacheFileLock(lockPath string, timeout, retryDelay time.Duration) (cacheFileUnlock, error) {
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, err
	}

	deadline := time.Now().Add(timeout)
	var overlapped syscall.Overlapped
	for {
		err = tryWindowsFileLock(file, &overlapped)
		if err == nil {
			return func() error {
				unlockErr := unlockWindowsFile(file, &overlapped)
				closeErr := file.Close()
				if unlockErr != nil {
					return unlockErr
				}
				return closeErr
			}, nil
		}
		if err != errorLockViolation {
			_ = file.Close()
			return nil, err
		}
		if time.Now().Add(retryDelay).After(deadline) {
			_ = file.Close()
			return nil, fmt.Errorf("cache lock timeout: %s", lockPath)
		}
		time.Sleep(retryDelay)
	}
}

func tryWindowsFileLock(file *os.File, overlapped *syscall.Overlapped) error {
	ret, _, err := procLockFileEx.Call(
		file.Fd(),
		uintptr(lockfileExclusiveLock|lockfileFailImmediately),
		0,
		uintptr(cacheLockRangeLow),
		uintptr(cacheLockRangeLow),
		uintptr(unsafe.Pointer(overlapped)),
	)
	if ret != 0 {
		return nil
	}
	if err != syscall.Errno(0) {
		return err
	}
	return syscall.EINVAL
}

func unlockWindowsFile(file *os.File, overlapped *syscall.Overlapped) error {
	ret, _, err := procUnlockFileEx.Call(
		file.Fd(),
		0,
		uintptr(cacheLockRangeLow),
		uintptr(cacheLockRangeLow),
		uintptr(unsafe.Pointer(overlapped)),
	)
	if ret != 0 {
		return nil
	}
	if err != syscall.Errno(0) {
		return err
	}
	return syscall.EINVAL
}
