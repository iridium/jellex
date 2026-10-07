// Per-item actions shared by Plex's menus (poster cards, search rows,
// details pages, the player): watched state, queueing, playlists, ratings.
import type { BaseItemDto } from '@jellyfin/sdk/lib/generated-client';
import { getLibraryApi } from '@jellyfin/sdk/lib/utils/api/library-api';
import { getPlaylistApi } from '@jellyfin/sdk/lib/utils/api/playlist-api';
import { getShowApi } from '@jellyfin/sdk/lib/utils/api/show-api';
import { getUserDataApi } from '@jellyfin/sdk/lib/utils/api/user-data-api';
import { invalidateAll } from '$app/navigation';
import type { MenuEntry } from './components/Menu.svelte';
import { modals } from './modals.svelte.ts';
import { resolveQueue } from './playback.ts';
import { player } from './player.svelte.ts';
import { session } from './session.svelte.ts';

const userData = () => getUserDataApi(session.requireApi);
const params = (item: BaseItemDto) => ({ itemId: item.Id!, userId: session.userId });

export async function markWatched(item: BaseItemDto, watched: boolean) {
	await (watched
		? userData().markPlayedItem(params(item))
		: userData().markUnplayedItem(params(item)));
	await invalidateAll();
}

/** Plex's "Remove from Continue Watching": forget the resume position. */
export async function removeFromContinueWatching(item: BaseItemDto) {
	await userData().updateItemUserData({
		...params(item),
		updateUserItemDataDto: { PlaybackPositionTicks: 0 }
	});
	await invalidateAll();
}

/** Plex's 5-star rating, stored as Jellyfin's 0–10 user rating (null clears). */
export async function rate(item: BaseItemDto, stars: number | null) {
	if (stars == null)
		await userData()
			.deleteUserItemRating(params(item))
			.catch(() => {});
	else
		await userData().updateItemUserData({
			...params(item),
			updateUserItemDataDto: { Rating: stars * 2 }
		});
	await invalidateAll();
}

export async function setFavorite(item: BaseItemDto, favorite: boolean) {
	await (favorite
		? userData().markFavoriteItem(params(item))
		: userData().unmarkFavoriteItem(params(item)));
	await invalidateAll();
}

export function downloadUrl(item: BaseItemDto): string {
	return getLibraryApi(session.requireApi).getDownloadUrl({ itemId: item.Id! });
}

const isAudio = (item: BaseItemDto) =>
	item.MediaType === 'Audio' || ['MusicAlbum', 'MusicArtist', 'Audio'].includes(item.Type ?? '');

// Playlists.
export async function listPlaylists(): Promise<BaseItemDto[]> {
	const { data } = await getLibraryApi(session.requireApi).getItems({
		userId: session.userId,
		includeItemTypes: ['Playlist'],
		recursive: true,
		sortBy: ['SortName']
	});
	return data.Items ?? [];
}

/** The playable items an item adds to a playlist or queue. */
async function contents(item: BaseItemDto): Promise<BaseItemDto[]> {
	// Play's queue starts at Next Up and runs on to the series' end; adding
	// means just the item: one episode, or all of a season or series.
	if (item.Type === 'Episode') return [item];
	if (item.Type === 'Series' || item.Type === 'Season') {
		const { data } = await getShowApi(session.requireApi).getEpisodes({
			seriesId: item.Type === 'Series' ? item.Id! : item.SeriesId!,
			seasonId: item.Type === 'Season' ? item.Id! : undefined,
			userId: session.userId
		});
		return data.Items ?? [];
	}
	return resolveQueue(item);
}

async function contentsOf(items: BaseItemDto[]): Promise<BaseItemDto[]> {
	return (await Promise.all(items.map(contents))).flat();
}

export async function addToPlaylist(playlist: BaseItemDto, items: BaseItemDto[]) {
	const entries = await contentsOf(items);
	await getPlaylistApi(session.requireApi).addItemToPlaylist({
		playlistId: playlist.Id!,
		ids: entries.map((i) => i.Id!),
		userId: session.userId
	});
	await invalidateAll();
}

export async function createPlaylist(name: string, items: BaseItemDto[] = []) {
	const entries = await contentsOf(items);
	const { data } = await getPlaylistApi(session.requireApi).createPlaylist({
		createPlaylistDto: {
			Name: name,
			Ids: entries.map((i) => i.Id!),
			UserId: session.userId,
			MediaType: items[0] && isAudio(items[0]) ? 'Audio' : 'Video'
		}
	});
	await invalidateAll();
	return data.Id;
}

let playlistCache = $state<BaseItemDto[]>([]);
/** Refreshes the playlists the "Add to" submenu offers. */
export async function loadPlaylists() {
	playlistCache = await listPlaylists().catch(() => []);
}

/** Playlists that can hold these items (audio and video don't mix). */
export function matchingPlaylists(items: BaseItemDto[]): BaseItemDto[] {
	const audio = !!items[0] && isAudio(items[0]);
	return playlistCache.filter((p) => !p.MediaType || (p.MediaType === 'Audio') === audio);
}

/**
 * Plex's "Add to" menu (the card ⋮ submenu and the toolbar's Add to…):
 * "Add to Playlist..." opens the playlist picker, then a "Recent" header
 * and the playlists.
 */
export function addToEntries(items: BaseItemDto[]): MenuEntry[] {
	const matching = matchingPlaylists(items);
	return [
		{
			label: 'Add to Playlist...',
			onselect: () => modals.open({ kind: 'add-to-playlist', items })
		},
		...(matching.length ? [{ heading: 'Recent' }] : []),
		...matching.map((p) => ({
			label: p.Name ?? 'Playlist',
			onselect: () => addToPlaylist(p, items)
		}))
	];
}

/**
 * Plex's item menu (MetadataPosterCard's ⋮): Play Next, Add to Queue,
 * Add to ▸, then watched state, Remove from Continue Watching, Save File
 * and Get Info.
 */
export function itemMenu(
	item: BaseItemDto,
	opts: { continueWatching?: boolean } = {}
): MenuEntry[] {
	const playable = !['Person', 'Genre', 'Studio'].includes(item.Type ?? '');
	const folder = !!item.IsFolder;
	const played = !!item.UserData?.Played;
	const partly =
		(item.UserData?.PlaybackPositionTicks ?? 0) > 0 ||
		(folder && (item.UserData?.PlayedPercentage ?? 0) > 0);
	const watchable = !isAudio(item) && item.Type !== 'Playlist';
	const entries: MenuEntry[] = [];
	if (playable) {
		entries.push(
			{ label: 'Play Next', onselect: async () => player.addToQueue(await contents(item), true) },
			{ label: 'Add to Queue', onselect: async () => player.addToQueue(await contents(item)) }
		);
		if (item.Type !== 'Playlist') entries.push({ label: 'Add to', items: addToEntries([item]) });
	}
	if (watchable) {
		if (entries.length) entries.push({ separator: true });
		if (!played || partly)
			entries.push({ label: 'Mark as Watched', onselect: () => markWatched(item, true) });
		if (played || partly)
			entries.push({ label: 'Mark as Unwatched', onselect: () => markWatched(item, false) });
	}
	if (opts.continueWatching && (item.UserData?.PlaybackPositionTicks ?? 0) > 0)
		entries.push({
			label: 'Remove from Continue Watching',
			onselect: () => removeFromContinueWatching(item)
		});
	if (!folder && item.MediaType) {
		entries.push(
			{ label: 'Save File', onselect: () => window.open(downloadUrl(item), '_blank', 'noopener') },
			{ label: 'Get Info', onselect: () => modals.open({ kind: 'info', item }) }
		);
	}
	return entries;
}
