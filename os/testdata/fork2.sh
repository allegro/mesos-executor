#!/bin/sh

set -m
sleep 3 &
python -c "import time; time.sleep(3)" &
(sh -c "(sleep 3)") || true
