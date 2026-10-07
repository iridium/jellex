// Per-browser preferences from the Settings pages (Plex Web keeps these in
// its local "user settings" too). Saved with "Save Changes".

export interface Settings {
	/** General: "Remember selected tab" (Plex's default: on). */
	rememberTab: boolean;
	/** Player: Plex's subtitle appearance for text subtitles. */
	subtitleColor: string;
	subtitlePosition: 'top' | 'middle' | 'bottom';
	/** Percent of the normal size. */
	subtitleSize: number;
	/** Theme: the accent colour (hex). */
	accent: string;
}

const KEY = 'jellex.settings';
const defaults: Settings = {
	rememberTab: true,
	subtitleColor: '#ffffff',
	subtitlePosition: 'bottom',
	subtitleSize: 100,
	accent: '#00a4dc'
};

function read(): Settings {
	try {
		return { ...defaults, ...JSON.parse(localStorage.getItem(KEY) ?? '{}') };
	} catch {
		return { ...defaults };
	}
}

let current = $state<Settings>(read());

export const settings = {
	get value(): Settings {
		return current;
	},
	save(next: Settings) {
		current = { ...next };
		try {
			localStorage.setItem(KEY, JSON.stringify(current));
		} catch {
			// Not persisted.
		}
	}
};

// "Remember selected tab": the last pivot per library.
const PIVOTS = 'jellex.pivots';
export function rememberedPivot(viewId: string): string | undefined {
	if (!current.rememberTab) return undefined;
	try {
		return JSON.parse(localStorage.getItem(PIVOTS) ?? '{}')[viewId];
	} catch {
		return undefined;
	}
}
export function rememberPivot(viewId: string, pivot: string) {
	try {
		const all = JSON.parse(localStorage.getItem(PIVOTS) ?? '{}');
		all[viewId] = pivot;
		localStorage.setItem(PIVOTS, JSON.stringify(all));
	} catch {
		// Not persisted.
	}
}

/** "Reset Customization": the sidebar's pins and collapsed state. */
export function resetCustomization() {
	for (const key of ['jellex.pins', 'jellex.sidebarCollapsed', PIVOTS]) {
		try {
			localStorage.removeItem(key);
		} catch {
			// Nothing stored.
		}
	}
}

/** The Settings sidebar (Plex's "jellex" list: General, Player). */
export const settingsPages = [
	{ key: 'general', label: 'General' },
	{ key: 'player', label: 'Player' },
	{ key: 'theme', label: 'Theme' }
] as const;

/**
 * Settings > Theme. Presets carry their own dark and light shades (Plex's
 * orange set, and the Jellyfin blue that replaces it); other colours mix
 * theirs from the main colour.
 */
export const accentPresets = [
	{ label: 'Jellyfin Blue', main: '#00a4dc', dark: '#0083b0', light: '#2cb9ec' },
	{ label: 'Plex Orange', main: '#e5a00d', dark: '#cc7b19', light: '#f9be03' },
	{ label: 'Purple', main: '#aa5cc3' },
	{ label: 'Green', main: '#3fb950' },
	{ label: 'Red', main: '#e5484d' },
	{ label: 'Pink', main: '#e45ba0' }
] as const;

export function applyTheme(accent: string) {
	const preset = accentPresets.find((p) => p.main === accent.toLowerCase());
	const dark =
		(preset && 'dark' in preset && preset.dark) || `color-mix(in srgb, ${accent} 80%, black)`;
	const light =
		(preset && 'light' in preset && preset.light) || `color-mix(in srgb, ${accent} 80%, white)`;
	const root = document.documentElement.style;
	for (const name of [
		'--color-brand-accent',
		'--color-static-yellow',
		'--color-text-accent',
		'--color-background-accent'
	])
		root.setProperty(name, accent);
	root.setProperty('--color-background-accent-focus', light);
	root.setProperty('--color-accent-light', light);
	root.setProperty('--color-accent-dark', dark);
	// The wordmark's "x": Jellyfin's purple-to-blue by default, else the
	// accent's light-to-main.
	const jellyfin = accent.toLowerCase() === '#00a4dc';
	root.setProperty('--wordmark-from', jellyfin ? '#aa5cc3' : light);
	root.setProperty('--wordmark-to', accent);
}
