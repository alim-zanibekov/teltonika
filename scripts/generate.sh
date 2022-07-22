#! /usr/bin/env sh

DIR=$(dirname "$0")
OUT_FILE=$(realpath "$DIR/../ioelements/ioelements_dump.go")

$DIR/elements_dump.py '["Permanent I/O elements", "Eventual I/O elements", "Bluetooth Low Energy"]' > $OUT_FILE
