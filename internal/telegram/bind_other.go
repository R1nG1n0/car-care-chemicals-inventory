//go:build !linux

package telegram

import (
	"errors"
	"syscall"
)

// bindControl: привязка к интерфейсу есть только в Linux. Если интерфейс
// задан, а привязать нельзя, соединение не открывается — трафик не должен
// молча уходить мимо туннеля. Пустое имя — обычное соединение.
func bindControl(iface string) func(network, address string, c syscall.RawConn) error {
	if iface == "" {
		return nil
	}
	return func(_, _ string, _ syscall.RawConn) error {
		return errors.New("привязка к интерфейсу поддерживается только в Linux")
	}
}
