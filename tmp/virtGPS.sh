#!/bin/sh

# sudo apt install -y socat
socat -d -d \
  pty,raw,echo=0,link=/tmp/ttyGW1 \
  pty,raw,echo=0,link=/tmp/ttyLR1 &
socat -d -d \
  pty,raw,echo=0,link=/tmp/ttyGW2 \
  pty,raw,echo=0,link=/tmp/ttyLR2 &
socat -d -d \
  pty,raw,echo=0,link=/tmp/ttyGPSS1 \
  pty,raw,echo=0,link=/tmp/ttyGPSR1 &
socat -d -d \
  pty,raw,echo=0,link=/tmp/ttyGPSS2 \
  pty,raw,echo=0,link=/tmp/ttyGPSR2 &
