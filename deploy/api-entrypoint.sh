#!/bin/sh
set -eu
/app/hadir migrate
exec /app/hadir serve
