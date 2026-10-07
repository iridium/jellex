import { redirect } from '@sveltejs/kit';

// Plex opens its settings on the General page.
export const load = () => redirect(307, '/settings/general');
