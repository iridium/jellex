// Image URLs for Jellyfin items. Jellyfin crops to fillWidth × fillHeight,
// so every card gets exactly the aspect ratio it asks for.
import type { BaseItemDto } from '@jellyfin/sdk/lib/generated-client';
import { session } from './session.svelte';

export type ImageKind = 'poster' | 'landscape' | 'square' | 'backdrop';

/** Width : height for each card shape. */
export const aspect: Record<ImageKind, number> = {
	poster: 2 / 3,
	landscape: 16 / 9,
	square: 1,
	backdrop: 16 / 9
};

function url(itemId: string, type: string, tag: string | undefined, w: number, h: number) {
	const dpr = Math.min(window.devicePixelRatio || 1, 2);
	const q = new URLSearchParams({
		fillWidth: String(Math.round(w * dpr)),
		fillHeight: String(Math.round(h * dpr)),
		quality: '90'
	});
	if (tag) q.set('tag', tag);
	return `${session.requireApi.basePath}/Items/${itemId}/Images/${type}?${q}`;
}

/** Picks the best source image for a card shape, or undefined if the item has none. */
export function imageUrl(item: BaseItemDto, kind: ImageKind, width: number): string | undefined {
	const height = Math.round(width / aspect[kind]);
	const tags = item.ImageTags ?? {};
	const id = item.Id!;

	if (kind === 'landscape' || kind === 'backdrop') {
		if (item.Type === 'Episode' && tags.Primary)
			return url(id, 'Primary', tags.Primary, width, height);
		if (tags.Thumb) return url(id, 'Thumb', tags.Thumb, width, height);
		if (item.BackdropImageTags?.length)
			return url(id, 'Backdrop', item.BackdropImageTags[0], width, height);
		if (item.ParentThumbItemId && item.ParentThumbImageTag)
			return url(item.ParentThumbItemId, 'Thumb', item.ParentThumbImageTag, width, height);
		if (item.ParentBackdropItemId && item.ParentBackdropImageTags?.length)
			return url(
				item.ParentBackdropItemId,
				'Backdrop',
				item.ParentBackdropImageTags[0],
				width,
				height
			);
		if (tags.Primary) return url(id, 'Primary', tags.Primary, width, height);
		return undefined;
	}

	// Plex shows an episode as its series' poster (or season's) in poster rows.
	if (item.Type === 'Episode') {
		if (item.SeriesId && item.SeriesPrimaryImageTag)
			return url(item.SeriesId, 'Primary', item.SeriesPrimaryImageTag, width, height);
		if (item.ParentId && item.ParentPrimaryImageTag)
			return url(item.ParentId, 'Primary', item.ParentPrimaryImageTag, width, height);
	}
	if (tags.Primary) return url(id, 'Primary', tags.Primary, width, height);
	// Seasons without their own poster fall back to the series'.
	if (item.SeriesId && item.SeriesPrimaryImageTag)
		return url(item.SeriesId, 'Primary', item.SeriesPrimaryImageTag, width, height);
	if (item.AlbumId && item.AlbumPrimaryImageTag)
		return url(item.AlbumId, 'Primary', item.AlbumPrimaryImageTag, width, height);
	return undefined;
}
