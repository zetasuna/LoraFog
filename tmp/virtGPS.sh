#!/bin/sh

# sudo apt install -y socat
socat -d -d \
  pty,raw,echo=0,link=/tmp/ttyV0 \
  pty,raw,echo=0,link=/tmp/ttyV12 &
socat -d -d \
  pty,raw,echo=0,link=/tmp/ttyV1 \
  pty,raw,echo=0,link=/tmp/ttyV11 &
socat -d -d \
  pty,raw,echo=0,link=/tmp/ttyV2 \
  pty,raw,echo=0,link=/tmp/ttyV21 &
