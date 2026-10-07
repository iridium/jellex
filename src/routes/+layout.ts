import { session } from '#lib/session.svelte.ts';

// Pure single-page app: no server rendering, no prerendering. The static
// adapter emits index.html as the fallback for every route.
export const ssr = false;
export const prerender = false;

// Reconnect with the saved token before any page decides what to show.
export const load = async () => {
	await session.restore();
};
