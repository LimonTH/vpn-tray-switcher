# VPN Switcher

Системный трей для управления VLESS/VPN-подключением через xray-core. Поддерживает два режима работы: **прокси** (SOCKS5/HTTP) и **TUN** (весь трафик системы).

## Требования

- **Go 1.26+**
- **xray-core** — [официальный релиз](https://github.com/xtls/xray-core/releases) (положить `xray` в `~/./ocal/bin/` или `/usr/local/bin/`)
- **libayatana-appindicator** — для системного трея (Linux):
  ```bash
  # Arch Linux
  sudo pacman -S libayatana-appindicator

  # Ubuntu/Debian
  sudo apt install libayatana-appindicator3-dev

  # Fedora
  sudo dnf install libayatana-appindicator-gtk3-devel
  ```

- **pkexec** — для TUN-режима (обычно в составе `polkit`, предустановлен в большинстве дистрибутивов)

## Установка

```bash
# Сборка
go build -o vpn-switcher .

# Установка xray-core
curl -L https://github.com/xtls/xray-core/releases/latest/download/Xray-linux-64.zip -o xray.zip
unzip xray.zip -d ~/.local/bin/
chmod +x ~/.local/bin/xray
```

## Конфигурация

При первом запуске конфиг создаётся автоматически. Отредактируйте `~/.config/vpn-switcher/config.txt`, вставив VLESS URI:

```
vless://your-uuid@your-server.com:443?encryption=none&security=reality&flow=xtls-rprx-vision&sni=www.example.com&pbk=your-public-key&fp=chrome&type=tcp#Метка
```

Параметры URI:

| Параметр | Описание |
|---|---|
| `encryption` | Шифрование VLESS (обычно `none`) |
| `security` | `tls` или `reality` |
| `flow` | XTLS-режим (`xtls-rprx-vision`) |
| `sni` | Server Name Indication |
| `pbk` | Публичный ключ REALITY |
| `sid` | Short ID REALITY (опционально) |
| `fp` | Отпечаток TLS (`chrome`, `firefox`, и т.д.) |
| `type` | Транспорт (`tcp`, `ws`, и т.д.) |

## Использование

```bash
./vpn-switcher
```

После запуска в системном трее появится иконка с меню:

- **Подключиться / Отключиться** — управление VPN-соединением
- **Режим: Прокси / TUN** — переключение между режимами
- **Выход** — завершение программы

### Режимы работы

#### 1. Прокси (режим по умолчанию)

Запускает xray-core с локальными прокси-серверами:

- **SOCKS5** — `127.0.0.1:1080`
- **HTTP** — `127.0.0.1:8080`

При подключении автоматически настраивает системный прокси (GNOME через gsettings, KDE через kwriteconfig5). При отключении — сбрасывает. Не требует root-прав.

#### 2. TUN (весь трафик через VPN)

Создаёт виртуальный TUN-интерфейс (`tun0`, IP `10.0.0.1/24`) и направляет весь системный трафик через VPN. Требует root-права — запрашивает через `pkexec`.

Трафик к VLESS-серверу автоматически исключается из TUN (bypass-маршрут) для предотвращения петли маршрутизации.

### Как проверить, что работает

```bash
# Проверка прокси-режима
curl --socks5 127.0.0.1:1080 https://www.google.com

# Проверка TUN-режима
curl https://www.google.com  # трафик идёт через TUN автоматически

# Проверить активные маршруты
ip route show
```

### Автозапуск (systemd)

```bash
mkdir -p ~/.config/systemd/user/

cat > ~/.config/systemd/user/vpn-switcher.service << 'EOF'
[Unit]
Description=VPN Switcher
After=network.target

[Service]
Type=simple
ExecStart=%h/go/bin/vpn-switcher
Restart=on-failure
RestartSec=5

[Install]
WantedBy=default.target
EOF

systemctl --user enable --now vpn-switcher
```

## Структура проекта

```
vpn-switcher/
├── main.go              # Точка входа, tray-меню, TUN helper
├── config/
│   └── config.go        # Парсинг VLESS URI, загрузка/сохранение
├── vless/
│   ├── client.go        # Генерация xray-конфига, управление процессом
│   └── connection.go    # Состояние соединения
├── tun/
│   ├── interface.go     # Ожидание/удаление TUN-интерфейса
│   └── routes.go        # Маршрутизация, DNS
├── proxy/
│   └── proxy.go         # Настройка системного прокси (GNOME/KDE)
├── tray/
│   ├── icons.go         # Генерация PNG-иконок
│   └── menu.go          # Меню системного трея
└── utils/
    └── logger.go        # Логирование в файл и stdout
```

## Логи

```
~/.local/share/vpn-switcher/vpn-switcher.log
```

Вывод xray-core дублируется в stdout вместе с логами приложения.
