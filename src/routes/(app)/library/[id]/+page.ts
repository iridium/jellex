import { error } from '@sveltejs/kit';
import {
	filters,
	hasRecommended,
	recommendedHubs,
	sorts,
	sortsFor,
	typesFor,
	type LibraryType,
	type FilterKey,
	type Pivot,
	type SortKey
} from '#lib/library.ts';
import { rememberedPivot, rememberPivot } from '#lib/settings.svelte.ts';
import type { PageLoad } from './$types';

// Plex's library page: ?pivot=recommended|library|collections, and for the
// grid ?sort=…&order=asc|desc&filter=…&genre=….
export const load: PageLoad = async ({ parent, params, url }) => {
	const { views } = await parent();
	const view = views.find((v) => v.Id === params.id);
	if (!view) error(404, 'Library not found');

	const q = url.searchParams;
	// Plex's hub list page ("Recently Added …" header): the library grid,
	// newest first, capped at the hub's 200 items.
	const hub = q.get('hub') === 'recent' ? ('recent' as const) : null;
	const defaultPivot: Pivot = hasRecommended(view.CollectionType) ? 'recommended' : 'library';
	const valid = (p: string | null | undefined): p is Pivot =>
		!!p &&
		['recommended', 'library', 'collections'].includes(p) &&
		(p !== 'recommended' || hasRecommended(view.CollectionType));
	// "Remember selected tab" (Settings > General): reopen a library on the
	// tab it was last left on.
	const asked = q.get('pivot');
	if (!hub && valid(asked)) rememberPivot(view.Id!, asked);
	const remembered = rememberedPivot(view.Id!);
	const pivot: Pivot = hub
		? 'library'
		: valid(asked)
			? asked
			: valid(remembered)
				? remembered
				: defaultPivot;
	// Plex's type menu (TV: Shows/Seasons/Episodes, music: Artists/Albums/
	// Tracks) on the Library tab; each type has its own sorts.
	const types = pivot === 'library' && !hub ? typesFor(view.CollectionType) : undefined;
	const type: LibraryType | undefined = types
		? (types.find((t) => t.key === q.get('type'))?.key ?? types[0].key)
		: undefined;
	const typeSorts = sortsFor(type);
	const sortDef =
		typeSorts.find((s) => s.key === q.get('sort')) ??
		(hub ? sorts.find((s) => s.key === 'DateCreated')! : (typeSorts[0] ?? sorts[0]));
	const order = q.get('order');
	const filter = (filters.find((f) => f.key === q.get('filter'))?.key ?? 'all') as FilterKey;

	return {
		view,
		hub,
		pivot,
		type,
		sort: sortDef.key as SortKey,
		desc: order ? order === 'desc' : sortDef.desc,
		filter,
		genre: q.get('genre') ?? undefined,
		year: q.get('year') ? Number(q.get('year')) : undefined,
		decade: q.get('decade') ? Number(q.get('decade')) : undefined,
		rating: q.get('rating') ?? undefined,
		personId: q.get('person') ?? undefined,
		personName: q.get('personName') ?? undefined,
		hubs: pivot === 'recommended' && !hub ? recommendedHubs(view) : null
	};
};
