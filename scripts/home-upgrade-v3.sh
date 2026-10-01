#!/bin/sh
exec /usr/bin/env -i PATH=/usr/sbin:/usr/bin:/sbin:/bin /bin/bash --noprofile --norc "${0%/*}/home-upgrade-v3.bash" "$@"
