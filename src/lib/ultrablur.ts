// Plex's "ultrablur": detail pages tint the four-corner backdrop with the
// artwork's colours, capped at 30% brightness (--ultrablur-max-brightness).
// Plex samples the image; Jellyfin already gives us a blurhash per image,
// which decodes to a tiny thumbnail whose corners serve the same purpose.
import type { BaseItemDto } from '@jellyfin/sdk/lib/generated-client';
import { decode } from 'blurhash';

const CORNERS = ['tl', 'tr', 'br', 'bl'] as const;
const MAX_BRIGHTNESS = 0.3;

function hash(item: BaseItemDto): string | undefined {
	const h = item.ImageBlurHashes;
	if (!h) return undefined;
	for (const type of ['Backdrop', 'Art', 'Thumb', 'Primary'] as const) {
		const v = h[type];
		if (v) {
			const first = Object.values(v)[0];
			if (first) return first;
		}
	}
	return undefined;
}

/** Scales a colour so its HSV value is at most MAX_BRIGHTNESS. */
function cap(r: number, g: number, b: number): string {
	const v = Math.max(r, g, b) / 255;
	const k = v > MAX_BRIGHTNESS ? MAX_BRIGHTNESS / v : 1;
	const c = (x: number) => Math.round(x * k);
	return `rgb(${c(r)}, ${c(g)}, ${c(b)})`;
}

/** The four corner colours for an item's artwork, or null. */
export function colorsFrom(item: BaseItemDto): Record<(typeof CORNERS)[number], string> | null {
	const h = hash(item);
	if (!h) return null;
	let px: Uint8ClampedArray;
	try {
		px = decode(h, 8, 8);
	} catch {
		return null;
	}
	// Average a 2×2 block in each corner.
	const at = (x: number, y: number) => {
		let r = 0,
			g = 0,
			b = 0;
		for (const [dx, dy] of [
			[0, 0],
			[1, 0],
			[0, 1],
			[1, 1]
		]) {
			const i = ((y + dy) * 8 + (x + dx)) * 4;
			r += px[i];
			g += px[i + 1];
			b += px[i + 2];
		}
		return cap(r / 4, g / 4, b / 4);
	};
	return { tl: at(0, 0), tr: at(6, 0), br: at(6, 6), bl: at(0, 6) };
}

/** Tints the page backdrop from the item's artwork; returns a reset. */
export function tintFrom(item: BaseItemDto): () => void {
	const root = document.documentElement;
	const colors = colorsFrom(item);
	if (!colors) return () => {};
	for (const c of CORNERS) root.style.setProperty(`--color-ultrablur-${c}`, colors[c]);
	return () => {
		for (const c of CORNERS) root.style.removeProperty(`--color-ultrablur-${c}`);
	};
}
