#!/bin/sh

set -e

SERVICE=querysheriff-collector.service

case "$1" in
  remove|deconfigure)
    if [ -d /run/systemd/system ]; then
      systemctl stop "$SERVICE" || true
    fi
    ;;
esac

exit 0
