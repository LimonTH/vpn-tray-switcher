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

// Состояние подключения.
type State int

const (
	StateDisconnected State = iota
	StateConnecting
	StateConnected
	StateDisconnecting
)

func (s State) String() string {
	switch s {
	case StateDisconnected:
		return "Отключён"
	case StateConnecting:
		return "Подключение..."
	case StateConnected:
		return "Подключён"
	case StateDisconnecting:
		return "Отключение..."
	default:
		return "Неизвестно"
	}
}

// Управление состоянием VLESS-подключения.
type Connection struct {
	client *Client
	state  State
}

// NewConnection создаёт новое подключение.
func NewConnection(cfg *Config) *Connection {
	return &Connection{
		client: NewClient(cfg),
		state:  StateDisconnected,
	}
}

// Connect запускает клиент и устанавливает соединение.
func (c *Connection) Connect() error {
	if c.state == StateConnected || c.state == StateConnecting {
		return nil
	}

	c.state = StateConnecting
	if err := c.client.Start(); err != nil {
		c.state = StateDisconnected
		return err
	}

	c.state = StateConnected
	return nil
}

// Disconnect останавливает клиент и разрывает соединение.
func (c *Connection) Disconnect() error {
	c.state = StateDisconnecting
	if err := c.client.Stop(); err != nil {
		c.state = StateConnected
		return err
	}
	c.state = StateDisconnected
	return nil
}

// Kill принудительно завершает xray.
func (c *Connection) Kill() {
	c.client.Kill()
	c.state = StateDisconnected
}

// State возвращает текущее состояние.
func (c *Connection) State() State {
	return c.state
}

// Client возвращает VLESS-клиент.
func (c *Connection) Client() *Client {
	return c.client
}

// SetMode изменяет режим работы.
func (c *Connection) SetMode(mode Mode) {
	c.client.SetMode(mode)
}
