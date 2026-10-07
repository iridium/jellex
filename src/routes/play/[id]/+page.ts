import { error, redirect } from '@sveltejs/kit';
import { getLibraryApi } from '@jellyfin/sdk/lib/utils/api/library-api';
import { resolveQueue } from '#lib/playback.ts';
import { session } from '#lib/session.svelte.ts';
import type { PageLoad } from './$types';

// /play/<id>[?audio=<index>&sub=<index>&start=0&shuffle=1] plays an item,
// or the queue Play starts for a series, season, album, collection or a
// whole library (shuffled with shuffle=1).
export const load: PageLoad = async ({ parent, params, url }) => {
	await parent();
	if (!session.signedIn) redirect(302, '/login');
	const { data: item } = await getLibraryApi(session.requireApi)
		.getItem({ itemId: params.id, userId: session.userId })
		.catch(() => error(404, 'Item not found'));
	// The player shuffles the queue itself, keeping this order for unshuffling.
	const shuffle = url.searchParams.get('shuffle') === '1';
	const queue = await resolveQueue(item);
	if (!queue.length) error(404, 'Nothing to play');
	const num = (k: string) =>
		url.searchParams.has(k) ? Number(url.searchParams.get(k)) : undefined;
	return {
		item,
		queue,
		shuffle,
		audio: num('audio'),
		subtitle: num('sub'),
		fromStart: url.searchParams.get('start') === '0'
	};
};
