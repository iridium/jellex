// The app-wide player, a copy of Plex Web's: one media element that lives
// in the root layout and survives navigation, shown either full screen or
// as the 100px mini player under the page. Music opens in the mini player
// (as in Plex), video full screen.
import type { BaseItemDto, ChapterInfo } from '@jellyfin/sdk/lib/generated-client';
import { getLibraryApi } from '@jellyfin/sdk/lib/utils/api/library-api';
import { getMediaSegmentApi } from '@jellyfin/sdk/lib/utils/api/media-segment-api';
import { getSessionApi } from '@jellyfin/sdk/lib/utils/api/session-api';
import Hls from 'hls.js';
import { getStream, resolveQueue, type Stream } from './playback.ts';
import { session } from './session.svelte.ts';

export type Mode = 'closed' | 'full' | 'mini';
export type Repeat = 'off' | 'all' | 'one';
export type Panel = 'none' | 'settings' | 'queue' | 'chapters';

export interface Marker {
	type: 'intro' | 'credits';
	start: number;
	end: number;
}

/** Plex's "Convert to …" presets: label and bitrate (bits/s). */
export const QUALITIES = [
	{ label: '1080p High', detail: '20 Mbps', bitrate: 20_000_000 },
	{ label: '1080p Medium', detail: '12 Mbps', bitrate: 12_000_000 },
	{ label: '1080p', detail: '10 Mbps', bitrate: 10_000_000 },
	{ label: '1080p', detail: '8 Mbps', bitrate: 8_000_000 },
	{ label: '720p High', detail: '4 Mbps', bitrate: 4_000_000 },
	{ label: '720p Medium', detail: '3 Mbps', bitrate: 3_000_000 },
	{ label: '720p', detail: '2 Mbps', bitrate: 2_000_000 },
	{ label: '480p', detail: '1.5 Mbps', bitrate: 1_500_000 },
	{ label: '480p', detail: '720 kbps', bitrate: 720_000 },
	{ label: '240p', detail: '320 kbps', bitrate: 320_000 }
] as const;

const NOW_PLAYING = 'jellex.nowPlaying';
interface Saved {
	server?: string;
	user?: string;
	ids: string[];
	index: number;
	position: number;
	mode: 'full' | 'mini';
	shuffle: boolean;
	repeat: Repeat;
	audio?: number;
	subtitle?: number;
	maxBitrate: number | null;
}

function stored<T>(key: string, fallback: T): T {
	try {
		const v = localStorage.getItem(key);
		return v === null ? fallback : (JSON.parse(v) as T);
	} catch {
		return fallback;
	}
}
function store(key: string, value: unknown) {
	try {
		localStorage.setItem(key, JSON.stringify(value));
	} catch {
		// Not persisted.
	}
}

function shuffled<T>(a: T[]): T[] {
	const r = [...a];
	for (let i = r.length - 1; i > 0; i--) {
		const j = Math.floor(Math.random() * (i + 1));
		[r[i], r[j]] = [r[j], r[i]];
	}
	return r;
}

function markersFor(
	item: BaseItemDto,
	segments: { Type?: string; StartTicks?: number; EndTicks?: number }[]
): Marker[] {
	const out: Marker[] = [];
	for (const s of segments) {
		const type = s.Type === 'Intro' ? 'intro' : s.Type === 'Outro' ? 'credits' : null;
		if (type && s.StartTicks != null && s.EndTicks != null)
			out.push({ type, start: s.StartTicks / 1e7, end: s.EndTicks / 1e7 });
	}
	if (out.length) return out;
	// No media segments: chapters named like an intro or credits stand in
	// for them, as in the old jellex.
	const chapters: ChapterInfo[] = item.Chapters ?? [];
	const runtime = (item.RunTimeTicks ?? 0) / 1e7;
	chapters.forEach((c, i) => {
		const name = c.Name ?? '';
		const type = /\b(intro|opening)\b/i.test(name)
			? 'intro'
			: /\b(credits|outro|ending)\b/i.test(name)
				? 'credits'
				: null;
		if (!type) return;
		const start = (c.StartPositionTicks ?? 0) / 1e7;
		const end = i + 1 < chapters.length ? (chapters[i + 1].StartPositionTicks ?? 0) / 1e7 : runtime;
		if (end > start) out.push({ type, start, end });
	});
	return out;
}

class Player {
	mode = $state<Mode>('closed');
	queue = $state<BaseItemDto[]>([]);
	index = $state(0);
	/** The current item with full details (chapters, trickplay, sources). */
	item = $state<BaseItemDto | null>(null);
	stream = $state<Stream | null>(null);
	loading = $state(false);
	error = $state('');
	paused = $state(true);
	currentTime = $state(0);
	duration = $state(0);
	buffered = $state(0);
	volume = $state(stored('jellex.volume', 1));
	muted = $state(stored('jellex.muted', false));
	repeat = $state<Repeat>('off');
	shuffle = $state(false);
	autoplay = $state(stored('jellex.autoplay', true));
	panel = $state<Panel>('none');
	fullscreen = $state(false);
	/** The media element has rendered a frame (Plex then clears the full player's background). */
	hasFrames = $state(false);
	markers = $state<Marker[]>([]);
	audioIndex = $state<number | undefined>(undefined);
	subtitleIndex = $state<number | undefined>(undefined);
	/** null = Original, otherwise a bitrate cap. */
	maxBitrate = $state<number | null>(null);
	/** Plex's "Resume Playback" question, when Play needs an answer. */
	resumePrompt = $state<{ item: BaseItemDto; resolve: (fromStart: boolean | null) => void } | null>(
		null
	);

	el: HTMLMediaElement | null = null;
	private hls: Hls | null = null;
	private progressTimer: ReturnType<typeof setInterval> | undefined;
	private loadToken = 0;
	private unshuffled: BaseItemDto[] | null = null;
	/** Restored after a reload but not played yet: Jellyfin hears about
	 *  the session only once it really starts. */
	private pendingStart = false;
	private lastSave = 0;
	private pagehideBound = false;

	get current(): BaseItemDto | undefined {
		return this.item ?? this.queue[this.index];
	}
	get isAudio(): boolean {
		return this.current?.MediaType === 'Audio';
	}
	get hasNext(): boolean {
		return this.index < this.queue.length - 1 || (this.repeat === 'all' && this.queue.length > 0);
	}
	get hasPrevious(): boolean {
		return this.index > 0;
	}

	/** Called by the Player component with its media element. */
	attach(el: HTMLMediaElement) {
		this.el = el;
		el.volume = this.volume;
		el.muted = this.muted;
		el.addEventListener('timeupdate', () => {
			this.currentTime = el.currentTime;
			if (Date.now() - this.lastSave > 2000) this.persist();
		});
		el.addEventListener('durationchange', () => {
			if (Number.isFinite(el.duration) && el.duration > 0) this.duration = el.duration;
		});
		el.addEventListener('progress', () => {
			const b = el.buffered;
			this.buffered = b.length ? b.end(b.length - 1) : 0;
		});
		el.addEventListener('play', () => {
			this.paused = false;
			if (this.pendingStart) {
				this.pendingStart = false;
				this.reportStart();
			} else this.reportProgress();
			this.persist();
		});
		el.addEventListener(
			'pause',
			() => ((this.paused = true), this.reportProgress(), this.persist())
		);
		el.addEventListener('seeked', () => (this.reportProgress(), this.persist()));
		el.addEventListener('volumechange', () => {
			this.volume = el.volume;
			this.muted = el.muted;
			store('jellex.volume', el.volume);
			store('jellex.muted', el.muted);
		});
		el.addEventListener('ended', () => this.ended());
		el.addEventListener('loadeddata', () => (this.hasFrames = true));
		// The Player component remounts on every sign-in; listen once.
		if (!this.pagehideBound) {
			this.pagehideBound = true;
			window.addEventListener('pagehide', () => {
				this.persist();
				this.reportStop(true);
			});
		}
		void this.restore();
	}

	// What's playing survives a reload: the queue, position and player
	// state are saved as they change and restored, paused, on start.
	private persist() {
		this.lastSave = Date.now();
		if (this.mode === 'closed' || !this.queue.length) return;
		store(NOW_PLAYING, {
			server: session.api?.basePath,
			user: session.userId,
			ids: this.queue.map((i) => i.Id!),
			index: this.index,
			position: this.el?.currentTime ?? this.currentTime,
			mode: this.mode,
			shuffle: this.shuffle,
			repeat: this.repeat,
			audio: this.audioIndex,
			subtitle: this.subtitleIndex,
			maxBitrate: this.maxBitrate
		} satisfies Saved);
	}

	private async restore() {
		const saved = stored<Saved | null>(NOW_PLAYING, null);
		if (!saved || this.mode !== 'closed' || !session.api) return;
		if (saved.server !== session.api.basePath || saved.user !== session.userId || !saved.ids.length)
			return;
		try {
			const { data } = await getLibraryApi(session.requireApi).getItems({
				userId: session.userId,
				ids: saved.ids,
				fields: ['MediaSources', 'Chapters']
			});
			const byId = new Map((data.Items ?? []).map((i) => [i.Id, i]));
			const queue = saved.ids.map((id) => byId.get(id)).filter((i): i is BaseItemDto => !!i);
			if (!queue.length || this.mode !== 'closed') return;
			const index = Math.min(
				Math.max(
					0,
					queue.findIndex((i) => i.Id === saved.ids[saved.index])
				),
				queue.length - 1
			);
			this.queue = queue;
			this.shuffle = saved.shuffle;
			this.repeat = saved.repeat;
			this.audioIndex = saved.audio;
			this.subtitleIndex = saved.subtitle;
			this.maxBitrate = saved.maxBitrate;
			// After a reload the player always comes back minimised.
			this.mode = 'mini';
			await this.load(index, Math.round(saved.position * 1e7), false);
		} catch {
			// Nothing to restore.
		}
	}

	/**
	 * Starts playing an item (or the queue Play makes for it: a series from
	 * its next episode, an album's tracks, a library, …).
	 */
	async play(
		target: BaseItemDto | string,
		opts: {
			shuffle?: boolean;
			/** Ask Plex's resume question for a partly watched item. */
			askResume?: boolean;
			fromStart?: boolean;
			audio?: number;
			subtitle?: number;
			queue?: BaseItemDto[];
			startIndex?: number;
		} = {}
	) {
		const library = getLibraryApi(session.requireApi);
		const item =
			typeof target === 'string'
				? (await library.getItem({ itemId: target, userId: session.userId })).data
				: target;
		// A given queue is in its own order; Shuffle shuffles it here and
		// keeps that order for turning shuffle off again.
		const given = opts.shuffle && opts.queue ? opts.queue : null;
		const queue = given
			? shuffled(given)
			: (opts.queue ?? (await resolveQueue(item, !!opts.shuffle)));
		if (!queue.length) return;
		const startIndex = given ? 0 : (opts.startIndex ?? 0);
		const first = queue[startIndex];

		let fromStart = !!opts.fromStart;
		// Plex asks over the current page; the player opens after the answer.
		if (
			opts.askResume &&
			!fromStart &&
			(first.UserData?.PlaybackPositionTicks ?? 0) > 0 &&
			first.MediaType !== 'Audio'
		) {
			const answer = await new Promise<boolean | null>(
				(resolve) => (this.resumePrompt = { item: first, resolve })
			);
			this.resumePrompt = null;
			if (answer === null) return;
			fromStart = answer;
		}

		this.unshuffled = given;
		this.shuffle = !!opts.shuffle;
		this.queue = queue;
		this.audioIndex = opts.audio;
		this.subtitleIndex = opts.subtitle;
		this.maxBitrate = null;
		this.panel = 'none';
		this.mode = first.MediaType === 'Audio' ? (this.mode === 'full' ? 'full' : 'mini') : 'full';
		await this.load(startIndex, fromStart ? 0 : (first.UserData?.PlaybackPositionTicks ?? 0));
	}

	/** Loads queue entry `index` at `startTicks`. */
	private async load(index: number, startTicks: number, autoplay = true) {
		const token = ++this.loadToken;
		this.reportStop(false);
		this.teardown();
		this.index = index;
		const entry = this.queue[index];
		this.item = entry;
		this.loading = true;
		this.error = '';
		this.currentTime = startTicks / 1e7;
		this.duration = (entry.RunTimeTicks ?? 0) / 1e7;
		this.buffered = 0;
		this.markers = [];
		this.hasFrames = false;

		try {
			const api = session.requireApi;
			const [{ data: full }, segments] = await Promise.all([
				getLibraryApi(api).getItem({ itemId: entry.Id!, userId: session.userId }),
				getMediaSegmentApi(api)
					.getItemSegments({ itemId: entry.Id! })
					.then((r) => r.data.Items ?? [])
					.catch(() => [])
			]);
			if (token !== this.loadToken) return;
			this.item = full;
			this.markers = markersFor(full, segments);
			const s = await getStream(full, {
				startTicks,
				audio: this.audioIndex,
				subtitle: this.subtitleIndex,
				maxBitrate: this.maxBitrate ?? undefined
			});
			if (token !== this.loadToken) return;
			this.stream = s;
			this.attachStream(s, startTicks / 1e7, autoplay);
			this.pendingStart = !autoplay;
			if (autoplay) this.reportStart();
			this.persist();
		} catch (e) {
			if (token === this.loadToken) this.error = e instanceof Error ? e.message : 'Playback failed';
		} finally {
			if (token === this.loadToken) this.loading = false;
		}
	}

	private attachStream(s: Stream, start: number, autoplay = true) {
		const el = this.el;
		if (!el) return;
		if (s.hls && !el.canPlayType('application/vnd.apple.mpegurl') && Hls.isSupported()) {
			this.hls = new Hls({ startPosition: start });
			this.hls.loadSource(s.url);
			this.hls.attachMedia(el);
		} else {
			el.src = s.url;
			if (start)
				el.addEventListener('loadedmetadata', () => (el.currentTime = start), { once: true });
		}
		if (autoplay) el.play().catch(() => (this.paused = true));
		else this.paused = true;
		clearInterval(this.progressTimer);
		this.progressTimer = setInterval(() => this.reportProgress(), 10_000);
	}

	private teardown() {
		clearInterval(this.progressTimer);
		this.hls?.destroy();
		this.hls = null;
		this.stream = null;
		if (this.el) {
			// Turn subtitles off first: a cue on screen stays drawn after its
			// track is removed or the source changes.
			for (const t of Array.from(this.el.textTracks)) t.mode = 'disabled';
			this.el.removeAttribute('src');
			this.el.load();
		}
	}

	// Reporting to Jellyfin.
	private info() {
		const s = this.stream;
		const item = this.item;
		if (!s || !item) return null;
		return {
			ItemId: item.Id!,
			MediaSourceId: s.source.Id!,
			PlaySessionId: s.info.PlaySessionId,
			PositionTicks: Math.round((this.el?.currentTime ?? this.currentTime) * 1e7),
			IsPaused: this.el?.paused ?? true,
			IsMuted: this.muted,
			VolumeLevel: Math.round(this.volume * 100),
			CanSeek: true,
			PlayMethod: s.method,
			AudioStreamIndex: this.audioIndex,
			SubtitleStreamIndex: this.subtitleIndex,
			RepeatMode:
				this.repeat === 'one'
					? ('RepeatOne' as const)
					: this.repeat === 'all'
						? ('RepeatAll' as const)
						: ('RepeatNone' as const)
		};
	}
	private reportStart() {
		const i = this.info();
		if (i)
			getSessionApi(session.requireApi)
				.reportPlaybackStart({ playbackStartInfo: i })
				.catch(() => {});
	}
	private reportProgress() {
		if (this.pendingStart) return;
		const i = this.info();
		if (i)
			getSessionApi(session.requireApi)
				.reportPlaybackProgress({ playbackProgressInfo: i })
				.catch(() => {});
	}
	/**
	 * A session that ends without a stop report counts as watched to the end
	 * in Jellyfin, so this also runs on pagehide (with keepalive).
	 */
	private reportStop(unloading: boolean) {
		if (this.pendingStart) return;
		const i = this.info();
		if (!i || !session.api) return;
		if (unloading) {
			const api = session.api;
			fetch(`${api.basePath}/Sessions/Playing/Stopped`, {
				method: 'POST',
				keepalive: true,
				headers: { 'Content-Type': 'application/json', Authorization: api.authorizationHeader },
				body: JSON.stringify(i)
			}).catch(() => {});
		} else {
			getSessionApi(session.api)
				.reportPlaybackStopped({ playbackStopInfo: i })
				.catch(() => {});
		}
	}

	// Transport.
	toggle() {
		if (!this.el) return;
		if (this.el.paused) this.el.play().catch(() => {});
		else this.el.pause();
	}
	seek(seconds: number) {
		if (!this.el) return;
		const max = this.duration || this.el.duration || seconds;
		this.el.currentTime = Math.max(0, Math.min(max, seconds));
		this.currentTime = this.el.currentTime;
	}
	skip(delta: number) {
		this.seek((this.el?.currentTime ?? this.currentTime) + delta);
	}
	next() {
		if (this.index < this.queue.length - 1) this.load(this.index + 1, 0);
		else if (this.repeat === 'all' && this.queue.length) this.load(0, 0);
	}
	previous() {
		// Plex: back to the start unless near it, then the previous item.
		if ((this.el?.currentTime ?? 0) > 5 || this.index === 0) this.seek(0);
		else this.load(this.index - 1, 0);
	}
	jumpTo(index: number) {
		if (index >= 0 && index < this.queue.length) this.load(index, 0);
	}
	private ended() {
		if (this.repeat === 'one') {
			this.seek(0);
			this.el?.play().catch(() => {});
			return;
		}
		const more =
			this.index < this.queue.length - 1 || (this.repeat === 'all' && this.queue.length > 0);
		if (more && (this.autoplay || this.isAudio)) this.next();
		else this.close();
	}
	setVolume(v: number) {
		if (!this.el) return;
		this.el.volume = Math.max(0, Math.min(1, v));
		if (this.el.volume > 0) this.el.muted = false;
	}
	toggleMute() {
		if (this.el) this.el.muted = !this.el.muted;
	}
	cycleRepeat() {
		this.repeat = this.repeat === 'off' ? 'all' : this.repeat === 'all' ? 'one' : 'off';
	}
	toggleShuffle() {
		const current = this.queue[this.index];
		if (!this.shuffle) {
			this.unshuffled = this.queue;
			const rest = shuffled(this.queue.filter((_, i) => i !== this.index));
			this.queue = [current, ...rest];
			this.index = 0;
		} else if (this.unshuffled) {
			this.queue = this.unshuffled;
			this.index = Math.max(
				0,
				this.queue.findIndex((q) => q.Id === current.Id)
			);
			this.unshuffled = null;
		}
		this.shuffle = !this.shuffle;
	}
	setAutoplay(v: boolean) {
		this.autoplay = v;
		store('jellex.autoplay', v);
	}
	removeFromQueue(index: number) {
		if (index === this.index) return;
		const q = [...this.queue];
		q.splice(index, 1);
		if (index < this.index) this.index--;
		this.queue = q;
	}
	moveInQueue(from: number, to: number) {
		const q = [...this.queue];
		const [it] = q.splice(from, 1);
		q.splice(to, 0, it);
		const cur = this.queue[this.index];
		this.queue = q;
		this.index = q.indexOf(cur);
	}
	addToQueue(items: BaseItemDto[], playNext = false) {
		if (this.mode === 'closed') return void this.play(items[0], { queue: items });
		const q = [...this.queue];
		q.splice(playNext ? this.index + 1 : q.length, 0, ...items);
		this.queue = q;
	}

	/** Re-requests the stream at the current position (new audio, subtitles, quality). */
	private reload() {
		const t = this.el?.currentTime ?? this.currentTime;
		const paused = this.el?.paused;
		this.load(this.index, Math.round(t * 1e7)).then(() => paused && this.el?.pause());
	}
	setAudio(index: number) {
		this.audioIndex = index;
		this.reload();
	}
	setSubtitle(index: number) {
		this.subtitleIndex = index;
		this.reload();
	}
	setQuality(bitrate: number | null) {
		this.maxBitrate = bitrate;
		this.reload();
	}

	// Presentation.
	minimize() {
		if (document.fullscreenElement) document.exitFullscreen().catch(() => {});
		this.panel = 'none';
		this.mode = 'mini';
		this.persist();
	}
	expand(panel: Panel = 'none') {
		this.mode = 'full';
		this.panel = panel;
		this.persist();
	}
	togglePanel(panel: Panel) {
		if (this.mode === 'mini') return this.expand(panel);
		this.panel = this.panel === panel ? 'none' : panel;
	}
	close() {
		this.reportStop(false);
		this.loadToken++;
		this.teardown();
		if (document.fullscreenElement) document.exitFullscreen().catch(() => {});
		this.mode = 'closed';
		this.pendingStart = false;
		try {
			localStorage.removeItem(NOW_PLAYING);
		} catch {
			// Nothing stored.
		}
		this.queue = [];
		this.item = null;
		this.panel = 'none';
		this.markers = [];
		this.error = '';
		this.loading = false;
	}

	/** The active intro/credits marker at the current time. */
	get activeMarker(): Marker | undefined {
		return this.markers.find((m) => this.currentTime >= m.start && this.currentTime < m.end - 1);
	}
	skipMarker(m: Marker) {
		if (m.type === 'credits' && this.hasNext) this.next();
		else this.seek(m.end);
	}

	/** Jellyfin trickplay tile for a time, as CSS background values. */
	trickplay(seconds: number, width: number) {
		const item = this.item;
		const s = this.stream;
		if (!item?.Trickplay || !s) return null;
		const bySource = item.Trickplay[s.source.Id!] ?? Object.values(item.Trickplay)[0];
		if (!bySource) return null;
		const res = Object.values(bySource)
			.sort((a, b) => (a.Width ?? 0) - (b.Width ?? 0))
			.at(-1);
		if (!res?.Width || !res.Height || !res.TileWidth || !res.TileHeight || !res.Interval)
			return null;
		const perTile = res.TileWidth * res.TileHeight;
		const n = Math.min((res.ThumbnailCount ?? 1) - 1, Math.floor((seconds * 1000) / res.Interval));
		const tile = Math.floor(n / perTile);
		const within = n % perTile;
		const scale = width / res.Width;
		const api = session.requireApi;
		return {
			width,
			height: Math.round(res.Height * scale),
			image: `${api.basePath}/Videos/${item.Id}/Trickplay/${res.Width}/${tile}.jpg?mediaSourceId=${s.source.Id}&api_key=${api.accessToken}`,
			size: `${res.TileWidth * width}px ${res.TileHeight * res.Height * scale}px`,
			position: `${-(within % res.TileWidth) * width}px ${-Math.floor(within / res.TileWidth) * res.Height * scale}px`
		};
	}
}

export const player = new Player();
