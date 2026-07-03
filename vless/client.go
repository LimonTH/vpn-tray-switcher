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

package vless

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"vpn-switcher/config"
)

// Режим работы клиента.
type Mode string

const (
	ModeProxy Mode = "proxy"
	ModeTUN   Mode = "tun"
)

// Параметры подключения VLESS.
type Config struct {
	Address       string
	Port          int
	UUID          string
	Transport     string
	Security      string
	Encryption    string
	Flow          string
	Sni           string
	Pbk           string
	Sid           string
	Fp            string
	SOCKS5Port    int
	HTTPPort      int
	TLSServerName string
}

// Клиент управляет процессом xray-core.
type Client struct {
	cfg        *Config
	mode       Mode
	cmd        *exec.Cmd
	running    bool
	configPath string
}

// XrayBin возвращает путь к бинарнику xray.
func XrayBin() (string, error) {
	if path, err := exec.LookPath("xray"); err == nil {
		return path, nil
	}
	candidates := []string{
		"/usr/local/bin/xray",
		"/usr/bin/xray",
		filepath.Join(os.Getenv("HOME"), ".local", "bin", "xray"),
		"/opt/xray/xray",
	}
	for _, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("xray binary not found; install from https://github.com/xtls/xray-core/releases")
}

// NewClient создаёт нового менеджера VLESS-клиента.
func NewClient(cfg *Config) *Client {
	return &Client{
		cfg:  cfg,
		mode: ModeProxy,
	}
}

// SetMode изменяет режим работы.
func (c *Client) SetMode(mode Mode) { c.mode = mode }

// IsRunning возвращает true, если xray запущен.
func (c *Client) IsRunning() bool { return c.running }

// SOCKS5Port возвращает порт SOCKS5.
func (c *Client) SOCKS5Port() int { return c.cfg.SOCKS5Port }

// HTTPPort возвращает порт HTTP.
func (c *Client) HTTPPort() int { return c.cfg.HTTPPort }

// ServerAddress возвращает адрес VLESS-сервера.
func (c *Client) ServerAddress() string { return c.cfg.Address }

// needsRegeneration проверяет, нужно ли перегенерировать конфиг.
func needsRegeneration(configPath string) bool {
	srcPath, err := config.Path()
	if err != nil {
		return true
	}

	dstInfo, err := os.Stat(configPath)
	if err != nil {
		return true
	}

	srcInfo, err := os.Stat(srcPath)
	if err != nil {
		return false
	}

	return srcInfo.ModTime().After(dstInfo.ModTime())
}

// WriteConfig генерирует и записывает xray-конфиг для текущего режима.
func (c *Client) WriteConfig() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	configDir := filepath.Join(home, ".config", "vpn-switcher")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return "", fmt.Errorf("cannot create config dir: %w", err)
	}
	path := filepath.Join(configDir, "xray-config.json")

	if !needsRegeneration(path) {
		return path, nil
	}

	return path, writeConfigTo(c.cfg, c.mode, path)
}

// WriteTUNConfig генерирует конфиг для TUN-режима.
func (c *Client) WriteTUNConfig() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	configDir := filepath.Join(home, ".config", "vpn-switcher")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return "", fmt.Errorf("cannot create config dir: %w", err)
	}
	path := filepath.Join(configDir, "xray-tun-config.json")

	if !needsRegeneration(path) {
		return path, nil
	}

	return path, writeConfigTo(c.cfg, ModeTUN, path)
}

// Start запускает xray (прокси-режим).
func (c *Client) Start() error {
	if c.running {
		return fmt.Errorf("client is already running")
	}
	xrayPath, err := XrayBin()
	if err != nil {
		return err
	}
	configPath, err := c.WriteConfig()
	if err != nil {
		return err
	}
	c.configPath = configPath

	c.cmd = exec.Command(xrayPath, "run", "-c", configPath)
	c.cmd.Stdout = os.Stdout
	c.cmd.Stderr = os.Stderr
	if err := c.cmd.Start(); err != nil {
		return fmt.Errorf("cannot start xray: %w", err)
	}
	c.running = true
	return nil
}

// Kill принудительно завершает xray.
func (c *Client) Kill() {
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
		_ = c.cmd.Wait()
	}
	c.running = false
	c.cmd = nil
}

// Stop корректно останавливает xray.
func (c *Client) Stop() error {
	if !c.running || c.cmd == nil {
		return nil
	}
	c.running = false
	if err := c.cmd.Process.Signal(os.Interrupt); err != nil {
		_ = c.cmd.Process.Kill()
	}
	_ = c.cmd.Wait()
	c.cmd = nil
	return nil
}

// writeConfigTo строит и записывает JSON-конфиг xray.
func writeConfigTo(cfg *Config, mode Mode, path string) error {
	userFlow := cfg.Flow
	if mode == ModeProxy && cfg.Security != "reality" {
		userFlow = ""
	}

	vnext := []map[string]interface{}{{
		"address": cfg.Address,
		"port":    cfg.Port,
		"users": []map[string]interface{}{{
			"id":         cfg.UUID,
			"encryption": cfg.Encryption,
			"flow":       userFlow,
		}},
	}}

	streamSettings := map[string]interface{}{
		"network":  cfg.Transport,
		"security": cfg.Security,
	}

	if cfg.Security == "tls" {
		ss := streamSettings
		tlsSettings := map[string]interface{}{
			"allowInsecure": false,
		}
		if cfg.Sni != "" {
			tlsSettings["serverName"] = cfg.Sni
		}
		if cfg.Fp != "" {
			tlsSettings["fingerprint"] = cfg.Fp
		}
		ss["tlsSettings"] = tlsSettings
	} else if cfg.Security == "reality" {
		ss := streamSettings
		realitySettings := map[string]interface{}{
			"publicKey": cfg.Pbk,
			"show":      false,
		}
		if cfg.Sid != "" {
			realitySettings["shortId"] = cfg.Sid
		}
		if cfg.Sni != "" {
			realitySettings["serverName"] = cfg.Sni
		}
		if cfg.Fp != "" {
			realitySettings["fingerprint"] = cfg.Fp
		}
		ss["realitySettings"] = realitySettings
	}

	if cfg.Transport == "ws" || cfg.Transport == "websocket" {
		streamSettings["network"] = "ws"
		streamSettings["wsSettings"] = map[string]interface{}{"path": "/"}
	}

	outbound := map[string]interface{}{
		"protocol":       "vless",
		"tag":            "proxy",
		"settings":       map[string]interface{}{"vnext": vnext},
		"streamSettings": streamSettings,
	}

	if cfg.TLSServerName != "" {
		if ts, ok := streamSettings["tlsSettings"].(map[string]interface{}); ok {
			ts["serverName"] = cfg.TLSServerName
		}
	}

	var inbounds []map[string]interface{}
	if mode == ModeProxy {
		inbounds = append(inbounds, map[string]interface{}{
			"port":     cfg.SOCKS5Port,
			"listen":   "127.0.0.1",
			"protocol": "socks",
			"settings": map[string]interface{}{"auth": "noauth", "udp": true},
			"tag":      "socks-in",
		})
		if cfg.HTTPPort > 0 {
			inbounds = append(inbounds, map[string]interface{}{
				"port":     cfg.HTTPPort,
				"listen":   "127.0.0.1",
				"protocol": "http",
				"settings": map[string]interface{}{},
				"tag":      "http-in",
			})
		}
	} else if mode == ModeTUN {
		inbounds = append(inbounds, map[string]interface{}{
			"protocol": "tun",
			"tag":      "tun-in",
			"settings": map[string]interface{}{
				"name":                   "tun0",
				"mtu":                    1500,
				"gateway":                []string{"10.0.0.1/24"},
				"dns":                    []string{"1.1.1.1", "8.8.8.8"},
				"autoOutboundsInterface": "auto",
			},
		})
	}

	var outbounds []map[string]interface{}
	freedomOutbound := map[string]interface{}{
		"protocol": "freedom",
		"tag":      "direct",
		"settings": map[string]interface{}{},
	}

	if mode == ModeTUN {
		outbounds = append(outbounds, outbound, freedomOutbound)
	} else {
		outbounds = append(outbounds, outbound)
	}

	config := map[string]interface{}{
		"log":       map[string]interface{}{"loglevel": "info"},
		"inbounds":  inbounds,
		"outbounds": outbounds,
		"dns": map[string]interface{}{
			"servers": []string{
				"https://1.1.1.1/dns-query",
				"https://dns.google/dns-query",
				"localhost",
			},
		},
	}

	if mode == ModeTUN {
		config["routing"] = map[string]interface{}{
			"domainStrategy": "IPIfNonMatch",
			"rules": []map[string]interface{}{
				{
					"type":        "field",
					"outboundTag": "direct",
					"ip": []string{
						"geoip:private",
						"geoip:cn",
					},
				},
				{
					"type":        "field",
					"outboundTag": "direct",
					"domain":      []string{"geosite:cn"},
				},
				{
					"type":        "field",
					"outboundTag": "direct",
					"ip":          []string{"ff00::/8", "fe80::/10"},
				},
			},
		}
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("cannot marshal xray config: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}
