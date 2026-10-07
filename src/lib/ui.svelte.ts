// Per-browser view preferences, kept in localStorage.

const KEY = 'jellex.posterSize';

/**
 * Plex's poster-size slider: 11 steps from 130px to 200px wide in 7px
 * increments, 165px by default (measured from Plex Web). Landscape cards
 * (episode thumbnails, 360px at the default) scale by the same factor.
 */
export const POSTER_STEPS = 10;
const DEFAULT_STEP = 5;

function readStep(): number {
	try {
		const v = Number(localStorage.getItem(KEY));
		return Number.isInteger(v) && v >= 0 && v <= POSTER_STEPS && localStorage.getItem(KEY) !== null
			? v
			: DEFAULT_STEP;
	} catch {
		return DEFAULT_STEP;
	}
}

let step = $state(readStep());

export const posterSize = {
	get step() {
		return step;
	},
	set step(v: number) {
		step = Math.max(0, Math.min(POSTER_STEPS, Math.round(v)));
		try {
			localStorage.setItem(KEY, String(step));
		} catch {
			// Not persisted.
		}
	},
	/** Poster and square card width in px. */
	get width() {
		return 130 + 7 * step;
	},
	/** Landscape card width in px. */
	get wideWidth() {
		return Math.round(((130 + 7 * step) * 360) / 165);
	}
};

// Plex's list styles (MetadataListStylesMenu), remembered per library.
export type ListStyle = 'grid' | 'detail' | 'table';
const STYLE_KEY = 'jellex.listStyles';
function readStyles(): Record<string, ListStyle> {
	try {
		return JSON.parse(localStorage.getItem(STYLE_KEY) ?? '{}');
	} catch {
		return {};
	}
}
let styles = $state<Record<string, ListStyle>>(readStyles());
export const listStyles = {
	get(key: string): ListStyle {
		return styles[key] ?? 'grid';
	},
	set(key: string, style: ListStyle) {
		styles = { ...styles, [key]: style };
		try {
			localStorage.setItem(STYLE_KEY, JSON.stringify(styles));
		} catch {
			// Not persisted.
		}
	}
};

// Plex's Show / Blur Background on details pages; it stays as left.
const BG_KEY = 'jellex.showBackground';
function readBackground(): boolean {
	try {
		return localStorage.getItem(BG_KEY) === '1';
	} catch {
		return false;
	}
}
let showBackground = $state(readBackground());
export const background = {
	get shown() {
		return showBackground;
	},
	toggle() {
		showBackground = !showBackground;
		try {
			localStorage.setItem(BG_KEY, showBackground ? '1' : '0');
		} catch {
			// Not persisted.
		}
	}
};
