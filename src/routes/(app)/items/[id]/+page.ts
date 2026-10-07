import { error } from '@sveltejs/kit';
import { loadDetails } from '#lib/item.ts';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ parent, params, depends }) => {
	const { views } = await parent();
	// Watched/favourite toggles invalidate this to refresh the page.
	depends('jellex:item');
	try {
		return { details: await loadDetails(params.id, views) };
	} catch (e) {
		const status = (e as { response?: { status?: number } })?.response?.status;
		if (status === 404 || status === 400) error(404, 'Item not found');
		throw e;
	}
};
