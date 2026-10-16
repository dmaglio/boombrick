#!/usr/bin/env bash
# Fetches the fonts Boombrick bundles, pinned to fixed upstream commits.
# Cormorant Garamond comes from google/fonts as variable fonts and is cut into
# static instances with fonttools, which scripts/setup-wsl.sh installs. Lora
# ships static TTFs from its authors, so it is bundled as published: its
# licence reserves the name "Lora", which a modified font may not keep.
set -euo pipefail

out="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/assets/fonts"
ft="$HOME/.venvs/fonttools/bin/fonttools"
gf=https://raw.githubusercontent.com/google/fonts/3dd78844021e948ceb633d1dcee3f7885561b5d9/ofl/cormorantgaramond
lora=https://raw.githubusercontent.com/cyrealtype/Lora-Cyrillic/0caf2e6389163dd49e5f973d7ed8bf42e60d83a2
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
mkdir -p "$out"

curl -fsSL -o "$tmp/cormorant.ttf"        "$gf/CormorantGaramond%5Bwght%5D.ttf"
curl -fsSL -o "$tmp/cormorant-italic.ttf" "$gf/CormorantGaramond-Italic%5Bwght%5D.ttf"
curl -fsSL -o "$out/OFL-CormorantGaramond.txt" "$gf/OFL.txt"

instance() { "$ft" varLib.instancer "$tmp/$1" "wght=$2" -o "$out/$3"; }
instance cormorant.ttf        400 CormorantGaramond-Regular.ttf
instance cormorant.ttf        600 CormorantGaramond-SemiBold.ttf
instance cormorant-italic.ttf 400 CormorantGaramond-Italic.ttf

for f in Regular SemiBold Italic; do
  curl -fsSL -o "$out/Lora-$f.ttf" "$lora/fonts/ttf/Lora-$f.ttf"
done
curl -fsSL -o "$out/OFL-Lora.txt" "$lora/OFL.txt"

ls -l "$out"
