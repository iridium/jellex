// Library queries behind Plex's library page: the Recommended hubs, and the
// Library grid with Plex's filter and sort menus and A–Z jump bar.
import type {
	BaseItemDto,
	BaseItemKind,
	ItemFilter,
	ItemSortBy
} from '@jellyfin/sdk/lib/generated-client';
import { getArtistApi } from '@jellyfin/sdk/lib/utils/api/artist-api';
import { getFilterApi } from '@jellyfin/sdk/lib/utils/api/filter-api';
import { getLibraryApi } from '@jellyfin/sdk/lib/utils/api/library-api';
import { getShowApi } from '@jellyfin/sdk/lib/utils/api/show-api';
import type { ImageKind } from './images.ts';
import { session } from './session.svelte.ts';

export type Pivot = 'recommended' | 'library' | 'collections';

export interface GridSpec {
	/** Jellyfin item types the grid lists. */
	types?: BaseItemKind[];
	/** Card shape. */
	kind: ImageKind;
	/** Albums and artists come from a different endpoint. */
	artists?: boolean;
	recursive: boolean;
}

/**
 * Plex's type menu on a TV or music library's Library tab: what the grid
 * lists. The first one is the default.
 */
export const libraryTypes = {
	tvshows: [
		{ key: 'shows', label: 'Shows' },
		{ key: 'seasons', label: 'Seasons' },
		{ key: 'episodes', label: 'Episodes' }
	],
	music: [
		{ key: 'artists', label: 'Artists' },
		{ key: 'albums', label: 'Albums' },
		{ key: 'tracks', label: 'Tracks' }
	]
} as const;
export type LibraryType = (typeof libraryTypes)[keyof typeof libraryTypes][number]['key'];

export function typesFor(collectionType: string | null | undefined) {
	return (libraryTypes as Record<string, readonly { key: LibraryType; label: string }[]>)[
		collectionType ?? ''
	];
}

/** What each library type shows; missing types list their folder as is. */
export function gridSpec(
	collectionType: string | null | undefined,
	pivot: Pivot,
	type?: LibraryType
): GridSpec {
	if (pivot === 'collections') return { types: ['BoxSet'], kind: 'poster', recursive: true };
	switch (collectionType) {
		case 'movies':
			return { types: ['Movie'], kind: 'poster', recursive: true };
		case 'tvshows':
			// Plex lists seasons and episodes with their show's poster.
			if (type === 'seasons') return { types: ['Season'], kind: 'poster', recursive: true };
			if (type === 'episodes') return { types: ['Episode'], kind: 'poster', recursive: true };
			return { types: ['Series'], kind: 'poster', recursive: true };
		case 'music':
			if (type === 'albums') return { types: ['MusicAlbum'], kind: 'square', recursive: true };
			if (type === 'tracks') return { types: ['Audio'], kind: 'square', recursive: true };
			return { kind: 'square', artists: true, recursive: true };
		case 'musicvideos':
			return { types: ['MusicVideo'], kind: 'landscape', recursive: true };
		case 'boxsets':
			return { types: ['BoxSet'], kind: 'poster', recursive: false };
		case 'playlists':
			return { types: ['Playlist'], kind: 'square', recursive: false };
		case 'homevideos':
		case 'photos':
			return { kind: 'landscape', recursive: false };
		default:
			return { kind: 'poster', recursive: false };
	}
}

/** Library types that open on Plex's Recommended tab. */
export function hasRecommended(collectionType: string | null | undefined): boolean {
	return collectionType === 'movies' || collectionType === 'tvshows' || collectionType === 'music';
}

/** Library types with a Collections tab. */
export function hasCollections(collectionType: string | null | undefined): boolean {
	return collectionType === 'movies' || collectionType === 'tvshows';
}

// Plex's sort menu, in Plex's order and wording. Choosing the current
// sort again flips its direction (shown as an arrow on the entry).
export const sorts = [
	{ key: 'SortName', label: 'Title', desc: false },
	{ key: 'DateCreated', label: 'Date Added', desc: true },
	{ key: 'PremiereDate', label: 'Release Date', desc: true },
	{ key: 'ProductionYear', label: 'Year', desc: true },
	{ key: 'CommunityRating', label: 'Audience Rating', desc: true },
	{ key: 'DatePlayed', label: 'Last Viewed', desc: true },
	{ key: 'Random', label: 'Randomly', desc: false }
] as const satisfies { key: ItemSortBy; label: string; desc: boolean }[];
export type SortKey = (typeof sorts)[number]['key'];

/**
 * Plex's sort and filter menus per type (measured): movies, shows and
 * episodes sort every way; seasons and artists by title, date added or at
 * random; albums also by year. Seasons have no filter menu and tracks
 * neither menu (their table header sorts). Music has no Unplayed or
 * Content Rating; episodes filter by year only.
 */
const SORTS_BY_TYPE: Record<string, SortKey[]> = {
	seasons: ['SortName', 'DateCreated', 'Random'],
	artists: ['SortName', 'DateCreated', 'Random'],
	albums: ['SortName', 'DateCreated', 'ProductionYear', 'Random'],
	tracks: []
};
const DEFAULT_SORTS: SortKey[] = [
	'SortName',
	'DateCreated',
	'PremiereDate',
	'CommunityRating',
	'DatePlayed',
	'Random'
];
export function sortsFor(type: LibraryType | undefined) {
	const keys = SORTS_BY_TYPE[type ?? ''] ?? DEFAULT_SORTS;
	return keys.map((k) => sorts.find((s) => s.key === k)!);
}

export type FilterCategory = 'genre' | 'year' | 'decade' | 'rating';
export interface FilterSet {
	unplayed: boolean;
	categories: FilterCategory[];
}
export function filtersFor(
	collectionType: string | null | undefined,
	type: LibraryType | undefined
): FilterSet | null {
	if (type === 'seasons' || type === 'tracks') return null;
	if (type === 'episodes') return { unplayed: true, categories: ['year'] };
	if (collectionType === 'music')
		return { unplayed: false, categories: ['genre', 'year', 'decade'] };
	return { unplayed: true, categories: ['genre', 'year', 'decade', 'rating'] };
}

// Plex's filter menu: All, Unplayed, then the drill-in filters (Genre,
// Year, Decade, Content Rating; music has no Unplayed or Content Rating).
export const filters = [
	{ key: 'all', label: 'All' },
	{ key: 'unwatched', label: 'Unplayed' }
] as const;
export type FilterKey = (typeof filters)[number]['key'];

/** The drill-in filters' values for a library (Jellyfin's /Items/Filters). */
export async function filterValues(q: Pick<GridQuery, 'parentId' | 'spec'>) {
	const { data } = await getFilterApi(session.requireApi).getQueryFiltersLegacy({
		userId: session.userId,
		parentId: q.parentId,
		includeItemTypes: q.spec.types
	});
	const years = [...(data.Years ?? [])].sort((a, b) => b - a);
	const decades = [...new Set(years.map((y) => Math.floor(y / 10) * 10))];
	return {
		genres: [...(data.Genres ?? [])].sort((a, b) => a.localeCompare(b)),
		years,
		decades,
		ratings: data.OfficialRatings ?? []
	};
}

export interface GridQuery {
	parentId: string;
	spec: GridSpec;
	sort: SortKey;
	desc: boolean;
	filter: FilterKey;
	genre?: string;
	personId?: string;
	year?: number;
	/** First year of a decade (1990 for the 1990s). */
	decade?: number;
	rating?: string;
}

function params(q: GridQuery) {
	const itemFilters: ItemFilter[] = [];
	return {
		userId: session.userId,
		parentId: q.spec.types?.includes('BoxSet') && q.spec.recursive ? undefined : q.parentId,
		includeItemTypes: q.spec.types,
		recursive: q.spec.recursive,
		// Jellyfin wants one sort order per sort field.
		sortBy: (q.sort === 'SortName' ? ['SortName'] : [q.sort, 'SortName']) as ItemSortBy[],
		sortOrder: (q.sort === 'SortName'
			? [q.desc ? 'Descending' : 'Ascending']
			: [q.desc ? 'Descending' : 'Ascending', 'Ascending']) as ('Ascending' | 'Descending')[],
		isPlayed: q.filter === 'unwatched' ? false : undefined,
		filters: itemFilters.length ? itemFilters : undefined,
		genres: q.genre ? [q.genre] : undefined,
		years: q.year
			? [q.year]
			: q.decade != null
				? Array.from({ length: 10 }, (_, i) => q.decade! + i)
				: undefined,
		officialRatings: q.rating ? [q.rating] : undefined,
		personIds: q.personId ? [q.personId] : undefined,
		fields: ['PrimaryImageAspectRatio' as const, 'DateCreated' as const],
		enableTotalRecordCount: true,
		imageTypeLimit: 1
	};
}

/** One page of the grid, plus the total count. */
export async function fetchGrid(
	q: GridQuery,
	startIndex: number,
	limit: number
): Promise<{ items: BaseItemDto[]; total: number }> {
	const api = session.requireApi;
	const p = { ...params(q), startIndex, limit };
	const { data } = q.spec.artists
		? await getArtistApi(api).getAlbumArtists(p)
		: await getLibraryApi(api).getItems(p);
	return { items: data.Items ?? [], total: data.TotalRecordCount ?? 0 };
}

/** Index of the first item at or after a letter ("#" is the start). */
export async function indexOfLetter(q: GridQuery, letter: string): Promise<number> {
	if (letter === '#') return 0;
	const api = session.requireApi;
	const p = { ...params(q), nameLessThan: letter, limit: 0 };
	const { data } = q.spec.artists
		? await getArtistApi(api).getAlbumArtists(p)
		: await getLibraryApi(api).getItems(p);
	return data.TotalRecordCount ?? 0;
}

export interface Hub {
	title: string;
	kind: ImageKind;
	items: BaseItemDto[];
	href?: string;
}

/** Plex's Recommended tab for a library. */
export async function recommendedHubs(view: BaseItemDto): Promise<Hub[]> {
	const api = session.requireApi;
	const userId = session.userId;
	const library = getLibraryApi(api);
	const parentId = view.Id!;
	const name = view.Name ?? '';
	const top = (types: BaseItemKind[], sortBy: ItemSortBy, extra = {}) =>
		library
			.getItems({
				userId,
				parentId,
				includeItemTypes: types,
				recursive: true,
				sortBy: [sortBy],
				sortOrder: ['Descending'],
				limit: 20,
				...extra
			})
			.then((r) => r.data.Items ?? []);
	const latest = () =>
		library.getLatestMedia({ userId, parentId, limit: 30, groupItems: true }).then((r) => r.data);

	let hubs: Hub[];
	switch (view.CollectionType) {
		case 'movies': {
			const [resume, added, released, rated] = await Promise.all([
				library
					.getResumeItems({ userId, parentId, limit: 20, mediaTypes: ['Video'] })
					.then((r) => r.data.Items ?? []),
				latest(),
				top(['Movie'], 'PremiereDate'),
				top(['Movie'], 'CommunityRating')
			]);
			hubs = [
				{ title: 'Continue Watching', kind: 'poster', items: resume },
				{ title: `Recently Added ${name}`, kind: 'poster', items: added },
				{ title: 'Recently Released Movies', kind: 'poster', items: released },
				{ title: 'Top Rated Movies', kind: 'poster', items: rated }
			];
			break;
		}
		case 'tvshows': {
			const [resume, nextUp, added, aired, rated] = await Promise.all([
				library
					.getResumeItems({ userId, parentId, limit: 20, mediaTypes: ['Video'] })
					.then((r) => r.data.Items ?? []),
				getShowApi(api)
					.getNextUp({ userId, parentId, limit: 20, enableResumable: false })
					.then((r) => r.data.Items ?? []),
				latest(),
				top(['Episode'], 'PremiereDate'),
				top(['Series'], 'CommunityRating')
			]);
			const seen = new Set(resume.map((i) => i.Id));
			hubs = [
				{
					title: 'Continue Watching',
					kind: 'poster',
					items: [...resume, ...nextUp.filter((i) => !seen.has(i.Id))]
				},
				{ title: `Recently Added ${name}`, kind: 'poster', items: added },
				{ title: 'Recently Aired', kind: 'poster', items: aired },
				{ title: 'Top Rated Shows', kind: 'poster', items: rated }
			];
			break;
		}
		case 'music': {
			const [added, released] = await Promise.all([latest(), top(['MusicAlbum'], 'PremiereDate')]);
			hubs = [
				{ title: `Recently Added ${name}`, kind: 'square', items: added },
				{ title: 'Recently Released Albums', kind: 'square', items: released }
			];
			break;
		}
		default:
			hubs = [];
	}
	return hubs.filter((h) => h.items.length > 0);
}

/** A person's titles in a library (Plex links cast and crew to this). */
export function personHref(
	libraryId: string | undefined | null,
	person: { Id?: string | null; Name?: string | null }
) {
	if (!libraryId || !person.Id) return undefined;
	return `/library/${libraryId}?pivot=library&person=${person.Id}&personName=${encodeURIComponent(person.Name ?? '')}`;
}
