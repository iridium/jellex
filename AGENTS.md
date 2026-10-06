# jellex

jellex emulates the Plex Media Server API on top of Jellyfin, so Plex clients
(starting with Plex Web) can browse and play a Jellyfin library.

## Layout

- `cmd/jellex` – entry point.
- `internal/config` – settings from environment variables.
- `internal/plex` – the emulated PMS API. `element.go` builds responses that
  encode as XML or JSON; `metadata.go` maps Jellyfin items to Plex metadata.
- `internal/ids` – persistent Jellyfin GUID ↔ Plex integer ID map. Besides
  items it holds parts, media, streams and playlist entries under prefixed
  keys (`part:…`, `stream:…`, `pli:…`); use `Server.itemGUID` when a
  ratingKey must be an item.
- `internal/fmp4` – splits muxed fragmented MP4 into per-track streams (used
  by the transcoder, see below).
- `internal/myplex` – plex.tv claim and publish (experimental).
- `internal/webui` – downloads the Plex Web client from a pinned PMS `.deb`
  on first start and serves it at `/web`.
- `ref/` (gitignored) – reference material: the PMS `.deb` and its unpacked
  contents, `plex-api-spec`, and `jellyfin-openapi.json`. The PMS package
  contains real sample responses under
  `Resources/Assets/JSON/MediaProviderResponses/`.

## Dev environment

Credentials are deliberately simple; this is a local dev stack only.

| What | Value |
| --- | --- |
| Jellyfin URL | http://localhost:8096 |
| Jellyfin admin | `admin` / `admin` |
| Jellyfin API key | in `.env` as `JELLYFIN_API_KEY` (app name `jellex`) |
| jellex (compose) | http://localhost:32400/web |

Any new dev accounts or secrets should also be simple (e.g. `admin`/`admin`)
and recorded in this table.

There are two compose files:

- `compose.dev.yaml` – jellex plus a local Jellyfin serving `dev/media`.
  `.env` sets `COMPOSE_FILE=compose.dev.yaml`, so plain `docker compose …`
  uses it.
- `compose.prod.yaml` – jellex only, for a Jellyfin running elsewhere;
  requires `JELLYFIN_URL` and `JELLYFIN_API_KEY`. Run it with
  `docker compose -f compose.prod.yaml up -d`.

Dev setup from scratch:

0. `echo COMPOSE_FILE=compose.dev.yaml >> .env`
1. `docker compose up -d jellyfin`
2. `./scripts/dev-media.sh` – generates the test library in `dev/media`
   (gitignored): short synthetic clips named after public-domain and
   Blender films so Jellyfin matches real metadata, plus special cases: a
   movie with two audio languages, embedded and external subtitles and
   chapters (Cosmos Laundromat), one with a trailer and featurette (Sintel),
   and an HEVC/AC-3 MKV that needs transcoding (Caminandes).
3. `./scripts/dev-jellyfin-setup.sh` – runs the Jellyfin startup wizard,
   creates the admin user and libraries, and writes the API key to `.env`.
4. `docker compose up` (or `docker compose watch`, which rebuilds jellex on
   every source change).

## Environment variables

| Variable | Default | Meaning |
| --- | --- | --- |
| `JELLYFIN_URL` | required | Upstream Jellyfin |
| `JELLYFIN_API_KEY` | | Jellyfin API key |
| `JELLYFIN_USER` | first admin | Jellyfin user every Plex client acts as |
| `JELLEX_LISTEN_ADDR` | `:32400` | Listen address |
| `JELLEX_SERVER_NAME` | `jellex` | Name shown to clients |
| `JELLEX_MACHINE_ID` | derived from Jellyfin server ID | Server identity; must differ between instances clients can see |
| `JELLEX_WEB_DIR` | user cache dir | Plex Web client cache |
| `JELLEX_DATA_DIR` | user config dir | jellex state (ID map, plex.tv claim) |
| `JELLEX_PUBLISH_URLS` | | Comma-separated URLs published to plex.tv |
| `PLEX_CLAIM` | | One-time claim token from https://plex.tv/claim |
| `JELLEX_DISABLE_CUSTOM_ASSETS` | `false` | Serve Plex Web exactly as shipped, without the jellex favicon and wordmark |

## Notes

- Plex Web signs in with a real plex.tv account. jellex is claimed to that
  account (`POST /myplex/claim?token=…` or `PLEX_CLAIM`, token from
  https://plex.tv/claim) and publishes `JELLEX_PUBLISH_URLS` to plex.tv so
  apps find it. Tokens aren't checked yet; every request acts as one
  Jellyfin user.
- Plex Web also probes `127.0.0.1:32400`. When running a second jellex for
  testing, give it a different `JELLEX_MACHINE_ID`, or the client confuses the
  two.
- Unhandled routes are logged at INFO as `unhandled` with the Plex client
  query params stripped; that log is the to-do list.
- Never change how the Plex Web client behaves: no patching its JS and no
  injected scripts. Its behavior is driven only by what the API returns.
  Cosmetic branding is allowed, and lives entirely in
  `internal/webui/branding.go`: the favicon is the jellex logo, and
  `index.html` gets a stylesheet link that swaps the top-bar Plex wordmark
  for `assets/wordmark.svg`. Keep branding to CSS and images. Its selectors
  target the pinned client, so check the top bar after bumping
  `PMSVersion`. `JELLEX_DISABLE_CUSTOM_ASSETS=true` turns all of it off. The
  cached client on disk is never modified.
- Scope is a single user's view of Jellyfin, not server administration.
  `/media/providers` omits the `manage` and `match` features, which makes
  Plex Web hide metadata editing, matching and deletion. Per-user actions
  (watched state, progress, ratings) are written back to Jellyfin.
- The server always reports `myPlexSigninState=ok` so Plex Web doesn't block
  pages with an "unclaimed server" notice; `myPlex`, `claimed` and
  `myPlexUsername` reflect the real claim.
- Playback: when the client offers direct play (`directPlay=1` on the
  decision), `/library/parts/...` proxies Jellyfin's static stream with Range
  support. Otherwise jellex transcodes (`transcode.go`): Plex Web in Chrome
  only does DASH, Jellyfin only does HLS. `start.mpd` starts a Jellyfin HLS
  transcode with fMP4 segments, reads its playlist and writes a DASH
  manifest; segments proxy to Jellyfin's. Shaka rejects muxed audio+video
  in DASH, so `internal/fmp4` splits each segment into a video and an audio
  stream. Audio track and burned-in subtitle come from the part's selected
  streams; `stop` (or 2 minutes idle) kills Jellyfin's encoder.
- Subtitles in direct play go through
  `/subtitles/:/transcode/universal/start`, which Plex Web renders with
  libjass and parses as ASS (not WebVTT). Jellyfin's ASS output lacks
  `PlayResX/Y`, which jellex adds or the text renders tiny in a corner.
- Stream selections (`PUT /library/parts/{id}`), play queues, transcode
  sessions and now-playing sessions are in memory and lost on restart.
- Some Plex Web features are gated by plex.tv account feature flags, not by
  the server: e.g. the Skip Intro/Credits buttons need `intro-markers` /
  `credits-markers` on the signed-in account.
- Jellyfin endpoints that take the user from the auth token (playlist move
  and update) fail with an API key ("Guid can't be empty"); jellex works
  around them with admin-capable endpoints.

## Testing against Plex Web

A headless Chromium run is the fastest feedback loop: load
`/web/index.html#!/...`, record requests to the server that fail, and
screenshot. Playwright's `chrome-headless-shell` decodes H.264/AAC, so the
dev media plays, and it can't decode HEVC, so the Caminandes file exercises
transcoding.

Plex Web now requires a plex.tv sign-in, so a fresh headless profile stops
at the login page; reuse a profile that has signed in (or has
`skipSignIn=1` in localStorage).

Don't build shell heredocs that contain text with its own heredoc
terminator: a stray `EOF` ends the heredoc early and runs the rest as shell
commands. Use the editor tools for text edits.
