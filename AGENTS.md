# jellex

jellex is a Plex Web–style client for Jellyfin: a SvelteKit single-page app
that talks to the Jellyfin API straight from the browser. There is no
backend; the build output is static files that can be hosted anywhere
(static host, reverse proxy next to Jellyfin, or a Jellyfin plugin that just
serves them).

The approach is a 1:1 copy of Plex Web first, tweaked afterwards: every
screen reproduces Plex Web's measured sizes, colours, fonts, icons and
animations (home hubs, library grids, details pages, the player) on top of
Jellyfin's data. See "Copying Plex Web" below for the workflow.

## Layout

- `src/routes` – SvelteKit routes. The app runs in SPA mode (`ssr = false`
  in `+layout.ts`), deployed to Cloudflare Pages with
  `@sveltejs/adapter-cloudflare`.
- `src/lib` – shared code: the Jellyfin session (`session.svelte.ts`),
  image URLs (`images.ts`), components, Plex's icon set (`icons.ts`,
  rendered by `components/Icon.svelte`) and Plex's design tokens
  (`styles/tokens.css`).
- `src/app.html` – also holds Plex's boot screen (the wordmark on #1f1f1f,
  first load per browser session), removed by the root layout once the app
  renders.
- `static` – files copied verbatim into the build.
- `assets` – jellex logo and wordmarks.
- `ref/` (gitignored) – reference material: the PMS `.deb` and its unpacked
  Plex Web client (handy when copying UI details), `plex-api-spec`, and
  `jellyfin-openapi.json`.

Stack: Svelte 5 (runes), SvelteKit, Tailwind 4, TypeScript,
`@jellyfin/sdk` (generated API client over axios), `hls.js` for transcoded
playback.

## Dev environment

There is no local Jellyfin: develop and test against Jellyfin's public demo
server, which the login page's "Try the Jellyfin demo" button signs in to.

| What                 | Value                            |
| -------------------- | -------------------------------- |
| Jellyfin demo server | https://demo.jellyfin.org/stable |
| Demo user            | `demo`, no password              |
| jellex dev server    | http://localhost:32400           |

Develop with `npm install && npm run dev` (Vite with hot reload on port
32400). `npm run build` writes the Cloudflare Pages output to
`.svelte-kit/cloudflare` (build command `npm run build`, output directory
`.svelte-kit/cloudflare`). Nothing reads a `.env` file.

`npm run check` type-checks; `npm run format` formats with Prettier
(`npm run lint` checks it).

## Notes

- The browser talks to Jellyfin directly. Jellyfin allows any origin by
  default, so the app can live on another host; the user enters the server
  URL and signs in (password or Quick Connect), and the token is kept in the
  browser. A page served over HTTPS can only reach an HTTPS Jellyfin.
- Everything Jellyfin offers maps onto what Plex Web needs: `PlaybackInfo`
  with a device profile decides direct play vs transcode, `master.m3u8`
  serves HLS, `Sessions/Playing` reports progress, media segments give Skip
  Intro/Credits, SyncPlay replaces Watch Together.
- Routes: `/` (home hubs), `/library/<view id>?pivot=recommended|library|collections`
  (grid params `sort`, `order`, `filter`, `genre`, `person`; `hub=recent`
  is Plex's "Recently Added" hub list: the grid newest first, capped at 200),
  `/items/<id>` (one details page for every type), `/play/<id>` (`audio`,
  `sub`, `start=0`, `shuffle=1`; folders and libraries play as a queue),
  `/settings/general|player|theme`, `/login`.
- Settings (`src/lib/settings.svelte.ts`) are per browser, in localStorage,
  saved with Save Changes: Remember selected tab, subtitle colour, position
  and size, and the Theme accent. Only Plex settings jellex honours are
  listed. The theme sets the accent tokens on `<html>` at runtime, so use
  `--color-brand-accent`, `--color-accent-dark` and `--color-accent-light`,
  never hard-coded blues.
- The player (`src/lib/playback.ts`) sends Jellyfin a device profile built
  from `canPlayType`, direct-plays what the browser can, and otherwise plays
  Jellyfin's HLS (natively or via hls.js). It reports start, progress and
  stop; the stop also goes out on `pagehide`, because Jellyfin treats a
  session that ends without one as watched to the end.
- Jellyfin marks anything shorter than `MinResumeDurationSeconds` (300s by
  default) as watched whenever playback stops, and a stopped session saves
  its position. The demo server is shared, so tests that open the player
  should block `/Sessions/Playing*` in the test browser.
- Scope is a single user's view of a library, not server administration.
- jellex has no users yet: breaking changes are fine.
- The `#lib/*` alias in `package.json` maps paths literally, so imports of
  `.ts` modules need the extension: `#lib/session.svelte.ts`, `#lib/images.ts`.
- Design tokens are Plex Web's own: `src/lib/styles/tokens.css` is its
  "chroma dark" variable block copied verbatim (orange accent recoloured to
  Jellyfin blue), so components use Plex's names (`--color-text-primary`,
  `--color-background-accent`, `--size-m`, `--border-radius-s`, …) and CSS
  lifted from Plex Web works unchanged. Plex's orange maps to Jellyfin blue
  as `#e5a00d`→`#00a4dc`, `#cc7b19`→`#0083b0`, `#f9be03`/`#ebaf00`→`#2cb9ec`.
- Plex's headings use PlexCircular, a commercial font licensed to Plex that
  can't be redistributed; Figtree (`@fontsource/figtree`) stands in via
  `--font-heading`. Body text is the system font stack, as in Plex.
- Tailwind is loaded, and it generates a utility for any class name it sees
  in the source, including inside scoped `<style>` blocks. Avoid component
  class names that are Tailwind utilities (`list-item`, `hidden`, `block`,
  `collapse`, `table`, `container`, `grid`, `outline`, `shadow`, `visible`,
  …); `hidden` once turned the player's slide-away bars into an instant
  `display: none`: `list-item` once
  put a bullet in front of every poster title, and `container` capped the
  modal backdrop at 1536px wide. Scoped styles beat Tailwind's layer only
  for the properties they set, so the rest of the utility leaks through.
- Session state is a Svelte 5 runes module (`src/lib/session.svelte.ts`);
  the token is kept in localStorage and restored in the root layout load.

## Testing in a browser

A headless Chromium run is the fastest feedback loop: load `/login` on the
dev server, click "Try the Jellyfin demo" (or fill `#server`, `#u`, `#p`),
record failed requests and console errors, and screenshot. Keep such
throwaway scripts (and their Playwright install) outside the repo, e.g. in
a scratch directory. Sign out at the end (`POST /Sessions/Logout` with the
token in `localStorage['jellex.session']`), or every run leaves a device
behind in Jellyfin. Playwright's `chrome-headless-shell` decodes H.264/AAC
but not HEVC, so HEVC files exercise transcoding.

## Copying Plex Web

Don't eyeball Plex: measure it. The Plex Web client ships inside the PMS
package (unpacked into `ref/plexmediaserver`, under
`Resources/Plug-ins-*/WebClient.bundle/Contents/Resources`); running it
against a server lets you read boxes and computed styles from the live DOM
with a headless browser. Plex Web class names look like
`MetadataPosterCard-progress-EXxFw9`; the component is the part before the
first dash, so `[class*="MetadataPosterCard-"]` finds one. Hover states,
transitions and keyframes are in the client's stylesheets:

    grep -rhoE '[^{}]*\.PosterCardLink-[^{]*\{[^}]*\}' ref/plexmediaserver --include=*.css

Copy the numbers, then screenshot ours at the same size and compare. Plex's
icons come out of the live DOM as SVG markup; add new ones to
`src/lib/icons.ts`. Useful Plex Web routes (`<id>` is the server's machine
identifier, `<n>` a Plex integer ID):

- library: `#!/media/<id>/com.plexapp.plugins.library?source=<n>&pivot=recommended|library|collections`
- details: `#!/server/<id>/details?key=/library/metadata/<n>`
- search: `#!/search?pivot=top&query=<term>`
- settings: `#!/settings/web/general` (and `player`)

Animations come from the bundle, not guesses. Plex's motion constants
(module 82810) are in `src/lib/motion.ts`: 200ms default, 600ms slow, 100ms
fast, 150ms menus, `motionEasing` cubic-bezier(0.6, 0.4, 0.2, 1.4), and
framer-motion's easeOut/easeInOut. Record a real interaction with
`document.getAnimations()` plus a MutationObserver on inline styles to see
what actually moves; many things Plex does not animate at all (opening
the player, page changes, card progress bars).

Two traps when diffing hover states: compare computed styles before and
after `mouse.move`, because some effects are box-shadows on overlay links
(cast photos) rather than rules on the element you'd expect; and check
whether rows are true siblings, since `:first-of-type` rules behave
differently in Plex's virtualised lists.

Don't build shell heredocs that contain text with its own heredoc
terminator: a stray `EOF` ends the heredoc early and runs the rest as shell
commands. Use the editor tools for text edits.
