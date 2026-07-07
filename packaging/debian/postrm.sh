#!/bin/sh

set -e

SERVICE=pgdozor-collector.service

case "$1" in
  remove)
    if [ -d /run/systemd/system ]; then
      systemctl disable "$SERVICE" || true
      systemctl daemon-reload || true
    fi
    ;;
  purge)
    rm -f /etc/pgdozor-collector.yml /etc/pgdozor-collector.yml.example
    rm -rf /var/lib/pgdozor-collector

    if getent passwd pgdozor-collector >/dev/null 2>&1; then
      deluser --system --quiet pgdozor-collector || true
    fi
    if getent group pgdozor-collector >/dev/null 2>&1; then
      delgroup --system --quiet pgdozor-collector || true
    fi

    if [ -d /run/systemd/system ]; then
      systemctl daemon-reload || true
    fi
    ;;
esac

exit 0
