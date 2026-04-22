//go:build linux || darwin || freebsd || netbsd || openbsd

package main

import (
	"fmt"
	"os"
	"syscall"
	"time"
)

type cacheFileUnlock func() error

func acquireCacheFileLock(lockPath string, timeout, retryDelay time.Duration) (cacheFileUnlock, error) {
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, err
	}

	deadline := time.Now().Add(timeout)
	for {
		err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return func() error {
				unlockErr := syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
				closeErr := file.Close()
				if unlockErr != nil {
					return unlockErr
				}
				return closeErr
			}, nil
		}
		if err != syscall.EWOULDBLOCK && err != syscall.EAGAIN {
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
