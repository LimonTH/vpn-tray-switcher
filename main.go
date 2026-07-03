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

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"vpn-switcher/config"
	"vpn-switcher/proxy"
	"vpn-switcher/tray"
	"vpn-switcher/tun"
	"vpn-switcher/utils"
	"vpn-switcher/vless"

	"github.com/vishvananda/netlink"
)

var (
	stateMu   sync.Mutex
	connected bool
	tunMode   bool
	conn      *vless.Connection
	quitCh    = make(chan struct{})

	tunCmd *exec.Cmd

	tunXrayPIDFile = filepath.Join(os.TempDir(), "vpn-switcher-tun-xray.pid")
	tunUpPIDFile   = filepath.Join(os.TempDir(), "vpn-switcher-tun-up.pid")
	tunStopFile    = filepath.Join(os.TempDir(), "vpn-switcher-tun-stop")
)

func main() {
	if err := utils.InitLogger(); err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}

	tunUpIface := flag.String("tun-up", "", "Start TUN + xray (i.e. tun0)")
	tunUpIP := flag.String("tun-ip", "10.0.0.1/24", "TUN interface IP with CIDR")
	tunUpGW := flag.String("tun-gw", "10.0.0.1", "TUN gateway IP")
	tunUpConfig := flag.String("tun-config", "", "Path to xray TUN config (required with --tun-up)")
	tunUpXray := flag.String("tun-xray", "", "Path to xray binary (required with --tun-up)")
	tunBypass := flag.String("tun-bypass", "", "VLESS server IP for route bypass (required with --tun-up)")
	tunDownIface := flag.String("tun-down", "", "Remove TUN interface (i.e. tun0)")
	tunDisconnectIface := flag.String("tun-disconnect", "", "Stop xray + remove TUN interface (i.e. tun0)")
	flag.Parse()

	if *tunUpIface != "" {
		os.Exit(runTunUp(*tunUpIface, *tunUpIP, *tunUpGW, *tunUpConfig, *tunUpXray, *tunBypass))
	}
	if *tunDownIface != "" {
		os.Exit(runTunDown(*tunDownIface))
	}
	if *tunDisconnectIface != "" {
		os.Exit(runTunDisconnect(*tunDisconnectIface))
	}

	utils.Logger.Println("VPN Switcher starting...")

	cfg, err := loadOrCreateConfig()
	if err != nil {
		utils.Logger.Fatalf("Failed to load config: %v", err)
	}
	utils.Logger.Printf(
		"Config loaded: %s:%d (mode: proxy) [TUN=%v]",
		cfg.VLESS.Address, cfg.VLESS.Port, cfg.Proxy.TUNMode,
	)

	conn = vless.NewConnection(&vless.Config{
		Address:       cfg.VLESS.Address,
		Port:          cfg.VLESS.Port,
		UUID:          cfg.VLESS.UUID,
		Transport:     cfg.VLESS.Transport,
		Security:      cfg.VLESS.Security,
		Encryption:    cfg.VLESS.Encryption,
		Flow:          cfg.VLESS.Flow,
		Sni:           cfg.VLESS.Sni,
		Pbk:           cfg.VLESS.Pbk,
		Sid:           cfg.VLESS.Sid,
		Fp:            cfg.VLESS.Fp,
		SOCKS5Port:    cfg.Proxy.SOCKS5Port,
		HTTPPort:      cfg.Proxy.HTTPPort,
		TLSServerName: cfg.VLESS.Address,
	})

	tunMode = cfg.Proxy.TUNMode

	go func() {
		tray.Start(tray.Actions{
			OnToggleConnect: handleToggleConnect,
			OnToggleMode:    handleToggleMode,
			OnQuit:          handleQuit,
		})
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-sigCh:
		utils.Logger.Println("Received signal, shutting down...")
	case <-quitCh:
		utils.Logger.Println("Tray requested quit...")
	}

	stateMu.Lock()
	disconnect()
	stateMu.Unlock()
	utils.Logger.Println("VPN Switcher stopped")
	os.Exit(0)
}

func runTunUp(iface, ipCIDR, gateway, xrayConfig, xrayPath, vlessBypass string) int {
	utils.Logger.Printf("TUN up: creating %s, starting xray", iface)

	killXrayByPIDFile()
	_ = exec.Command("pkill", "-x", "xray").Run()

	_ = exec.Command("ip", "link", "delete", iface).Run()
	for i := 0; i < 20; i++ {
		if _, err := netlink.LinkByName(iface); err != nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	_ = tun.CleanupRoutes(iface)

	tunIface := tun.NewInterface(iface, 1500)

	if xrayPath == "" {
		utils.Logger.Printf("Xray not found: empty path")
		return 1
	}
	xrayCmd := exec.Command(xrayPath, "run", "-c", xrayConfig)
	xrayCmd.Stdout = os.Stdout
	xrayCmd.Stderr = os.Stderr
	if err := xrayCmd.Start(); err != nil {
		utils.Logger.Printf("Xray start failed: %v", err)
		return 1
	}
	utils.Logger.Printf("Xray started (PID %d), waiting for TUN interface %s...", xrayCmd.Process.Pid, iface)

	if _, err := tunIface.WaitForReady(30 * time.Second); err != nil {
		utils.Logger.Printf("TUN interface %s not created by xray: %v", iface, err)
		_ = xrayCmd.Process.Signal(os.Interrupt)
		_ = xrayCmd.Wait()
		return 1
	}
	utils.Logger.Printf("TUN interface %s is ready", iface)

	if vlessBypass != "" {
		bypassLog, err := exec.Command("sh", "-c",
			fmt.Sprintf("ip route show default | head -1 | awk '{print $3, $5}'"),
		).Output()
		if err == nil {
			parts := strings.Fields(string(bypassLog))
			if len(parts) >= 2 {
				physGW := parts[0]
				physIface := parts[1]
				cmd := exec.Command("ip", "route", "replace", vlessBypass+"/32", "via", physGW, "dev", physIface)
				if out, err := cmd.CombinedOutput(); err != nil {
					utils.Logger.Printf("Warning: failed to add bypass route for %s: %v (output: %s)", vlessBypass, err, out)
				} else {
					utils.Logger.Printf("Bypass route added: %s/32 via %s dev %s", vlessBypass, physGW, physIface)
				}
			}
		}
	}

	routeCfg := tun.DefaultRouteConfig(iface)
	if err := tun.SetupRoutes(routeCfg); err != nil {
		utils.Logger.Printf("Route setup failed: %v", err)
		tunIface.Destroy()
		_ = xrayCmd.Process.Signal(os.Interrupt)
		_ = xrayCmd.Wait()
		return 1
	}
	_ = tun.SetDNS(routeCfg.DNSServers)

	utils.Logger.Printf("TUN + xray running (PID %d)", xrayCmd.Process.Pid)

	time.Sleep(500 * time.Millisecond)
	if xrayCmd.ProcessState != nil && xrayCmd.ProcessState.Exited() {
		utils.Logger.Printf("Xray exited shortly after start")
		_ = xrayCmd.Wait()
		tunIface.Destroy()
		_ = tun.RestoreDNS()
		_ = tun.CleanupRoutes(iface)
		return 1
	}

	_ = os.WriteFile(tunXrayPIDFile, []byte(fmt.Sprintf("%d", xrayCmd.Process.Pid)), 0644)

	// ── Watch for stop signal (signal or stop file) ────────────────
	// Сначала удаляем стоп-файл на случай, если он остался от прошлого раза.
	_ = os.Remove(tunStopFile)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Ожидаем сигнал или появление стоп-файла (создаётся disconnectTUN
	// без вызова pkexec — не требует пароля).
	stopPoll := time.NewTicker(500 * time.Millisecond)
	defer stopPoll.Stop()

waitLoop:
	for {
		select {
		case <-sigCh:
			break waitLoop
		case <-stopPoll.C:
			if _, err := os.Stat(tunStopFile); err == nil {
				_ = os.Remove(tunStopFile)
				break waitLoop
			}
		}
	}

	utils.Logger.Println("TUN helper shutting down...")

	if xrayCmd.Process != nil {
		_ = xrayCmd.Process.Signal(os.Interrupt)
		done := make(chan struct{})
		go func() {
			_ = xrayCmd.Wait()
			close(done)
		}()
		select {
		case <-done:
			utils.Logger.Println("Xray stopped gracefully")
		case <-time.After(3 * time.Second):
			_ = xrayCmd.Process.Kill()
			utils.Logger.Println("Xray force killed")
		}
	}

	tunIface.Destroy()
	_ = tun.RestoreDNS()
	_ = tun.CleanupRoutes(iface)
	_ = os.Remove(tunXrayPIDFile)
	return 0
}

func runTunDisconnect(iface string) int {
	utils.Logger.Printf("TUN disconnect: stop xray + clean %s", iface)

	killXrayByPIDFile()
	_ = exec.Command("pkill", "-x", "xray").Run()

	_ = tun.RestoreDNS()
	_ = tun.CleanupRoutes(iface)
	_ = exec.Command("ip", "link", "delete", iface).Run()
	for i := 0; i < 10; i++ {
		if _, err := netlink.LinkByName(iface); err != nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
		_ = exec.Command("ip", "link", "delete", iface).Run()
	}

	_ = os.Remove(tunXrayPIDFile)
	utils.Logger.Printf("TUN %s disconnected", iface)
	return 0
}

func runTunDown(iface string) int {
	utils.Logger.Printf("TUN down: cleaning %s", iface)
	killXrayByPIDFile()
	_ = exec.Command("pkill", "-x", "xray").Run()
	_ = tun.RestoreDNS()
	_ = tun.CleanupRoutes(iface)
	_ = exec.Command("ip", "link", "delete", iface).Run()
	_ = os.Remove(tunXrayPIDFile)
	utils.Logger.Printf("TUN %s cleaned up", iface)
	return 0
}

func loadOrCreateConfig() (*config.Config, error) {
	cfg, err := config.Load()
	if err == nil {
		return cfg, nil
	}
	utils.Logger.Printf("Config not found, creating default: %v", err)
	def := config.DefaultConfig()
	if e := config.Save(def); e != nil {
		return nil, e
	}
	p, _ := config.Path()
	utils.Logger.Printf("Default config created at %s", p)
	utils.Logger.Printf("Edit %s with your VLESS server details and restart", p)
	return def, nil
}

func handleToggleConnect() {
	if connected {
		stateMu.Lock()
		disconnect()
		stateMu.Unlock()
	} else {
		connectAsync()
	}
}

func connectAsync() {
	tray.UpdateStatus("Статус: Подключение...")
	tray.UpdateConnectButton("Отключиться")

	go func() {
		stateMu.Lock()
		defer stateMu.Unlock()

		if connected {
			return
		}

		utils.Logger.Println("Connecting...")

		if tunMode {
			connectTUN()
		} else {
			connectProxy()
		}
	}()
}

func connectProxy() {
	if err := conn.Connect(); err != nil {
		utils.Logger.Printf("Connection failed: %v", err)
		tray.UpdateStatus("Статус: Ошибка подключения")
		tray.UpdateConnectButton("Подключиться")
		return
	}

	cfg := conn.Client()
	proxy.ConfigureSystemProxy(&proxy.ProxySettings{
		SOCKS5Port: cfg.SOCKS5Port(),
		HTTPPort:   cfg.HTTPPort(),
	})

	connected = true
	tray.SetConnectedIcon()
	tray.UpdateStatus("Статус: Подключён")
	tray.SetTooltip("VPN Switcher — Подключён (прокси)")
	utils.Logger.Println("Proxy mode connected")
}

func connectTUN() {
	configPath, err := conn.Client().WriteTUNConfig()
	if err != nil {
		utils.Logger.Printf("Failed to generate TUN config: %v", err)
		tray.UpdateStatus("Статус: Ошибка конфигурации")
		tray.UpdateConnectButton("Подключиться")
		return
	}
	utils.Logger.Printf("TUN xray config written to %s", configPath)

	xrayPath, err := vless.XrayBin()
	if err != nil {
		utils.Logger.Printf("Xray not found: %v", err)
		tray.UpdateStatus("Статус: Ошибка TUN")
		tray.UpdateConnectButton("Подключиться")
		return
	}

	serverAddr := conn.Client().ServerAddress()

	tunCmd = exec.Command("pkexec",
		selfPath,
		"--tun-up", "tun0",
		"--tun-ip", "10.0.0.1/24",
		"--tun-gw", "10.0.0.1",
		"--tun-config", configPath,
		"--tun-xray", xrayPath,
		"--tun-bypass", serverAddr,
	)
	tunCmd.Stdout = os.Stdout
	tunCmd.Stderr = os.Stderr

	utils.Logger.Println("Requesting root privileges for TUN mode...")
	if err := tunCmd.Start(); err != nil {
		utils.Logger.Printf("Failed to launch TUN helper: %v", err)
		tray.UpdateStatus("Статус: Ошибка TUN")
		tray.UpdateConnectButton("Подключиться")
		tunCmd = nil
		return
	}

	_ = os.WriteFile(tunUpPIDFile, []byte(fmt.Sprintf("%d", tunCmd.Process.Pid)), 0644)

	utils.Logger.Println("Waiting for TUN helper to initialize...")
	timeout := 30 * time.Second
	deadline := time.Now().Add(timeout)
	interval := 500 * time.Millisecond

	time.Sleep(2 * time.Second)

	for time.Now().Before(deadline) {
		if tunCmd.ProcessState != nil && tunCmd.ProcessState.Exited() {
			utils.Logger.Printf("TUN helper exited too early (password cancelled?)")
			tray.UpdateStatus("Статус: Ошибка TUN")
			tray.UpdateConnectButton("Подключиться")
			tunCmd = nil
			return
		}

		if isXrayAlive() {
			connected = true
			tray.SetConnectedIcon()
			tray.UpdateStatus("Статус: Подключён (TUN)")
			tray.SetTooltip("VPN Switcher — Подключён (TUN)")
			utils.Logger.Println("TUN mode connected")
			return
		}

		time.Sleep(interval)
	}

	utils.Logger.Printf("TUN helper did not initialize within %v", timeout)
	if tunCmd != nil && tunCmd.Process != nil {
		_ = tunCmd.Process.Kill()
		_ = tunCmd.Wait()
	}
	tunCmd = nil
	_ = exec.Command("pkexec", selfPath, "--tun-down", "tun0").Run()
	tray.UpdateStatus("Статус: Ошибка TUN")
	tray.UpdateConnectButton("Подключиться")
}

func disconnect() {
	if tunMode {
		disconnectTUN()
	} else {
		disconnectProxy()
	}
}

func disconnectProxy() {
	tray.UpdateStatus("Статус: Отключение...")
	tray.UpdateConnectButton("Подключиться")
	utils.Logger.Println("Disconnecting proxy...")

	proxy.RestoreSystemProxy()

	if conn != nil {
		if err := conn.Disconnect(); err != nil {
			utils.Logger.Printf("Graceful disconnect failed, force killing: %v", err)
			conn.Kill()
		}
	}
	connected = false
	tray.SetDisconnectedIcon()
	tray.UpdateStatus("Статус: Отключён")
	tray.SetTooltip("VPN Switcher — Отключён")
	utils.Logger.Println("Proxy disconnected")
}

func disconnectTUN() {
	tray.UpdateStatus("Статус: Отключение TUN...")
	tray.UpdateConnectButton("Подключиться")
	utils.Logger.Println("Disconnecting TUN...")

	// ── Сигнал к остановке через стоп-файл (без пароля) ──────────────
	// Создаём файл, который runTunUp отслеживает в цикле ожидания.
	// Это полностью заменяет pkexec --tun-disconnect и не требует
	// повторного ввода пароля.
	_ = os.WriteFile(tunStopFile, []byte("stop"), 0644)

	if tunCmd != nil && tunCmd.Process != nil {
		utils.Logger.Printf("Waiting for TUN helper (PID %d) to stop...", tunCmd.Process.Pid)
		done := make(chan struct{})
		go func() {
			_ = tunCmd.Wait()
			close(done)
		}()
		select {
		case <-done:
			utils.Logger.Println("TUN helper stopped gracefully (via stop file)")
		case <-time.After(5 * time.Second):
			utils.Logger.Println("TUN helper did not respond to stop file, using pkexec cleanup...")
			_ = exec.Command("pkexec", selfPath, "--tun-disconnect", "tun0").Run()
			_ = tunCmd.Process.Kill()
			_ = tunCmd.Wait()
			utils.Logger.Println("TUN helper force-killed")
		}
		tunCmd = nil
	} else {
		// Helper не запущен — чистим через pkexec на всякий случай.
		_ = exec.Command("pkexec", selfPath, "--tun-disconnect", "tun0").Run()
	}

	_ = os.Remove(tunUpPIDFile)
	_ = os.Remove(tunStopFile)
	connected = false
	tray.SetDisconnectedIcon()
	tray.UpdateStatus("Статус: Отключён")
	tray.SetTooltip("VPN Switcher — Отключён")
	utils.Logger.Println("TUN disconnected")
}

func handleToggleMode() {
	stateMu.Lock()

	wasConnected := connected
	if wasConnected {
		disconnect()
	}

	tunMode = !tunMode
	var label string
	if tunMode {
		label = "TUN"
	} else {
		label = "Прокси"
	}

	tray.UpdateModeButton("Режим: " + label)
	utils.Logger.Printf("Switched to %s mode", label)

	stateMu.Unlock()

	if wasConnected {
		connectAsync()
	}
}

func handleQuit() {
	utils.Logger.Println("Exiting application")
	close(quitCh)
}

func killXrayByPIDFile() {
	data, err := os.ReadFile(tunXrayPIDFile)
	if err != nil {
		return
	}
	var pid int
	if _, err := fmt.Sscanf(string(data), "%d", &pid); err != nil || pid <= 0 {
		return
	}
	utils.Logger.Printf("Killing xray (PID %d) directly...", pid)

	statusPath := filepath.Join("/proc", strconv.Itoa(pid), "status")
	raw, err := os.ReadFile(statusPath)
	if err != nil {
		return
	}
	isXray := false
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(line, "Name:") && strings.Contains(line, "xray") {
			isXray = true
			break
		}
	}
	if !isXray {
		utils.Logger.Printf("PID %d is not xray (unexpected), skipping", pid)
		return
	}

	utils.Logger.Printf("Killing xray (PID %d) via pkexec...", pid)
	_ = exec.Command("pkexec", "kill", "-SIGINT", strconv.Itoa(pid)).Run()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		_, err := os.Stat(statusPath)
		if err != nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	if _, err := os.Stat(statusPath); err == nil {
		_ = exec.Command("pkexec", "kill", "-SIGKILL", strconv.Itoa(pid)).Run()
	}

	_ = os.Remove(tunXrayPIDFile)
}

func isXrayAlive() bool {
	data, err := os.ReadFile(tunXrayPIDFile)
	if err != nil {
		return false
	}
	var pid int
	if _, err := fmt.Sscanf(string(data), "%d", &pid); err != nil || pid <= 0 {
		return false
	}
	statusPath := filepath.Join("/proc", strconv.Itoa(pid), "status")
	raw, err := os.ReadFile(statusPath)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(line, "Name:") && strings.Contains(line, "xray") {
			return true
		}
	}
	return false
}

var selfPath = func() string {
	exe, err := os.Executable()
	if err != nil {
		return "vpn-switcher"
	}
	real, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return exe
	}
	return real
}()
