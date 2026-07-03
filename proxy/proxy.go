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

package proxy

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"vpn-switcher/utils"
)

// Настройки системного прокси.
type ProxySettings struct {
	SOCKS5Port int
	HTTPPort   int
}

// ConfigureSystemProxy настраивает системные прокси SOCKS5/HTTP.
// Поддерживает GNOME (gsettings), KDE (kwriteconfig5).
func ConfigureSystemProxy(s *ProxySettings) {
	configured := false

	if hasGNOME() {
		if err := setGNOMEProxy(s); err == nil {
			configured = true
			utils.Logger.Printf("System proxy configured via GNOME gsettings (SOCKS5 :%d, HTTP :%d)", s.SOCKS5Port, s.HTTPPort)
		}
	}

	if hasKDE() {
		if err := setKDEProxy(s); err == nil {
			configured = true
			utils.Logger.Printf("System proxy configured via KDE kwriteconfig5 (SOCKS5 :%d, HTTP :%d)", s.SOCKS5Port, s.HTTPPort)
		}
	}

	if !configured {
		utils.Logger.Printf(
			"Не удалось настроить системный прокси автоматически.\n"+
				"Настройте прокси вручную:\n"+
				"  SOCKS5: 127.0.0.1:%d\n"+
				"  HTTP:   127.0.0.1:%d",
			s.SOCKS5Port, s.HTTPPort,
		)
	}
}

// RestoreSystemProxy сбрасывает системные настройки прокси.
func RestoreSystemProxy() {
	if hasGNOME() {
		if err := unsetGNOMEProxy(); err != nil {
			utils.Logger.Printf("Failed to restore GNOME proxy: %v", err)
		}
	}

	if hasKDE() {
		if err := unsetKDEProxy(); err != nil {
			utils.Logger.Printf("Failed to restore KDE proxy: %v", err)
		}
	}
}

func hasGNOME() bool {
	if _, err := exec.LookPath("gsettings"); err != nil {
		return false
	}
	desktop := os.Getenv("XDG_CURRENT_DESKTOP")
	if strings.Contains(strings.ToLower(desktop), "gnome") {
		return true
	}
	if err := exec.Command("gsettings", "get", "org.gnome.system.proxy", "mode").Run(); err == nil {
		return true
	}
	return false
}

func gsettingsSet(schema, key, value string) error {
	return exec.Command("gsettings", "set", schema, key, value).Run()
}

func gsettingsSetInt(schema, key string, value int) error {
	return gsettingsSet(schema, key, strconv.Itoa(value))
}

func setGNOMEProxy(s *ProxySettings) error {
	if err := gsettingsSet("org.gnome.system.proxy", "mode", "manual"); err != nil {
		return fmt.Errorf("set mode: %w", err)
	}
	if err := gsettingsSetInt("org.gnome.system.proxy.socks", "host", 0); err != nil {
		return fmt.Errorf("set socks host type: %w", err)
	}
	if err := gsettingsSet("org.gnome.system.proxy.socks", "host", "127.0.0.1"); err != nil {
		return fmt.Errorf("set socks host: %w", err)
	}
	if err := gsettingsSetInt("org.gnome.system.proxy.socks", "port", s.SOCKS5Port); err != nil {
		return fmt.Errorf("set socks port: %w", err)
	}
	if err := gsettingsSet("org.gnome.system.proxy.http", "host", "127.0.0.1"); err != nil {
		return fmt.Errorf("set http host: %w", err)
	}
	if err := gsettingsSetInt("org.gnome.system.proxy.http", "port", s.HTTPPort); err != nil {
		return fmt.Errorf("set http port: %w", err)
	}
	return nil
}

func unsetGNOMEProxy() error {
	return gsettingsSet("org.gnome.system.proxy", "mode", "none")
}

func kconfigDir() (string, error) {
	if kdeHome := os.Getenv("KDEHOME"); kdeHome != "" {
		return kdeHome, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot get home directory: %w", err)
	}
	configDir := filepath.Join(home, ".config")
	return configDir, nil
}

func hasKDE() bool {
	if _, err := exec.LookPath("kwriteconfig5"); err != nil {
		return false
	}
	desktop := os.Getenv("XDG_CURRENT_DESKTOP")
	if strings.Contains(strings.ToLower(desktop), "kde") || strings.Contains(strings.ToLower(desktop), "plasma") {
		return true
	}
	return false
}

func setKDEProxy(s *ProxySettings) error {
	cfgDir, err := kconfigDir()
	if err != nil {
		return err
	}
	kioslaverc := filepath.Join(cfgDir, "kioslaverc")

	if err := exec.Command("kwriteconfig5", "--file", kioslaverc, "--group", "Proxy Settings", "--key", "ProxyType", "1").Run(); err != nil {
		return fmt.Errorf("kwriteconfig5 ProxyType: %w", err)
	}
	socksProxy := fmt.Sprintf("socks://127.0.0.1:%d", s.SOCKS5Port)
	if err := exec.Command("kwriteconfig5", "--file", kioslaverc, "--group", "Proxy Settings", "--key", "SocksProxy", socksProxy).Run(); err != nil {
		return fmt.Errorf("kwriteconfig5 SocksProxy: %w", err)
	}
	httpProxy := fmt.Sprintf("http://127.0.0.1:%d", s.HTTPPort)
	if err := exec.Command("kwriteconfig5", "--file", kioslaverc, "--group", "Proxy Settings", "--key", "httpProxy", httpProxy).Run(); err != nil {
		return fmt.Errorf("kwriteconfig5 httpProxy: %w", err)
	}
	_ = exec.Command("dbus-send", "--type=signal", "/KIO/Scheduler", "org.kde.KIO.Scheduler.reparseConfiguration").Run()
	return nil
}

func unsetKDEProxy() error {
	cfgDir, err := kconfigDir()
	if err != nil {
		return err
	}
	kioslaverc := filepath.Join(cfgDir, "kioslaverc")

	if err := exec.Command("kwriteconfig5", "--file", kioslaverc, "--group", "Proxy Settings", "--key", "ProxyType", "0").Run(); err != nil {
		return fmt.Errorf("kwriteconfig5 restore ProxyType: %w", err)
	}
	_ = exec.Command("dbus-send", "--type=signal", "/KIO/Scheduler", "org.kde.KIO.Scheduler.reparseConfiguration").Run()
	return nil
}
