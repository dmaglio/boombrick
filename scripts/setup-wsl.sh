#!/usr/bin/env bash
# Prepares a fresh WSL Ubuntu 24.04 for Boombrick development.
set -euo pipefail

sudo apt-get update
sudo apt-get install -y build-essential pkg-config curl zip file python3-venv \
  libsdl2-dev libsdl2-image-dev libasound2-dev

go_version="$(curl -fsSL 'https://go.dev/VERSION?m=text' | head -n1)"
curl -fsSL "https://go.dev/dl/${go_version}.linux-amd64.tar.gz" -o /tmp/go.tgz
sudo rm -rf /usr/local/go
sudo tar -C /usr/local -xzf /tmp/go.tgz
rm /tmp/go.tgz
grep -q '/usr/local/go/bin' ~/.profile || echo 'export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin' >> ~/.profile

python3 -m venv ~/.venvs/fonttools
~/.venvs/fonttools/bin/pip install --quiet fonttools

/usr/local/go/bin/go version
