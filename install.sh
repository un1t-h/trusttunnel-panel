#!/bin/sh
# Установка TrustTunnel Panel одной командой:
#   curl -fsSL https://raw.githubusercontent.com/un1t-h/trusttunnel-panel/main/install.sh | sudo sh
#
# Скачивает готовый бинарник под вашу архитектуру (amd64 / arm64 / armv7) из GitHub Releases,
# ставит его в /opt/ttpanel и запускает systemd-службу ttpanel. Повторный запуск — обновление.
# Если запускать из каталога с исходниками (sudo sh install.sh), панель соберётся из них (нужен Go).
#
# Параметры (переменные окружения):
#   TTPANEL_PORT=8443        порт веб-интерфейса
#   TTPANEL_UNINSTALL=1      удалить панель (данные в /var/lib/ttpanel сохраняются)
set -eu

REPO="un1t-h/trusttunnel-panel"
PORT="${TTPANEL_PORT:-8443}"
BIN=/opt/ttpanel/ttpanel
UNIT=/etc/systemd/system/ttpanel.service

say() { printf '\033[1;36m==>\033[0m %s\n' "$*"; }
die() { printf '\033[1;31mОшибка:\033[0m %s\n' "$*" >&2; exit 1; }

[ "$(id -u)" -eq 0 ] || die "запустите с правами root (через sudo)"
command -v systemctl >/dev/null || die "нужен systemd"

if [ "${TTPANEL_UNINSTALL:-}" = "1" ]; then
  systemctl disable --now ttpanel 2>/dev/null || true
  rm -f "$UNIT" && systemctl daemon-reload
  rm -rf /opt/ttpanel
  say "Панель удалена. Данные (серверы, клиенты, ключ) остались в /var/lib/ttpanel — удалите вручную, если не нужны."
  exit 0
fi

case "$(uname -m)" in
  x86_64|amd64) ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
  armv7l|armv7*|armhf) ARCH=armv7 ;;
  *) die "неподдерживаемая архитектура: $(uname -m)" ;;
esac

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT
SRC_DIR=$(cd "$(dirname "$0")" 2>/dev/null && pwd || echo "")

if [ -n "$SRC_DIR" ] && [ -f "$SRC_DIR/go.mod" ] && [ -f "$SRC_DIR/main.go" ]; then
  say "Сборка из исходников в $SRC_DIR"
  if ! command -v go >/dev/null; then
    say "Устанавливаю Go"
    apt-get update -qq && apt-get install -y -qq golang-go
  fi
  VER=$(git -C "$SRC_DIR" describe --tags --always 2>/dev/null || echo dev)
  (cd "$SRC_DIR" && GOFLAGS=-p=1 CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$VER" -o "$TMP/ttpanel" .)
else
  command -v curl >/dev/null || { apt-get update -qq && apt-get install -y -qq curl; }
  say "Скачиваю последнюю версию для linux/$ARCH"
  URL="https://github.com/$REPO/releases/latest/download/ttpanel-linux-$ARCH"
  curl -fL --progress-bar -o "$TMP/ttpanel" "$URL" || die "не удалось скачать $URL"
  chmod +x "$TMP/ttpanel"
fi

say "Устанавливаю в /opt/ttpanel"
install -d /opt/ttpanel
install -m 0755 "$TMP/ttpanel" "$BIN"

cat > "$UNIT" <<EOF
[Unit]
Description=TrustTunnel Panel
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=$BIN -listen :$PORT -data /var/lib/ttpanel
Restart=on-failure
RestartSec=3
MemoryMax=64M
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/ttpanel
PrivateTmp=true
StateDirectory=ttpanel
StateDirectoryMode=0700

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable ttpanel >/dev/null 2>&1
systemctl restart ttpanel
sleep 2
systemctl is-active --quiet ttpanel || { journalctl -u ttpanel -n 20 --no-pager; die "служба не запустилась"; }

IP=$(hostname -I 2>/dev/null | awk '{print $1}')
say "Готово! Откройте https://${IP:-<ip-адрес>}:$PORT"
echo "    Браузер предупредит о самоподписанном сертификате — это нормально."
echo "    При первом входе панель предложит создать администратора."
