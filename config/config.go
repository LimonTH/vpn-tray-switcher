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

package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Конфигурация VLESS-сервера.
type VLESSConfig struct {
	Address    string
	Port       int
	UUID       string
	Transport  string
	Security   string
	Encryption string
	Flow       string
	Sni        string
	Pbk        string
	Sid        string
	Fp         string
	Type       string
	Remark     string
}

// Локальные настройки прокси.
type ProxyConfig struct {
	SOCKS5Port int
	HTTPPort   int
	TUNMode    bool
}

// Общая конфигурация приложения.
type Config struct {
	VLESS VLESSConfig
	Proxy ProxyConfig
}

const configDirRel = ".config/vpn-switcher"
const configFileName = "config.txt"

// Путь к файлу конфигурации.
func Path() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot get home directory: %w", err)
	}
	return filepath.Join(home, configDirRel, configFileName), nil
}

// VLESSURI возвращает VLESS-ссылку.
func (c *VLESSConfig) VLESSURI() string {
	u := &url.URL{
		Scheme: "vless",
		User:   url.User(c.UUID),
		Host:   fmt.Sprintf("%s:%d", c.Address, c.Port),
	}
	q := u.Query()
	if c.Encryption != "" {
		q.Set("encryption", c.Encryption)
	}
	if c.Security != "" {
		q.Set("security", c.Security)
	}
	if c.Type != "" {
		q.Set("type", c.Type)
	}
	if c.Flow != "" {
		q.Set("flow", c.Flow)
	}
	if c.Sni != "" {
		q.Set("sni", c.Sni)
	}
	if c.Pbk != "" {
		q.Set("pbk", c.Pbk)
	}
	if c.Sid != "" {
		q.Set("sid", c.Sid)
	}
	if c.Fp != "" {
		q.Set("fp", c.Fp)
	}
	u.RawQuery = q.Encode()
	if c.Remark != "" {
		u.Fragment = c.Remark
	}
	return u.String()
}

// ParseVLESSURI парсит VLESS-ссылку.
func ParseVLESSURI(uriStr string) (*VLESSConfig, error) {
	u, err := url.Parse(uriStr)
	if err != nil {
		return nil, fmt.Errorf("invalid VLESS URI: %w", err)
	}
	if u.Scheme != "vless" {
		return nil, fmt.Errorf("not a VLESS URI: scheme=%s", u.Scheme)
	}
	cfg := &VLESSConfig{
		UUID:    u.User.String(),
		Address: u.Hostname(),
		Remark:  u.Fragment,
	}
	if cfg.Address == "" {
		return nil, fmt.Errorf("missing host in VLESS URI")
	}
	portStr := u.Port()
	if portStr != "" {
		p, err := strconv.Atoi(portStr)
		if err != nil {
			return nil, fmt.Errorf("invalid port: %w", err)
		}
		cfg.Port = p
	} else {
		cfg.Port = 443
	}
	q := u.Query()
	cfg.Encryption = q.Get("encryption")
	cfg.Security = q.Get("security")
	cfg.Type = q.Get("type")
	cfg.Flow = q.Get("flow")
	cfg.Sni = q.Get("sni")
	cfg.Pbk = q.Get("pbk")
	cfg.Sid = q.Get("sid")
	cfg.Fp = q.Get("fp")
	if cfg.Type == "" {
		cfg.Type = "tcp"
	}
	cfg.Transport = cfg.Type
	return cfg, nil
}

// Load читает и парсит конфигурацию.
func Load() (*Config, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read config file %s: %w", path, err)
	}

	uriStr := strings.TrimSpace(string(data))
	if uriStr == "" {
		return nil, fmt.Errorf("config file is empty")
	}

	vlessCfg, err := ParseVLESSURI(uriStr)
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		VLESS: *vlessCfg,
		Proxy: ProxyConfig{
			SOCKS5Port: 1080,
			HTTPPort:   8080,
			TUNMode:    false,
		},
	}
	return cfg, nil
}

// Save записывает конфигурацию.
func Save(cfg *Config) error {
	path, err := Path()
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("cannot create config directory %s: %w", dir, err)
	}

	data := []byte(cfg.VLESS.VLESSURI() + "\n")
	return os.WriteFile(path, data, 0644)
}

// DefaultConfig возвращает конфигурацию по умолчанию.
func DefaultConfig() *Config {
	return &Config{
		VLESS: VLESSConfig{
			Address:    "your-server.com",
			Port:       443,
			UUID:       "your-uuid-here",
			Transport:  "tcp",
			Security:   "reality",
			Encryption: "none",
			Sni:        "your-sni.com",
			Pbk:        "your-public-key",
			Sid:        "01",
			Fp:         "chrome",
			Remark:     "Default VLESS",
		},
		Proxy: ProxyConfig{
			SOCKS5Port: 1080,
			HTTPPort:   8080,
			TUNMode:    false,
		},
	}
}
