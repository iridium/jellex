import { redirect } from '@sveltejs/kit';
import { getSystemApi } from '@jellyfin/sdk/lib/utils/api/system-api';
import { getUserViewApi } from '@jellyfin/sdk/lib/utils/api/user-view-api';
import { session } from '#lib/session.svelte.ts';
import type { LayoutLoad } from './$types';

// Everything under (app) needs a session. The libraries feed the sidebar;
// the server name is the second line of Plex's breadcrumbs.
export const load: LayoutLoad = async ({ parent }) => {
	await parent();
	if (!session.signedIn) redirect(302, '/login');
	const api = session.requireApi;
	const [views, info] = await Promise.all([
		getUserViewApi(api).getUserViews({ userId: session.userId }),
		getSystemApi(api)
			.getPublicSystemInfo()
			.catch(() => null)
	]);
	return {
		views: views.data.Items ?? [],
		serverName: info?.data.ServerName ?? 'Jellyfin',
		serverVersion: info?.data.Version ?? ''
	};
};
