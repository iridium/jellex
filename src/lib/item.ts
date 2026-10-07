// Data behind Plex's details ("pre-play") pages.
import type { BaseItemDto, MediaStream } from '@jellyfin/sdk/lib/generated-client';
import { getLibraryApi } from '@jellyfin/sdk/lib/utils/api/library-api';
import { getPlaylistApi } from '@jellyfin/sdk/lib/utils/api/playlist-api';
import { getShowApi } from '@jellyfin/sdk/lib/utils/api/show-api';
import { session } from './session.svelte.ts';

export interface Details {
	item: BaseItemDto;
	/** The library (user view) the item belongs to, for the breadcrumb. */
	library?: BaseItemDto;
	seasons: BaseItemDto[];
	episodes: BaseItemDto[];
	tracks: BaseItemDto[];
	albums: BaseItemDto[];
	children: BaseItemDto[];
	similar: BaseItemDto[];
	/** A series' next episode (Plex's "On Deck"). */
	nextUp?: BaseItemDto;
	/** What the header's Previous / list / Next step through: a season's
	 *  episodes, a show's seasons, or an artist's albums. */
	siblings: BaseItemDto[];
}

const none = Promise.resolve([] as BaseItemDto[]);

export async function loadDetails(itemId: string, views: BaseItemDto[]): Promise<Details> {
	const api = session.requireApi;
	const userId = session.userId;
	const library = getLibraryApi(api);
	const shows = getShowApi(api);

	const [{ data: item }, ancestors] = await Promise.all([
		library.getItem({ itemId, userId }),
		library
			.getAncestors({ itemId, userId })
			.then((r) => r.data)
			.catch(() => [] as BaseItemDto[])
	]);
	const viewIds = new Set(views.map((v) => v.Id));
	const libraryView = views.find((v) => v.Id === ancestors.find((a) => viewIds.has(a.Id))?.Id);

	const items = (params: Parameters<typeof library.getItems>[0]) =>
		library.getItems({ userId, ...params }).then((r) => r.data.Items ?? []);
	const type = item.Type;

	const siblings: Promise<BaseItemDto[]> =
		type === 'Episode' && item.SeriesId && item.SeasonId
			? shows
					.getEpisodes({ seriesId: item.SeriesId, seasonId: item.SeasonId, userId })
					.then((r) => r.data.Items ?? [])
					.catch(() => [])
			: type === 'Season' && item.SeriesId
				? shows
						.getSeasons({ seriesId: item.SeriesId, userId })
						.then((r) => r.data.Items ?? [])
						.catch(() => [])
				: type === 'MusicAlbum' && item.AlbumArtists?.[0]?.Id
					? items({
							albumArtistIds: [item.AlbumArtists[0].Id],
							includeItemTypes: ['MusicAlbum'],
							recursive: true,
							sortBy: ['ProductionYear', 'SortName'],
							sortOrder: ['Descending', 'Ascending']
						}).catch(() => [])
					: none;

	const [seasons, episodes, tracks, albums, children, similar, nextUp] = await Promise.all([
		type === 'Series'
			? shows.getSeasons({ seriesId: itemId, userId }).then((r) => r.data.Items ?? [])
			: none,
		type === 'Season'
			? shows
					.getEpisodes({ seriesId: item.SeriesId!, seasonId: itemId, userId, fields: ['Overview'] })
					.then((r) => r.data.Items ?? [])
			: none,
		type === 'MusicAlbum'
			? items({
					parentId: itemId,
					includeItemTypes: ['Audio'],
					sortBy: ['ParentIndexNumber', 'IndexNumber', 'SortName']
				})
			: none,
		type === 'MusicArtist'
			? items({
					albumArtistIds: [itemId],
					includeItemTypes: ['MusicAlbum'],
					recursive: true,
					sortBy: ['ProductionYear', 'SortName'],
					sortOrder: ['Descending', 'Ascending']
				})
			: none,
		type === 'Playlist'
			? getPlaylistApi(api)
					.getPlaylistItems({ playlistId: itemId, userId })
					.then((r) => r.data.Items ?? [])
			: type === 'BoxSet' || type === 'Folder' || type === 'CollectionFolder'
				? items({ parentId: itemId })
				: none,
		['Movie', 'Series', 'MusicAlbum', 'MusicArtist'].includes(type ?? '')
			? library
					.getSimilarItems({ itemId, userId, limit: 20 })
					.then((r) => r.data.Items ?? [])
					.catch(() => [] as BaseItemDto[])
			: none,
		type === 'Series'
			? shows
					.getNextUp({ seriesId: itemId, userId, limit: 1 })
					.then((r) => r.data.Items?.[0])
					.catch(() => undefined)
			: Promise.resolve(undefined)
	]);

	return {
		item,
		library: libraryView,
		seasons,
		episodes,
		tracks,
		albums,
		children,
		similar,
		nextUp,
		siblings: await siblings
	};
}

/** The item's default media source's streams, by kind. */
export function streams(item: BaseItemDto) {
	const source = item.MediaSources?.[0];
	const all: MediaStream[] = source?.MediaStreams ?? [];
	return {
		video: all.find((s) => s.Type === 'Video'),
		audio: all.filter((s) => s.Type === 'Audio'),
		subtitles: all.filter((s) => s.Type === 'Subtitle'),
		defaultAudio: source?.DefaultAudioStreamIndex ?? undefined,
		defaultSubtitle: source?.DefaultSubtitleStreamIndex ?? -1
	};
}
