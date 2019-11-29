#!/bin/sh

if command -v setsid 2>/dev/null; then
    setsid sh -c "sleep 3"
else  # osx
    set -m
    sh -c "sleep 3"
fi
