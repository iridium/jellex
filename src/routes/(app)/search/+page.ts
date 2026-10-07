import { search, tabs, type SearchTab } from '#lib/search.ts';
import type { PageLoad } from './$types';

// /search?query=…&pivot=top|video|music|people (Plex's search page).
export const load: PageLoad = async ({ url, parent }) => {
	await parent();
	const query = url.searchParams.get('query') ?? '';
	const pivot = (tabs.find((t) => t.key === url.searchParams.get('pivot'))?.key ??
		'top') as SearchTab;
	return { query, pivot, results: search(query, pivot, 50).catch(() => []) };
};
