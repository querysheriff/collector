#!/bin/sh

set -e

SVC_USER=pgdozor-collector
SVC_GROUP=pgdozor-collector
HOME_DIR=/var/lib/pgdozor-collector
CONF=/etc/pgdozor-collector.yml
EXAMPLE=/etc/pgdozor-collector.yml.example
SERVICE=pgdozor-collector.service

case "$1" in
  configure)
    if ! getent group "$SVC_GROUP" >/dev/null; then
      addgroup --system --quiet "$SVC_GROUP"
    fi
    if ! getent passwd "$SVC_USER" >/dev/null; then
      adduser --system --quiet \
        --home "$HOME_DIR" --no-create-home \
        --ingroup "$SVC_GROUP" --shell /usr/sbin/nologin "$SVC_USER"
    fi

    # Let the collector read PostgreSQL jsonlogs.
    if getent group postgres >/dev/null; then
      usermod --append --groups postgres "$SVC_USER" || true
    fi

    install -d -o "$SVC_USER" -g "$SVC_GROUP" -m 0750 "$HOME_DIR"

    # Seed the config on first install.
    if [ ! -e "$CONF" ]; then
      cp "$EXAMPLE" "$CONF"
    fi
    chown root:"$SVC_GROUP" "$CONF" || true
    chmod 0640 "$CONF"
    ;;
esac

if [ -d /run/systemd/system ]; then
  systemctl daemon-reload || true
  systemctl enable "$SERVICE" || true

  if [ -n "$2" ]; then
    # Upgrade ($2 = old version): restart only if it was already running.
    systemctl try-restart "$SERVICE" || true
  else
    echo "pgdozor-collector installed. Edit $CONF, then: systemctl start $SERVICE"
  fi
fi

exit 0
