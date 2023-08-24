#!/bin/bash
set -euf -o pipefail
if [[ -z "${BRIDGE_NAME:-}" ]]; then
	if [[ ! -z "$1" ]]; then
		export BRIDGE_NAME="$1"
	else
		echo "BRIDGE_NAME not set"
		exit 1
	fi
fi
export BBCTL_CONFIG=${BBCTL_CONFIG:-/tmp/bbctl.json}
export BEEPER_ENV=${BEEPER_ENV:-prod}
if [[ ! -f $BBCTL_CONFIG ]]; then
	if [[ -z "$MATRIX_ACCESS_TOKEN" ]]; then
		echo "MATRIX_ACCESS_TOKEN not set"
		exit 1
	fi
	export DATA_DIR=${DATA_DIR:-/data}
	if [[ ! -d $DATA_DIR ]]; then
		echo "DATA_DIR ($DATA_DIR) does not exist, creating"
		mkdir -p $DATA_DIR
	fi
	export DB_DIR=${DB_DIR:-/data/db}
	mkdir -p $DB_DIR
	jq -n '{environments: {"\(env.BEEPER_ENV)": {access_token: env.MATRIX_ACCESS_TOKEN, database_dir: env.DB_DIR, bridge_data_dir: env.DATA_DIR}}}' > $BBCTL_CONFIG
fi
bbctl -e $BEEPER_ENV run $BRIDGE_NAME
