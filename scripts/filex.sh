#!/bin/bash
set -e

FILEX_HOME="${HOME}/.filex"
FILEX_CURRENT="${FILEX_HOME}/current"

if [ ! -f "${FILEX_CURRENT}/filex" ]; then
    echo "Error: FileX current version not found"
    echo "Please run: filex upgrade"
    exit 1
fi

exec "${FILEX_CURRENT}/filex" "$@"
