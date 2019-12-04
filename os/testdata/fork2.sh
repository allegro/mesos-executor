#!/bin/sh

if command -v setsid 2>/dev/null; then
    setsid sleep 3 &
    setsid python -c "import time; time.sleep(3)" &
    setsid sh -c "sleep 3"
else  # osx
    set -m
    sleep 3 &
    python -c "import time; time.sleep(3)" &
    sh -c "sleep 3"
fi
