#!/bin/sh

set -m
sleep 5 &
python -c "import time; time.sleep(5)" &
(sh -c "(sleep 5)") || true
