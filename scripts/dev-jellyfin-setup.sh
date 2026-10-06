#!/usr/bin/env bash
# Completes the Jellyfin startup wizard for the compose dev stack, adds the
# dev/media libraries, and writes an API key to .env for jellex.
set -euo pipefail

JF=${JF:-http://localhost:8096}
USER=${JF_USER:-admin}
PASS=${JF_PASS:-admin}
AUTH='MediaBrowser Client="jellex-dev", Device="script", DeviceId="jellex-dev-setup", Version="1"'
cd "$(dirname "$0")/.."

curl -fsS "$JF/System/Info/Public" >/dev/null || { echo "jellyfin not reachable at $JF" >&2; exit 1; }

if [ "$(curl -fsS "$JF/System/Info/Public" | jq -r .StartupWizardCompleted)" != "true" ]; then
  echo "running startup wizard"
  # The first user is created lazily; GET it once before updating it.
  curl -fsS "$JF/Startup/User" >/dev/null
  curl -fsS -X POST "$JF/Startup/Configuration" -H 'Content-Type: application/json' \
    -d '{"UICulture":"en-US","MetadataCountryCode":"US","PreferredMetadataLanguage":"en"}'
  curl -fsS -X POST "$JF/Startup/User" -H 'Content-Type: application/json' \
    -d "{\"Name\":\"$USER\",\"Password\":\"$PASS\"}"
  curl -fsS -X POST "$JF/Startup/RemoteAccess" -H 'Content-Type: application/json' \
    -d '{"EnableRemoteAccess":true,"EnableAutomaticPortMapping":false}'
  curl -fsS -X POST "$JF/Startup/Complete"
fi

TOKEN=$(curl -fsS -X POST "$JF/Users/AuthenticateByName" -H "Authorization: $AUTH" \
  -H 'Content-Type: application/json' -d "{\"Username\":\"$USER\",\"Pw\":\"$PASS\"}" | jq -r .AccessToken)
H="Authorization: MediaBrowser Token=\"$TOKEN\""

existing=$(curl -fsS "$JF/Library/VirtualFolders" -H "$H" | jq -r '.[].Name')
add_lib() { # name collectionType path
  grep -qx "$1" <<<"$existing" && return
  echo "adding library $1"
  curl -fsS -X POST "$JF/Library/VirtualFolders?name=$1&collectionType=$2&refreshLibrary=true" \
    -H "$H" -H 'Content-Type: application/json' -d "{\"LibraryOptions\":{\"PathInfos\":[{\"Path\":\"$3\"}]}}"
}
add_lib Movies movies /media/movies
add_lib Shows tvshows /media/tv
add_lib Music music /media/music

# Libraries added moments after their media is written can come up empty, so
# rescan everything once.
curl -fsS -X POST "$JF/Library/Refresh" -H "$H"

# Trickplay (seek preview thumbnails) for the video libraries. The dev clips
# are only 20-30s long, so use a 1s interval instead of Jellyfin's 10s.
curl -fsS "$JF/System/Configuration" -H "$H" |
  jq '.TrickplayOptions.Interval = 1000' |
  curl -fsS -X POST "$JF/System/Configuration" -H "$H" -H 'Content-Type: application/json' -d @-
curl -fsS "$JF/Library/VirtualFolders" -H "$H" |
  jq -c '.[] | select(.CollectionType=="movies" or .CollectionType=="tvshows") |
    {Id: .ItemId, LibraryOptions: (.LibraryOptions + {EnableTrickplayImageExtraction: true, ExtractTrickplayImagesDuringLibraryScan: true})}' |
  while read -r body; do
    curl -fsS -X POST "$JF/Library/VirtualFolders/LibraryOptions" -H "$H" -H 'Content-Type: application/json' -d "$body"
  done
task=$(curl -fsS "$JF/ScheduledTasks" -H "$H" | jq -r '.[] | select(.Key=="RefreshTrickplayImages") | .Id')
if [ -n "$task" ]; then
  curl -fsS -X POST "$JF/ScheduledTasks/Running/$task" -H "$H"
  echo "started trickplay generation"
fi

# A second, restricted user (guest/guest) that can only see Movies, for
# testing per-user sign-in.
gid=$(curl -fsS "$JF/Users" -H "$H" | jq -r '.[] | select(.Name=="guest") | .Id')
if [ -z "$gid" ]; then
  gid=$(curl -fsS -X POST "$JF/Users/New" -H "$H" -H 'Content-Type: application/json' \
    -d '{"Name":"guest","Password":"guest"}' | jq -r .Id)
  movies=$(curl -fsS "$JF/Library/VirtualFolders" -H "$H" | jq -r '.[] | select(.CollectionType=="movies") | .ItemId')
  curl -fsS "$JF/Users/$gid" -H "$H" | jq ".Policy | .EnableAllFolders=false | .EnabledFolders=[\"$movies\"]" |
    curl -fsS -X POST "$JF/Users/$gid/Policy" -H "$H" -H 'Content-Type: application/json' -d @-
  echo "created user guest (Movies only)"
fi

# A collection of the Blender films, for testing collections.
uid=$(curl -fsS "$JF/Users" -H "$H" | jq -r '.[0].Id')
if [ -z "$(curl -fsS "$JF/Items?userId=$uid&Recursive=true&IncludeItemTypes=BoxSet" -H "$H" | jq -r '.Items[] | select(.Name=="Blender Open Movies") | .Id')" ]; then
  ids=$(curl -fsS "$JF/Items?userId=$uid&Recursive=true&IncludeItemTypes=Movie" -H "$H" |
    jq -r '[.Items[] | select(.Name | test("Bunny|Sintel|Tears of Steel|Elephants|Cosmos|Caminandes")) | .Id] | join(",")')
  if [ -n "$ids" ]; then
    curl -fsS -X POST "$JF/Collections?name=Blender%20Open%20Movies&ids=$ids" -H "$H" >/dev/null
    echo "created collection Blender Open Movies"
  fi
fi

key=$(curl -fsS "$JF/Auth/Keys" -H "$H" | jq -r '.Items[] | select(.AppName=="jellex") | .AccessToken' | head -1)
if [ -z "$key" ]; then
  curl -fsS -X POST "$JF/Auth/Keys?app=jellex" -H "$H"
  key=$(curl -fsS "$JF/Auth/Keys" -H "$H" | jq -r '.Items[] | select(.AppName=="jellex") | .AccessToken' | head -1)
fi
touch .env
grep -v '^JELLYFIN_API_KEY=' .env > .env.tmp || true
echo "JELLYFIN_API_KEY=$key" >> .env.tmp && mv .env.tmp .env
echo "wrote JELLYFIN_API_KEY to .env"
