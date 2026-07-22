#!/bin/sh

set -e

SERVICE=querysheriff-collector.service

case "$1" in
  remove)
    if [ -d /run/systemd/system ]; then
      systemctl disable "$SERVICE" || true
      systemctl daemon-reload || true
    fi
    ;;
  purge)
    rm -f /etc/querysheriff-collector.yml /etc/querysheriff-collector.yml.example
    rm -rf /var/lib/querysheriff-collector

    if getent passwd querysheriff-collector >/dev/null 2>&1; then
      deluser --system --quiet querysheriff-collector || true
    fi
    if getent group querysheriff-collector >/dev/null 2>&1; then
      delgroup --system --quiet querysheriff-collector || true
    fi

    if [ -d /run/systemd/system ]; then
      systemctl daemon-reload || true
    fi
    ;;
esac

exit 0
