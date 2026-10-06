# jellex

jellex emulates the Plex Media Server API on top of Jellyfin, so Plex clients
(starting with Plex Web) can browse and play a Jellyfin library.

## Layout

- `cmd/jellex` – entry point.
- `internal/config` – settings from environment variables.
- `internal/plex` – the emulated PMS API. `element.go` builds responses that
  encode as XML or JSON; `metadata.go` maps Jellyfin items to Plex metadata.
- `internal/store` – opens the SQLite database and creates its schema.
- `internal/ids` – persistent Jellyfin GUID ↔ Plex integer ID map. Besides
  items it holds parts, media, streams and playlist entries under prefixed
  keys (`part:…`, `stream:…`, `pli:…`); use `Server.itemGUID` when a
  ratingKey must be an item.
- `internal/auth` – Jellyfin sign-in (password and Quick Connect) and the
  persisted browser sessions; the login page and middleware are in
  `internal/plex/login.go`.
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
| Jellyfin restricted user (Movies only) | `guest` / `guest` |
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
   and an HEVC/AC-3 MKV that needs transcoding (Caminandes). It also adds
   bulk filler for pagination and infinite scroll: 1000 movies, 300 shows
   and 150 artists by default, all copies of one tiny clip (tune with
   `DEV_MEDIA_MOVIES`/`_SHOWS`/`_ARTISTS`, skip with `DEV_MEDIA_BULK=0`).
   The first Jellyfin scan of these takes a while.
3. `./scripts/dev-jellyfin-setup.sh` – runs the Jellyfin startup wizard,
   creates the admin user and libraries, and writes the API key to `.env`.
4. `docker compose up` (or `docker compose watch`, which rebuilds jellex on
   every source change).

## Environment variables

| Variable | Default | Meaning |
| --- | --- | --- |
| `JELLYFIN_URL` | required | Upstream Jellyfin |
| `JELLYFIN_API_KEY` | | Jellyfin API key |
| `JELLEX_DEV` | `false` | Dev mode: no sign-in, everyone acts as the first Jellyfin admin. Never in deployments |
| `JELLEX_LISTEN_ADDR` | `:32400` | Listen address |
| `JELLEX_SERVER_NAME` | Jellyfin's server name | Name shown to clients |
| `JELLEX_WEB_DIR` | user cache dir | Plex Web client cache |
| `JELLEX_DATA_DIR` | user config dir | jellex state (`jellex.db`) |
| `JELLEX_DISABLE_CUSTOM_ASSETS` | `false` | Serve Plex Web exactly as shipped: no patches, branding or sign-in skip |

## Notes

- Sign-in: Plex Web runs without a plex.tv account (patched); jellex asks for
  a Jellyfin login at `/web/login` (password or Quick Connect), keeps the
  user's own Jellyfin token in a persisted session, and sets an HttpOnly
  cookie. Everything Plex Web loads is same-origin, so the cookie rides
  along; each request then acts as that Jellyfin user (`Server.user`). The
  Plex Web static files are public; the page that boots it and the whole
  API need a session. Sign out at `/web/logout`. Only Plex Web is supported:
  the native apps authenticate through plex.tv, which is out of scope for
  now. Sign-in is always on except in dev mode (`JELLEX_DEV=true`), where
  everyone acts as the first Jellyfin admin; handy for headless tests,
  never for deployments.
- The server identity (`machineIdentifier`) is generated on first start and
  kept in `jellex.db`, so a second jellex for testing just needs its own
  `JELLEX_DATA_DIR`; Plex Web keeps the two apart by that ID.
- Unhandled routes are logged at INFO as `unhandled` with the Plex client
  query params stripped; that log is the to-do list.
- Prefer making Plex Web behave correctly through what the API returns.
  When the client itself has to change, do it with a patch in
  `internal/webui/patches/` (format documented in `patch.go`): exact
  find/replace snippets with an expected match count, applied to a copy of
  the pristine client (`<web dir>-patched-<patch set hash>`, rebuilt when
  the patches change). Integrity (SRI) hashes are stripped from that copy so
  patched files load. Plex's orange accents are rewritten to Jellyfin's
  blue across all CSS/JS (`recolor` / `accentColors` in `patch.go`);
  they're hard-coded in hundreds of places, so that isn't done with patch
  blocks. A patch that no longer matches fails loudly at startup and jellex falls back to the unpatched client, so check the logs
  after bumping `PMSVersion`. Current patches: skipping plex.tv sign-in;
  branding (page title, "jellex" for "Plex Web" in settings, plus `jellexCSS` served by `branding.go`, which swaps the
  top-bar wordmark and hides the activity dashboard, account menu and server
  settings); no tracking (Sentry, analytics, Statsig in local mode); no
  server administration (`isFullOwnedServer` is always false, hiding Grant
  Access, play history, scans, Manage Library); no loopback discovery;
  Skip Intro/Credits without the plex.tv account features; and HLS instead
  of DASH for transcodes.
  Plex Web's page is also served with a Content-Security-Policy allowing
  connections only to jellex (and scripts from Google's Cast SDK), which
  blocks plex.tv outright. Don't stub plex.tv's connection test out of the
  client instead: that leaves plex.tv "pending" and new browsers hang
  before first-run setup. Always check a fresh browser profile after
  changing patches.
  `JELLEX_DISABLE_CUSTOM_ASSETS=true` serves the client exactly as shipped.
- Scope is a single user's view of Jellyfin, not server administration.
  `/media/providers` omits the `manage` and `match` features, which makes
  Plex Web hide metadata editing, matching and deletion. Per-user actions
  (watched state, progress, ratings) are written back to Jellyfin.
- The server always reports `myPlexSigninState=ok` so Plex Web doesn't block
  pages with an "unclaimed server" notice; it is never claimed to plex.tv.
- Playback: when the client offers direct play (`directPlay=1` on the
  decision), `/library/parts/...` proxies Jellyfin's static stream with Range
  support. Otherwise jellex transcodes (`transcode.go`) over HLS: Plex Web
  would use DASH in Chrome, but has an experimental HLS path (hls.js) that
  `70-hls.patch` makes unconditional. `start.m3u8` starts a Jellyfin HLS
  transcode with fMP4 segments and returns a master playlist; the media
  playlist lists every segment up front (VOD), so seeking just requests a
  later segment, and init and media segments pass through from Jellyfin.
  Audio track and burned-in subtitle come from the part's selected streams
  (Plex Web sends `subtitles=auto` for HLS, which jellex treats as burn);
  `stop` (or 2 minutes idle) kills Jellyfin's encoder.
- Subtitles in direct play go through
  `/subtitles/:/transcode/universal/start`, which Plex Web renders with
  libjass and parses as ASS (not WebVTT). Jellyfin's ASS output lacks
  `PlayResX/Y`, which jellex adds or the text renders tiny in a corner.
- Persistent state is one SQLite database, `jellex.db` in the data dir
  (`internal/store`, pure-Go `modernc.org/sqlite`, so builds stay
  `CGO_ENABLED=0`): the ID map (cached fully in memory), sign-in sessions,
  stream selections (`PUT /library/parts/{id}`) and play queues (pruned
  after 30 days). The schema is created on open (`schema` in `store.go`).
  Transcode and now-playing sessions are in memory and lost on restart.
- jellex has no users yet: breaking changes are fine. Don't write
  migrations or compatibility code for old data; change the schema and
  delete `jellex.db` if needed.
- Some Plex Web features are gated by plex.tv account feature flags, not by
  the server. With no account they're all off; the Skip Intro/Credits
  buttons (`intro-markers` / `credits-markers`) are patched back on.
- Jellyfin endpoints that take the user from the auth token (playlist move
  and update) fail with an API key ("Guid can't be empty"); jellex works
  around them with admin-capable endpoints.

## Testing against Plex Web

A headless Chromium run is the fastest feedback loop: load
`/web/index.html#!/...`, record requests to the server that fail, and
screenshot. Playwright's `chrome-headless-shell` decodes H.264/AAC, so the
dev media plays, and it can't decode HEVC, so the Caminandes file exercises
transcoding.

A fresh headless profile first gets jellex's login page (fill `#u`/`#p` and
submit, or run with `JELLEX_DEV=true` to skip it), then Plex Web's first-run setup ("Finish Setup"), then the app.
When checking whether a page loaded, don't only count library cards: the
setup wizard has none.

Don't build shell heredocs that contain text with its own heredoc
terminator: a stray `EOF` ends the heredoc early and runs the rest as shell
commands. Use the editor tools for text edits.
