import { error } from '@sveltejs/kit';
import { settingsPages } from '#lib/settings.svelte.ts';
import type { PageLoad } from './$types';

export const load: PageLoad = ({ params }) => {
	const page = settingsPages.find((p) => p.key === params.page);
	if (!page) error(404, 'Not found');
	return { page: page.key, label: page.label };
};
