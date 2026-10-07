import type { BaseItemDto } from '@jellyfin/sdk/lib/generated-client';
import { MediaType } from '@jellyfin/sdk/lib/generated-client';
import { getLibraryApi } from '@jellyfin/sdk/lib/utils/api/library-api';
import { getShowApi } from '@jellyfin/sdk/lib/utils/api/show-api';
import type { ImageKind } from '#lib/images.ts';
import { session } from '#lib/session.svelte.ts';
import type { PageLoad } from './$types';

export interface Hub {
	title: string;
	kind: ImageKind;
	items: BaseItemDto[];
	href?: string;
}

const latestKinds: Record<string, ImageKind> = {
	movies: 'poster',
	tvshows: 'poster',
	music: 'square',
	homevideos: 'landscape',
	musicvideos: 'landscape'
};

// Home is Plex's hub list: Continue Watching (Jellyfin's resume items plus
// Next Up), then Recently Added for each library.
async function loadHubs(views: BaseItemDto[]): Promise<Hub[]> {
	const api = session.requireApi;
	const userId = session.userId;
	const library = getLibraryApi(api);

	const latestViews = views.filter((v) => v.CollectionType && v.CollectionType in latestKinds);
	const [resume, nextUp, ...latest] = await Promise.all([
		library.getResumeItems({ userId, limit: 20, mediaTypes: [MediaType.Video] }),
		getShowApi(api).getNextUp({ userId, limit: 20, enableResumable: false }),
		...latestViews.map((v) =>
			library.getLatestMedia({ userId, parentId: v.Id!, limit: 30, groupItems: true })
		)
	]);

	const resumeItems = resume.data.Items ?? [];
	const seen = new Set(resumeItems.map((i) => i.Id));
	const continueWatching = [
		...resumeItems,
		...(nextUp.data.Items ?? []).filter((i) => !seen.has(i.Id))
	];

	return [
		{ title: 'Continue Watching', kind: 'poster' as const, items: continueWatching },
		...latestViews.map((v, i) => ({
			title: `Recently Added ${v.Name}`,
			kind: latestKinds[v.CollectionType!],
			items: latest[i].data,
			href: `/library/${v.Id}?hub=recent`
		}))
	].filter((h) => h.items.length > 0);
}

// Plex shows the page header straight away and a spinner until the hubs
// arrive, so the hubs are returned unawaited and streamed in.
export const load: PageLoad = async ({ parent }) => {
	const { views } = await parent();
	return { hubs: loadHubs(views) };
};
