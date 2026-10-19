//go:build linux

package telegram

import (
	"fmt"
	"syscall"
)

// bindControl привязывает сокет к сетевому интерфейсу (SO_BINDTODEVICE,
// ФТ-11.1.2). Пустое имя — без привязки.
func bindControl(iface string) func(network, address string, c syscall.RawConn) error {
	if iface == "" {
		return nil
	}
	return func(_, _ string, c syscall.RawConn) error {
		var sockErr error
		if err := c.Control(func(fd uintptr) {
			sockErr = syscall.SetsockoptString(int(fd), syscall.SOL_SOCKET, syscall.SO_BINDTODEVICE, iface)
		}); err != nil {
			return err
		}
		if sockErr != nil {
			return fmt.Errorf("привязка к интерфейсу %s: %w", iface, sockErr)
		}
		return nil
	}
}
