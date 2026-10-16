#!/usr/bin/env bash
# Generates the short tones the decoder tests read. They are kept out of git;
# tests that need them skip until this has run.
set -euo pipefail

out="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/internal/audio/decode/testdata"
mkdir -p "$out"

tone() { ffmpeg -loglevel error -y -f lavfi -i "sine=frequency=440:sample_rate=$1:duration=1" "${@:2}"; }
tone 44100 -ac 2 -c:a libmp3lame -b:a 192k "$out/tone-44100.mp3"
tone 44100 -ac 2 -c:a libmp3lame -q:a 4 "$out/vbr-44100.mp3"
tone 44100 -ac 2 -c:a libmp3lame -b:a 128k -write_xing 0 "$out/noxing-44100.mp3"
# Noise shows a misplaced seek. The long one carries an ID3v1 tag after the
# audio as well as the LAME tag in front.
noise() { ffmpeg -loglevel error -y -f lavfi -i "anoisesrc=color=pink:sample_rate=$1:amplitude=0.3:seed=7:duration=$2" "${@:3}"; }
noise 44100 180 -ac 2 -c:a libmp3lame -b:a 128k -write_id3v1 1 -metadata title=Noise "$out/noise-180s.mp3"
noise 22050 60 -ac 1 -c:a libmp3lame -b:a 64k "$out/noise-60s-22050.mp3"
# Encodings that differ in how far a frame reaches back into earlier ones.
noise 44100 60 -ac 2 -c:a libmp3lame -b:a 320k "$out/noise-320k.mp3"
noise 44100 60 -ac 2 -c:a libmp3lame -b:a 128k -reservoir 0 "$out/noise-reservoir0.mp3"
noise 48000 45 -ac 2 -c:a libmp3lame -b:a 256k "$out/noise-256k-48000.mp3"
noise 44100 45 -ac 2 -c:a libmp3lame -q:a 0 "$out/noise-v0.mp3"
noise 44100 45 -ac 2 -c:a libmp3lame -q:a 4 "$out/noise-v4.mp3"
noise 11025 45 -ac 2 -c:a libmp3lame -b:a 16k "$out/noise-11025.mp3"
noise 8000 45 -ac 1 -c:a libmp3lame -b:a 8k "$out/noise-8000.mp3"
# No LAME tag, so no delay or padding to trim, and no ID3 tag either.
noise 44100 45 -ac 2 -c:a libmp3lame -b:a 128k -write_xing 0 -id3v2_version 0 "$out/noise-untagged.mp3"
# Cut part-way through a frame, as a recording of a stream can start: the
# first whole frames reach back into audio that isn't there.
tail -c +200001 "$out/noise-v4.mp3" > "$out/noise-cut.mp3"
# A stream cut at a frame and joined on to one encoded without a reservoir,
# so its first frame reaches back for data that isn't there part-way through.
noise 44100 20 -ac 2 -c:a libmp3lame -b:a 128k -reservoir 0 -write_xing 0 -id3v2_version 0 "$out/joined-a.mp3"
ffmpeg -loglevel error -y -ss 20 -i "$out/noise-untagged.mp3" -c:a copy -write_xing 0 -id3v2_version 0 "$out/joined-b.mp3"
cat "$out/joined-a.mp3" "$out/joined-b.mp3" > "$out/noise-joined.mp3"
rm "$out/joined-a.mp3" "$out/joined-b.mp3"
tone 48000 -ac 2 -c:a flac "$out/tone-48000.flac"
tone 44100 -ac 1 -c:a flac "$out/mono-44100.flac"

# Channels that differ, so a swapped or mis-strided conversion shows up.
sines() {
	local n=$# i inputs=()
	for i in "$@"; do inputs+=(-f lavfi -i "sine=frequency=$i:sample_rate=44100:duration=1"); done
	ffmpeg -loglevel error -y "${inputs[@]}" -filter_complex "amerge=inputs=$n" -c:a flac "$out/$OUT"
}
OUT=stereo-440-880.flac sines 440 880
OUT=three-440-880-220.flac sines 440 880 220
tone 48000 -ac 2 -c:a libopus -b:a 96k "$out/tone-48000.opus"
ffmpeg -loglevel error -y -f lavfi -i anullsrc=r=44100:cl=mono -f lavfi -i anullsrc=r=44100:cl=mono \
  -f lavfi -i "sine=frequency=440:sample_rate=44100:duration=1" \
  -filter_complex "[0][1][2]amerge=inputs=3[a]" -map "[a]" -t 1 -c:a flac "$out/centre-only.flac"
tone 44100 -ac 2 -c:a aac -b:a 128k "$out/tone-44100.m4a"
tone 22050 -ac 1 -c:a aac -b:a 64k "$out/mono-22050.m4a"
# Noise fills the high band that SBR rebuilds, so a seek that decodes it
# wrongly shows where a pure tone hides it.
ffmpeg -loglevel error -y -f lavfi -i "anoisesrc=color=pink:sample_rate=22050:amplitude=0.3:duration=1" \
  -ac 1 -c:a aac -b:a 64k "$out/noise-22050.m4a"
# ffmpeg's config says "no SBR"; flag SBR at 44.1 kHz instead, so faad2
# decodes these 22.05 kHz streams at double rate as it does HE-AAC.
python3 - "$out/mono-22050.m4a" "$out/noise-22050.m4a" <<'PY'
import sys
for p in sys.argv[1:]:
    d = open(p, "rb").read()
    old = bytes.fromhex("138856e500")
    assert d.count(old) == 1
    open(p, "wb").write(d.replace(old, bytes.fromhex("138856e5a0")))
PY
tone 44100 -ac 2 -c:a alac "$out/alac.m4a"
# A length that isn't a whole number of milliseconds, and a file with its
# index up front so a cut in the audio leaves it readable.
# Noise substitution is off: faad2 makes its noise from what it decoded
# before, so seeks into such frames can't match a straight decode.
tone 44100 -af atrim=end_sample=44022 -ac 2 -c:a aac -aac_pns 0 -b:a 128k "$out/odd-44100.m4a"
tone 44100 -ac 2 -c:a aac -b:a 128k -movflags +faststart "$out/faststart-44100.m4a"

lib="$(dirname "$out")/../../library/testdata"
mkdir -p "$lib"
meta=(-metadata title=Rust -metadata artist=Odalys -metadata album_artist="Various Artists"
  -metadata album="Copper Season" -metadata track=3/10 -metadata disc=2/2 -metadata date=2019
  -metadata REPLAYGAIN_TRACK_GAIN="-6.50 dB" -metadata REPLAYGAIN_TRACK_PEAK=0.900000
  -metadata REPLAYGAIN_ALBUM_GAIN="-7.25 dB" -metadata REPLAYGAIN_ALBUM_PEAK=0.950000)
tone 44100 -ac 2 -c:a libmp3lame -b:a 128k -id3v2_version 3 "${meta[@]}" "$lib/tagged.mp3"
tone 44100 -ac 2 -c:a flac "${meta[@]}" "$lib/tagged.flac"
tone 48000 -ac 2 -c:a libopus -b:a 96k "${meta[@]}" "$lib/tagged.opus"
tone 44100 -ac 2 -c:a aac -b:a 128k "${meta[@]}" "$lib/tagged.m4a"
ls -l "$lib"

ls -l "$out"
