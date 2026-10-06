#!/usr/bin/env bash
# Generates the dev test library in dev/media: short synthetic clips named
# after public-domain / Blender films and old TV so Jellyfin matches real
# metadata and artwork. Existing files are left alone.
set -euo pipefail
cd "$(dirname "$0")/../dev/media"
mkdir -p movies tv music

video() { # out seconds width height
  [ -f "$1" ] && return
  mkdir -p "$(dirname "$1")"
  ffmpeg -nostdin -loglevel error \
    -f lavfi -i "testsrc2=size=${3}x${4}:rate=24:duration=$2" \
    -f lavfi -i "sine=frequency=440:duration=$2" \
    -c:v libx264 -preset ultrafast -pix_fmt yuv420p -c:a aac -b:a 96k -shortest "$1"
}

for m in "Big Buck Bunny (2008)" "Sintel (2010)" "Tears of Steel (2012)" "Elephants Dream (2006)" \
  "Night of the Living Dead (1968)" "Nosferatu (1922)" "The General (1926)" "Metropolis (1927)"; do
  video "movies/$m/$m.mp4" 30 1280 720 &
done
for s in 01 02; do for e in 01 02 03; do
  video "tv/The Twilight Zone (1959)/Season $s/The Twilight Zone (1959) - S${s}E${e}.mp4" 20 640 480 &
done; done
for e in 01 02 03 04; do
  video "tv/The Lucy Show (1962)/Season 01/The Lucy Show (1962) - S01E${e}.mp4" 20 640 480 &
done
wait

# Extras for Sintel: a local trailer and a featurette, in the layouts
# Jellyfin recognizes.
video "movies/Sintel (2010)/Sintel (2010)-trailer.mp4" 10 1280 720
video "movies/Sintel (2010)/featurettes/Making Of.mp4" 10 1280 720

i=1
for t in Morning Afternoon Evening; do
  f="music/Test Artist/Test Album (2020)/0$i - $t.mp3"
  mkdir -p "$(dirname "$f")"
  [ -f "$f" ] || ffmpeg -nostdin -loglevel error -f lavfi -i "sine=frequency=$((300 + i * 110)):duration=30" \
    -c:a libmp3lame -b:a 128k -metadata artist="Test Artist" -metadata album="Test Album" \
    -metadata title="$t" -metadata track=$i -metadata date=2020 "$f"
  i=$((i + 1))
done

# A movie exercising tracks and chapters: two audio languages, two embedded
# subtitle tracks, an external .srt, and chapters named Intro/Credits (which
# jellex turns into skip markers when Jellyfin has no media segments).
d="movies/Cosmos Laundromat (2015)"
f="$d/Cosmos Laundromat (2015).mp4"
if [ ! -f "$f" ]; then
  mkdir -p "$d"
  tmp=$(mktemp -d)
  sub() { # file lang
    printf '1\n00:00:02,000 --> 00:00:08,000\n[%s] Opening line\n\n2\n00:00:15,000 --> 00:00:25,000\n[%s] Middle line\n\n3\n00:00:40,000 --> 00:00:50,000\n[%s] Closing line\n' "$2" "$2" "$2" >"$1"
  }
  sub "$tmp/en.srt" en
  sub "$tmp/fr.srt" fr
  cat >"$tmp/chapters.txt" <<'CHAPTERS'
;FFMETADATA1
[CHAPTER]
TIMEBASE=1/1000
START=0
END=10000
title=Intro
[CHAPTER]
TIMEBASE=1/1000
START=10000
END=50000
title=Story
[CHAPTER]
TIMEBASE=1/1000
START=50000
END=60000
title=Credits
CHAPTERS
  ffmpeg -nostdin -loglevel error \
    -f lavfi -i "testsrc2=size=1280x720:rate=24:duration=60" \
    -f lavfi -i "sine=frequency=330:duration=60" \
    -f lavfi -i "sine=frequency=550:duration=60" \
    -i "$tmp/en.srt" -i "$tmp/fr.srt" -i "$tmp/chapters.txt" \
    -map 0:v -map 1:a -map 2:a -map 3 -map 4 -map_metadata 5 -map_chapters 5 \
    -c:v libx264 -preset ultrafast -pix_fmt yuv420p -c:a aac -b:a 96k -c:s mov_text \
    -metadata:s:a:0 language=eng -metadata:s:a:0 title=English \
    -metadata:s:a:1 language=fra -metadata:s:a:1 title=Français \
    -metadata:s:s:0 language=eng -metadata:s:s:1 language=fra \
    -disposition:a:0 default -disposition:a:1 0 -disposition:s:0 0 -disposition:s:1 0 \
    -t 60 "$f"
  sub "$d/Cosmos Laundromat (2015).de.srt" de
  rm -rf "$tmp"
fi

# A movie browsers can't direct play (HEVC + AC-3 in MKV), so Plex clients
# have to use jellex's transcoder.
f="movies/Caminandes Llama Drama (2013)/Caminandes Llama Drama (2013).mkv"
if [ ! -f "$f" ]; then
  mkdir -p "$(dirname "$f")"
  ffmpeg -nostdin -loglevel error \
    -f lavfi -i "testsrc2=size=1280x720:rate=24:duration=30" \
    -f lavfi -i "sine=frequency=440:duration=30" \
    -c:v libx265 -preset ultrafast -x265-params log-level=error -pix_fmt yuv420p \
    -c:a ac3 -b:a 192k -ac 2 -shortest "$f"
fi

echo "dev media ready: $(find . -type f | wc -l) files"
