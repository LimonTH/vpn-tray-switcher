/*
VLESS Config Switcher - A lightweight configuration switcher for VLESS profiles.
Copyright (C) 2026  LimonTH

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <https://gnu.org>.
*/

package tun

import (
	"fmt"
	"os/exec"
	"time"

	"vpn-switcher/utils"

	"github.com/vishvananda/netlink"
)

// Обёртка над TUN-интерфейсом.
type Interface struct {
	name    string
	mtu     int
	running bool
}

// NewInterface создаёт менеджер TUN-интерфейса.
func NewInterface(name string, mtu int) *Interface {
	if name == "" {
		name = "tun0"
	}
	if mtu <= 0 {
		mtu = 1500
	}
	return &Interface{
		name: name,
		mtu:  mtu,
	}
}

// WaitForReady ждёт появления TUN-интерфейса в ядре.
func (t *Interface) WaitForReady(timeout time.Duration) (*netlink.Link, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		link, err := netlink.LinkByName(t.name)
		if err != nil {
			time.Sleep(200 * time.Millisecond)
			continue
		}

		t.running = true
		time.Sleep(500 * time.Millisecond)

		if addrs, err := netlink.AddrList(link, netlink.FAMILY_V4); err == nil && len(addrs) > 0 {
			utils.Logger.Printf("TUN interface %s is ready with IP %s", t.name, addrs[0].IP.String())
		} else {
			utils.Logger.Printf("TUN interface %s is ready (IP not yet assigned)", t.name)
		}

		return &link, nil
	}
	return nil, fmt.Errorf("TUN interface %s did not appear within timeout (xray may have failed to start)", t.name)
}

// IsRunning возвращает true, если интерфейс активен.
func (t *Interface) IsRunning() bool {
	return t.running
}

// Name возвращает имя TUN-интерфейса.
func (t *Interface) Name() string {
	return t.name
}

// Destroy удаляет TUN-интерфейс из системы.
func (t *Interface) Destroy() error {
	t.running = false

	link, err := netlink.LinkByName(t.name)
	if err != nil {
		return nil
	}

	addrs, err := netlink.AddrList(link, netlink.FAMILY_V4)
	if err == nil {
		for _, addr := range addrs {
			_ = netlink.AddrDel(link, &addr)
		}
	}

	if err := netlink.LinkDel(link); err != nil {
		return fmt.Errorf("failed to delete TUN interface %s: %w", t.name, err)
	}

	return nil
}

// runCommand выполняет внешнюю команду.
func runCommand(args ...string) error {
	cmd := exec.Command(args[0], args[1:]...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("command %v failed: %w\nOutput: %s", args, err, output)
	}
	return nil
}
