#!/bin/bash

# install teamserver build dependencies.
# the musl.cc cross toolchains this script used to download are no longer
# available; the system mingw-w64 packages are used instead (make sure the
# Teamserver.Build compiler paths in your profile point to them, e.g.
# /usr/bin/x86_64-w64-mingw32-gcc — the shipped profiles already do).

SUDO=""
if [ "$(id -u)" -ne 0 ]; then
	SUDO="sudo"
fi

$SUDO apt -qq update
$SUDO apt -qq --yes install golang-go nasm mingw-w64 wget
