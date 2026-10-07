// The signed-in Jellyfin session: one Api instance plus the current user,
// persisted in localStorage so a reload reconnects without signing in again.
import { Jellyfin } from '@jellyfin/sdk';
import type { Api } from '@jellyfin/sdk/lib/api';
import type { UserDto } from '@jellyfin/sdk/lib/generated-client';
import { getAuthenticationApi } from '@jellyfin/sdk/lib/utils/api/authentication-api';
import { getSystemApi } from '@jellyfin/sdk/lib/utils/api/system-api';
import { getUserApi } from '@jellyfin/sdk/lib/utils/api/user-api';

const SESSION_KEY = 'jellex.session';
const DEVICE_KEY = 'jellex.device';
const SERVER_KEY = 'jellex.server';

interface Saved {
	serverUrl: string;
	token: string;
}

function read(key: string): string | null {
	try {
		return localStorage.getItem(key);
	} catch {
		return null;
	}
}

function write(key: string, value: string | null) {
	try {
		if (value === null) localStorage.removeItem(key);
		else localStorage.setItem(key, value);
	} catch {
		// Private mode or blocked storage: the session just won't survive a reload.
	}
}

function deviceId(): string {
	let id = read(DEVICE_KEY);
	if (!id) {
		id = crypto.randomUUID();
		write(DEVICE_KEY, id);
	}
	return id;
}

function deviceName(): string {
	const ua = navigator.userAgent;
	const browser = /Edg\//.test(ua)
		? 'Edge'
		: /OPR\//.test(ua)
			? 'Opera'
			: /Firefox\//.test(ua)
				? 'Firefox'
				: /Chrome\//.test(ua)
					? 'Chrome'
					: /Safari\//.test(ua)
						? 'Safari'
						: 'Browser';
	const os = /Windows/.test(ua)
		? 'Windows'
		: /Mac OS/.test(ua)
			? 'macOS'
			: /Android/.test(ua)
				? 'Android'
				: /iPhone|iPad/.test(ua)
					? 'iOS'
					: /Linux/.test(ua)
						? 'Linux'
						: '';
	return os ? `${browser} on ${os}` : browser;
}

export const jellyfin = new Jellyfin({
	clientInfo: { name: 'jellex', version: '0.1.0' },
	deviceInfo: { name: deviceName(), id: deviceId() }
});

/** Jellyfin's public demo server (user "demo", no password). */
export const DEMO_SERVER = 'https://demo.jellyfin.org/stable';

/** Accepts "host", "host:8096", "https://host/jellyfin/"; returns a base URL. */
export function normalizeServerUrl(input: string): string {
	let url = input.trim();
	if (!/^https?:\/\//i.test(url)) url = `http://${url}`;
	return url.replace(/\/+$/, '');
}

/** The server URL to prefill on the sign-in page: the last one used, or none. */
export function defaultServerUrl(): string {
	return read(SERVER_KEY) ?? '';
}

/** A server found by `findServer`, ready to sign in to. */
export interface FoundServer {
	url: string;
	name: string;
	quickConnect: boolean;
}

/**
 * Finds the Jellyfin server behind what the user typed ("host",
 * "host:8096", "https://host/jellyfin"): tries the same address candidates
 * as Jellyfin's own clients (https and Jellyfin's default ports) at once and
 * takes the first that answers. Null if none does.
 */
export async function findServer(input: string): Promise<FoundServer | null> {
	let candidates = jellyfin.discovery.getAddressCandidates(input.trim());
	// An https page can't call an http server, so don't try.
	if (location.protocol === 'https:') candidates = candidates.filter((c) => c.startsWith('https:'));
	try {
		return await Promise.any(
			candidates.map(async (candidate) => {
				const url = candidate.replace(/\/+$/, '');
				const a = jellyfin.createApi(url);
				const { data } = await getSystemApi(a).getPublicSystemInfo({ timeout: 5000 });
				if (!data.Id) throw new Error('not a Jellyfin server');
				const quickConnect = await getAuthenticationApi(a)
					.getQuickConnectEnabled()
					.then((r) => r.data === true)
					.catch(() => false);
				return { url, name: data.ServerName || url, quickConnect };
			})
		);
	} catch {
		return null;
	}
}

let api = $state<Api | null>(null);
let user = $state<UserDto | null>(null);
let restored = false;

function adopt(a: Api, u: UserDto) {
	api = a;
	user = u;
	write(SERVER_KEY, a.basePath);
	write(
		SESSION_KEY,
		JSON.stringify({ serverUrl: a.basePath, token: a.accessToken } satisfies Saved)
	);
}

export const session = {
	get api(): Api | null {
		return api;
	},
	get user(): UserDto | null {
		return user;
	},
	get signedIn(): boolean {
		return api !== null && user !== null;
	},
	/** The Api, for code that only runs once signed in. */
	get requireApi(): Api {
		if (!api) throw new Error('not signed in');
		return api;
	},
	get userId(): string {
		if (!user?.Id) throw new Error('not signed in');
		return user.Id;
	},

	/** Reconnects with the saved token. Runs once per page load. */
	async restore(): Promise<boolean> {
		if (restored) return this.signedIn;
		restored = true;
		const raw = read(SESSION_KEY);
		if (!raw) return false;
		try {
			const saved = JSON.parse(raw) as Saved;
			const a = jellyfin.createApi(saved.serverUrl, saved.token);
			const { data } = await getUserApi(a).getCurrentUser();
			api = a;
			user = data;
			return true;
		} catch (e) {
			// Only a rejected token is forgotten; an unreachable server keeps
			// it for the next reload.
			const status = (e as { response?: { status?: number } })?.response?.status;
			if (status === 401 || status === 403) write(SESSION_KEY, null);
			return false;
		}
	},

	async signIn(serverUrl: string, username: string, password: string): Promise<void> {
		const a = jellyfin.createApi(normalizeServerUrl(serverUrl));
		const { data } = await a.authenticateUserByName(username, password);
		if (!data.AccessToken || !data.User) throw new Error('Jellyfin returned no token');
		adopt(jellyfin.createApi(a.basePath, data.AccessToken), data.User);
	},

	/** Starts Quick Connect; returns the code to show and a poll function. */
	async startQuickConnect(
		serverUrl: string
	): Promise<{ code: string; poll: () => Promise<boolean> }> {
		const a = jellyfin.createApi(normalizeServerUrl(serverUrl));
		const auth = getAuthenticationApi(a);
		const { data } = await auth.initiateQuickConnect();
		const secret = data.Secret;
		if (!data.Code || !secret) throw new Error('Quick Connect is not enabled on this server');
		return {
			code: data.Code,
			poll: async () => {
				const state = await auth.getQuickConnectState({ secret });
				if (!state.data.Authenticated) return false;
				const { data: result } = await auth.authenticateWithQuickConnect({
					quickConnectDto: { Secret: secret }
				});
				if (!result.AccessToken || !result.User) throw new Error('Jellyfin returned no token');
				adopt(jellyfin.createApi(a.basePath, result.AccessToken), result.User);
				return true;
			}
		};
	},

	async signOut(): Promise<void> {
		try {
			await api?.logout();
		} catch {
			// The token may already be gone; forget it either way.
		}
		api = null;
		user = null;
		write(SESSION_KEY, null);
	}
};
