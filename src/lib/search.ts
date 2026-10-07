// Plex Web's search: top results across everything (Jellyfin's ranked
// search hints, resolved to full items), or one category.
import type { BaseItemDto, BaseItemKind } from '@jellyfin/sdk/lib/generated-client';
import { getLibraryApi } from '@jellyfin/sdk/lib/utils/api/library-api';
import { getPersonApi } from '@jellyfin/sdk/lib/utils/api/person-api';
import { getSearchApi } from '@jellyfin/sdk/lib/utils/api/search-api';
import { episodeCode } from './format.ts';
import { session } from './session.svelte.ts';

export type SearchTab = 'top' | 'video' | 'music' | 'people';

export const tabs: { key: SearchTab; label: string }[] = [
	{ key: 'top', label: 'Top Results' },
	{ key: 'video', label: 'Movies & Shows' },
	{ key: 'music', label: 'Music' },
	{ key: 'people', label: 'People' }
];

const VIDEO: BaseItemKind[] = ['Movie', 'Series', 'Episode', 'BoxSet', 'MusicVideo', 'Video'];
const MUSIC: BaseItemKind[] = ['MusicArtist', 'MusicAlbum', 'Audio'];

export async function search(term: string, tab: SearchTab, limit: number): Promise<BaseItemDto[]> {
	const api = session.requireApi;
	const userId = session.userId;
	if (!term.trim()) return [];
	if (tab === 'people') {
		const { data } = await getPersonApi(api).getPersons({
			searchTerm: term,
			userId,
			limit,
			enableImages: true
		});
		return data.Items ?? [];
	}
	const kinds =
		tab === 'video' ? VIDEO : tab === 'music' ? MUSIC : [...VIDEO, ...MUSIC, 'Playlist' as const];
	const { data: hints } = await getSearchApi(api).getSearchHints({
		searchTerm: term,
		userId,
		limit,
		includeItemTypes: kinds,
		includePeople: tab === 'top',
		includeMedia: true,
		includeGenres: false,
		includeStudios: false,
		includeArtists: tab !== 'video'
	});
	const ids = (hints.SearchHints ?? []).map((h) => h.Id!).filter(Boolean);
	if (!ids.length) return [];
	const { data } = await getLibraryApi(api).getItems({
		userId,
		ids,
		fields: ['PrimaryImageAspectRatio']
	});
	const byId = new Map((data.Items ?? []).map((i) => [i.Id, i]));
	// People in top results come back as hints only.
	const people = new Map(
		(hints.SearchHints ?? [])
			.filter((h) => h.Type === 'Person')
			.map((h) => [
				h.Id,
				{
					Id: h.Id,
					Name: h.Name,
					Type: 'Person' as const,
					ImageTags: h.PrimaryImageTag ? { Primary: h.PrimaryImageTag } : undefined
				}
			])
	);
	return ids.map((id) => byId.get(id) ?? people.get(id)).filter((x): x is BaseItemDto => !!x);
}

const TYPE_LABEL: Record<string, string> = {
	Movie: 'Movie',
	Series: 'Show',
	Season: 'Season',
	Episode: 'Episode',
	MusicArtist: 'Artist',
	MusicAlbum: 'Album',
	Audio: 'Track',
	BoxSet: 'Collection',
	Playlist: 'Playlist',
	Person: 'Person',
	MusicVideo: 'Music Video',
	Video: 'Video'
};

/** Plex's search row lines under the title. */
export function describe(item: BaseItemDto): string[] {
	const type = TYPE_LABEL[item.Type ?? ''] ?? item.Type ?? '';
	const year = item.ProductionYear ? String(item.ProductionYear) : '';
	switch (item.Type) {
		case 'Episode':
			return [item.SeriesName ?? '', [episodeCode(item), type].filter(Boolean).join(' • ')];
		case 'MusicAlbum':
			return [item.AlbumArtist ?? '', [year, type].filter(Boolean).join(' • ')];
		case 'Audio':
			return [[item.AlbumArtist, item.Album].filter(Boolean).join(' — '), type];
		default:
			return [[year, type].filter(Boolean).join(' • ')];
	}
}
