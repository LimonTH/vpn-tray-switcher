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

package tray

import (
	"log"

	"github.com/getlantern/systray"
)

// Действия трея.
type Actions struct {
	OnToggleConnect func()
	OnToggleMode    func()
	OnQuit          func()
}

// Меню трея.
type Menu struct {
	connectItem *systray.MenuItem
	modeItem    *systray.MenuItem
	statusItem  *systray.MenuItem
}

var menuInstance *Menu

// Start запускает системный трей.
func Start(actions Actions) {
	systray.Run(func() {
		onReady(actions)
	}, onExit)
}

func onReady(actions Actions) {
	systray.SetIcon(disconnectedIcon())
	systray.SetTitle("VPN")
	systray.SetTooltip("VPN Switcher")

	statusItem := systray.AddMenuItem("Статус: Отключён", "Текущий статус")
	statusItem.Disable()

	systray.AddSeparator()

	connectItem := systray.AddMenuItem("Подключиться", "Подключиться к VPN")
	modeItem := systray.AddMenuItem("Режим: Прокси", "Переключить режим (Прокси/TUN)")

	systray.AddSeparator()

	quitItem := systray.AddMenuItem("Выход", "Выйти из программы")

	menuInstance = &Menu{
		connectItem: connectItem,
		modeItem:    modeItem,
		statusItem:  statusItem,
	}

	go handleMenuEvents(actions, connectItem, modeItem, quitItem)
}

func onExit() {
	log.Println("System tray exited")
}

func handleMenuEvents(actions Actions, connectItem, modeItem, quitItem *systray.MenuItem) {
	for {
		select {
		case <-connectItem.ClickedCh:
			actions.OnToggleConnect()
		case <-modeItem.ClickedCh:
			actions.OnToggleMode()
		case <-quitItem.ClickedCh:
			if actions.OnQuit != nil {
				actions.OnQuit()
			}
			return
		}
	}
}

// UpdateStatus обновляет текст статуса.
func UpdateStatus(text string) {
	if menuInstance != nil && menuInstance.statusItem != nil {
		menuInstance.statusItem.SetTitle(text)
	}
}

// UpdateConnectButton обновляет текст кнопки подключения.
func UpdateConnectButton(text string) {
	if menuInstance != nil && menuInstance.connectItem != nil {
		menuInstance.connectItem.SetTitle(text)
	}
}

// UpdateModeButton обновляет текст кнопки режима.
func UpdateModeButton(text string) {
	if menuInstance != nil && menuInstance.modeItem != nil {
		menuInstance.modeItem.SetTitle(text)
	}
}

// SetConnectedIcon меняет иконку на подключённую.
func SetConnectedIcon() {
	systray.SetIcon(connectedIcon())
}

// SetDisconnectedIcon меняет иконку на отключённую.
func SetDisconnectedIcon() {
	systray.SetIcon(disconnectedIcon())
}

// SetTooltip устанавливает подсказку.
func SetTooltip(text string) {
	systray.SetTooltip(text)
}
