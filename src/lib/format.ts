// Text formatting the way Plex Web writes it.
import type { BaseItemDto } from '@jellyfin/sdk/lib/generated-client';

/** Jellyfin ticks (100 ns) to seconds. */
export const ticksToSeconds = (ticks: number | null | undefined) => (ticks ?? 0) / 1e7;

/**
 * Plex's duration (its module 42464): minutes up to 90 minutes ("74min",
 * "30sec"), then hours and minutes ("1hr 37min", "2hr") up to a day, then
 * days ("2 days, 3hr"). Each unit is rounded down.
 */
export function duration(seconds: number): string {
	const s = Math.floor(seconds);
	if (s <= 0) return '';
	const d = Math.floor(s / 86400);
	const h = Math.floor((s % 86400) / 3600);
	const m = Math.floor((s % 3600) / 60);
	if (s <= 90 * 60) return s >= 60 ? `${Math.floor(s / 60)}min` : `${s}sec`;
	if (s <= 86400) {
		const hours = h + 24 * d;
		return m ? `${hours}hr ${m}min` : `${hours}hr`;
	}
	const days = `${d} ${d === 1 ? 'day' : 'days'}`;
	return s <= 7 * 86400 && h ? `${days}, ${h}hr` : days;
}

/** Plex's clock duration for track lists: "0:02", "4:31", "1:02:09". */
export function clock(seconds: number): string {
	const s = Math.round(seconds);
	const h = Math.floor(s / 3600);
	const m = Math.floor((s % 3600) / 60);
	const ss = String(s % 60).padStart(2, '0');
	return h > 0 ? `${h}:${String(m).padStart(2, '0')}:${ss}` : `${m}:${ss}`;
}

/** "14sec left" for a partly watched item, or undefined. */
export function timeLeft(item: BaseItemDto): string | undefined {
	const total = ticksToSeconds(item.RunTimeTicks);
	const pos = ticksToSeconds(item.UserData?.PlaybackPositionTicks);
	if (!total || !pos || pos >= total) return undefined;
	const left = duration(total - pos);
	return left ? `${left} left` : undefined;
}

/** "October 1, 1962". */
export function longDate(iso: string | null | undefined): string {
	if (!iso) return '';
	return new Date(iso).toLocaleDateString('en-US', {
		year: 'numeric',
		month: 'long',
		day: 'numeric',
		timeZone: 'UTC'
	});
}

/** "S1 · E3", or "" when the numbers are unknown. */
export function episodeCode(item: BaseItemDto): string {
	const s = item.ParentIndexNumber;
	const e = item.IndexNumber;
	return s != null && e != null ? `S${s} · E${e}` : '';
}

/** "1 Episode", "4 Episodes". */
export function plural(n: number, word: string): string {
	return `${n} ${word}${n === 1 ? '' : 's'}`;
}

/**
 * "13 hours ago": Plex formats added dates with date-fns's strict
 * distance (English strings, rounded, one unit: seconds up to a minute,
 * then minutes, hours, days under 30, months under 12, then years).
 */
export function timeAgo(iso: string | null | undefined, now = Date.now()): string {
	if (!iso) return '';
	const ms = now - new Date(iso).getTime();
	const minutes = Math.abs(ms) / 60000;
	let n: number, unit: string;
	if (minutes < 1) [n, unit] = [Math.round(Math.abs(ms) / 1000), 'second'];
	else if (minutes < 60) [n, unit] = [Math.round(minutes), 'minute'];
	else if (minutes < 1440) [n, unit] = [Math.round(minutes / 60), 'hour'];
	else if (minutes < 43200) [n, unit] = [Math.round(minutes / 1440), 'day'];
	else if (minutes < 525600) [n, unit] = [Math.round(minutes / 43200), 'month'];
	else [n, unit] = [Math.round(minutes / 525600), 'year'];
	const text = `${n} ${unit}${n === 1 ? '' : 's'}`;
	return ms < 0 ? `in ${text}` : `${text} ago`;
}
