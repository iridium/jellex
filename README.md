<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="assets/wordmark.svg">
    <img src="assets/wordmark-light.svg" alt="jellex" width="280">
  </picture>
</p>

> [!NOTE]
> **Not production software.** jellex was written entirely by an LLM
> (Claude) in about two hours of directed sessions. Nobody has reviewed it
> line by line, it has almost no tests, and it has not been used outside a dev
> setup. Expect bugs, missing features and rough edges. Don't point it at
> anything you can't afford to lose, and don't expose it to the internet.

jellex lets Plex clients use a [Jellyfin](https://jellyfin.org) server. It
speaks the Plex Media Server API, translates each request into Jellyfin API
calls, and serves the official Plex Web client, so you get the Plex interface
on top of your Jellyfin library.

Jellyfin stays the source of truth: libraries, metadata, artwork, watch
state, progress and ratings all live there. jellex stores almost nothing of
its own.

## What works

Tested against Plex Web 4.160 with a small library of movies, shows and music:

- Home screen: Continue Watching and Recently Added hubs, plus "see all"
- Library browsing for movies, shows (seasons and episodes) and music
  (artists, albums and tracks), with filters (unplayed, genre, year, decade,
  content rating), sorting, paging and the A–Z jump bar
- Detail pages with summary, genres, cast & crew (clickable, filters the
  library), media info, "up next" for shows, More Like This, trailers and
  extras
- Search
- Direct play of video and audio, with seeking, watch progress, mark
  watched/unwatched (including whole seasons and shows), remove from
  Continue Watching, and star ratings, all saved back to Jellyfin
- Transcoding through Jellyfin for files the browser can't play (e.g. HEVC,
  AC-3, MKV), for picking a non-default audio track, and for burning in
  subtitles; seeking works within transcodes
- Subtitles: external and embedded text subtitles shown over direct play,
  or burned in when transcoding
- Chapters, and intro/credits markers from Jellyfin media segments or from
  chapters named Intro/Credits
- Seek preview thumbnails from Jellyfin trickplay images
- Playlists: create, add to, reorder, rename, delete and play
- Collections (Jellyfin box sets), per library, browsable and playable
- Play queues for shows, seasons, albums and playlists, including "continue
  to the next episode"
- Now Playing on the server dashboard, with live updates

## What doesn't (yet)

- Auth: every client acts as one Jellyfin user. Claiming to plex.tv and
  publishing to it (so mobile and TV apps find the server) is partly built
  but not working yet.
- Skip Intro/Credits buttons only appear for plex.tv accounts that have that
  feature; jellex serves the markers either way.
- Picture-based subtitles (PGS, VobSub) only work when burned in.
- Stream selections and play queues are kept in memory and reset when jellex
  restarts.
- Admin features (metadata editing, matching, server settings) are out of
  scope on purpose.

## Running it

You need Docker and an existing Jellyfin server with an API key (Jellyfin
dashboard → API Keys).

```bash
cp .env.example .env
```

Set `JELLYFIN_URL` and `JELLYFIN_API_KEY` in `.env`, then:

```bash
docker compose -f compose.prod.yaml up -d
```

Then open http://localhost:32400/web. On first start jellex downloads the
Plex Web client from a pinned Plex Media Server release (about 80 MB), so the
page may answer 503 for a few seconds.

Plex Web requires signing in with a plex.tv account.

### Without Docker

jellex is a single static Go binary. Build it with Go 1.27 or newer:

```bash
CGO_ENABLED=0 go build -o jellex ./cmd/jellex
```

Run it from the directory with your `.env` (it's loaded automatically), or
pass the settings as environment variables, which take precedence:

```bash
JELLYFIN_URL=http://your-jellyfin:8096 JELLYFIN_API_KEY=your-api-key ./jellex
```

It listens on `:32400`. Without `JELLEX_DATA_DIR` and `JELLEX_WEB_DIR`, state
goes in `~/.config/jellex` and the Plex Web client in
`~/.cache/jellex/plex-web-<version>` (the platform's user config and cache
directories on macOS and Windows). The first start needs internet access to
download the Plex Web client.

### Configuration

All settings, with comments, are in [.env.example](.env.example).

| Variable | Default | Meaning |
| --- | --- | --- |
| `JELLYFIN_URL` | required | Your Jellyfin server |
| `JELLYFIN_API_KEY` | required | Jellyfin API key |
| `JELLYFIN_USER` | first admin | Jellyfin user every Plex client acts as |
| `JELLEX_SERVER_NAME` | `jellex` | Server name shown in Plex apps |
| `JELLEX_LISTEN_ADDR` | `:32400` | Listen address |
| `JELLEX_MACHINE_ID` | derived from Jellyfin | Server identity |
| `JELLEX_PUBLISH_URLS` | | URLs published to plex.tv (experimental) |
| `PLEX_CLAIM` | | Claim token from https://plex.tv/claim (experimental) |
| `JELLEX_DISABLE_CUSTOM_ASSETS` | `false` | Serve Plex Web exactly as shipped, without the jellex favicon and wordmark |

State lives in `/data` (an ID map that keeps Plex item IDs stable, and the
plex.tv claim). Back up that volume if you care about keeping links and
watch history in Plex apps consistent.

## Development

See [AGENTS.md](AGENTS.md) for the dev stack: a local Jellyfin with
generated test media, a setup script, and how the code is laid out.

## Legal

jellex is not affiliated with Plex or Jellyfin. It doesn't include or
redistribute Plex software: the Plex Web client is downloaded from Plex's
own servers at runtime and never modified on disk. By default jellex swaps
in its own favicon and top-bar wordmark as the client is served; set
`JELLEX_DISABLE_CUSTOM_ASSETS=true` to serve it exactly as shipped. Using it
with plex.tv may be against Plex's terms of service.
