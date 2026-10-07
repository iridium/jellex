import { session } from '#lib/session.svelte.ts';

// Pure single-page app: no server rendering, no prerendering. On
// Cloudflare (adapter-cloudflare) every route serves the same app shell.
export const ssr = false;
export const prerender = false;

// Reconnect with the saved token before any page decides what to show.
export const load = async () => {
	await session.restore();
};
