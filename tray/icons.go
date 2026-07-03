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
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
)

// Кэш сгенерированных иконок.
var iconPool struct {
	connected    []byte
	disconnected []byte
}

func init() {
	var err error
	iconPool.connected, err = generateIcon(color.RGBA{R: 76, G: 175, B: 80, A: 255})
	if err != nil {
		log.Fatalf("Failed to generate connected icon: %v", err)
	}
	iconPool.disconnected, err = generateIcon(color.RGBA{R: 158, G: 158, B: 158, A: 255})
	if err != nil {
		log.Fatalf("Failed to generate disconnected icon: %v", err)
	}
}

// generateIcon создаёт 16x16 PNG иконку.
func generateIcon(c color.Color) ([]byte, error) {
	const size = 16
	img := image.NewRGBA(image.Rect(0, 0, size, size))

	draw.Draw(img, img.Bounds(), &image.Uniform{C: color.Transparent}, image.Point{}, draw.Src)

	centerX, centerY := size/2, size/2
	radius := size/2 - 1
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx, dy := x-centerX, y-centerY
			if dx*dx+dy*dy <= radius*radius {
				img.Set(x, y, c)
			}
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// connectedIcon возвращает иконку подключённого состояния.
func connectedIcon() []byte {
	return iconPool.connected
}

// disconnectedIcon возвращает иконку отключённого состояния.
func disconnectedIcon() []byte {
	return iconPool.disconnected
}
