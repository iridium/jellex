// Car mode's playback engine: Jellyfin's fMP4 HLS, demuxed here, decoded
// with WebCodecs and drawn to a canvas, with the audio scheduled on an
// AudioContext whose clock drives the video. It exposes the slice of
// HTMLMediaElement the player uses (currentTime, paused, play(), the usual
// events, …), so the player drives it like the <video> element.
import { parseInit, parseSegment, type Sample, type Track } from './fmp4.ts';

interface Segment {
	url: string;
	start: number;
	duration: number;
}

/** Seconds of media fetched ahead of the playhead. */
const AHEAD = 20;
/** Seconds of audio decoded and scheduled ahead of the playhead. */
const AUDIO_AHEAD = 2;
/** Decoded video frames kept ahead of the playhead. */
const MAX_FRAMES = 8;

async function fetchOk(url: string, signal: AbortSignal, as: 'text'): Promise<string>;
async function fetchOk(url: string, signal: AbortSignal, as: 'bytes'): Promise<Uint8Array>;
async function fetchOk(
	url: string,
	signal: AbortSignal,
	as: 'text' | 'bytes'
): Promise<string | Uint8Array> {
	// Jellyfin answers segments once the transcoder has written them; a
	// restart after a far seek can fail the first requests.
	for (let attempt = 0; ; attempt++) {
		try {
			const r = await fetch(url, { signal });
			if (!r.ok) throw new Error(`HTTP ${r.status}`);
			return as === 'text' ? r.text() : new Uint8Array(await r.arrayBuffer());
		} catch (e) {
			if (signal.aborted || attempt >= 3) throw e;
			await new Promise((res) => setTimeout(res, 500 * 2 ** attempt));
		}
	}
}

async function readPlaylist(url: string, signal: AbortSignal) {
	let text = await fetchOk(url, signal, 'text');
	let base = url;
	// A master playlist: follow its first variant.
	if (!text.includes('#EXTINF')) {
		const variant = text.split('\n').find((l) => l.trim() && !l.startsWith('#'));
		if (!variant) throw new Error('Empty HLS playlist');
		base = new URL(variant.trim(), url).href;
		text = await fetchOk(base, signal, 'text');
	}
	const map = text.match(/#EXT-X-MAP:URI="([^"]+)"/);
	if (!map) throw new Error('The stream is not fragmented MP4');
	const segments: Segment[] = [];
	let start = 0;
	let duration = 0;
	for (const line of text.split('\n').map((l) => l.trim())) {
		if (line.startsWith('#EXTINF:')) duration = parseFloat(line.slice(8));
		else if (line && !line.startsWith('#')) {
			segments.push({ url: new URL(line, base).href, start, duration });
			start += duration;
		}
	}
	return { init: new URL(map[1], base).href, segments, duration: start };
}

export class CanvasMedia extends EventTarget {
	readonly canvas: HTMLCanvasElement;
	private draw: CanvasRenderingContext2D;
	private audio: AudioContext | null = null;
	private gain: GainNode | null = null;

	private segments: Segment[] = [];
	private tracks: Track[] = [];
	private videoTrack?: Track;
	private audioTrack?: Track;
	private videoDecoder: VideoDecoder | null = null;
	private audioDecoder: AudioDecoder | null = null;
	private videoConfig?: VideoDecoderConfig;
	private audioConfig?: AudioDecoderConfig;

	/** Demuxed, not yet decoded. */
	private videoQueue: Sample[] = [];
	private audioQueue: Sample[] = [];
	/** Decoded, waiting for their time. */
	private frames: VideoFrame[] = [];
	private sources = new Set<AudioBufferSourceNode>();
	/** Before the first keyframe after a seek, nothing can be decoded. */
	private needKey = true;

	private nextSegment = 0;
	private fetching = false;
	private rangeStart = 0;
	private bufferedEnd = 0;
	/** Bumped by open/seek/reset: async work from an older generation is dropped. */
	private generation = 0;
	private abort = new AbortController();

	// The clock: media time `mediaAt` was at AudioContext time `clockAt`.
	// The context is suspended while paused, stalled or ended, so its
	// currentTime (and the scheduled audio) stops with the picture.
	private mediaAt = 0;
	private clockAt = 0;
	private seekTarget: number | null = null;
	private stalled = false;
	private opened = false;

	private _paused = true;
	private _ended = false;
	private _duration = NaN;
	private _volume = 1;
	private _muted = false;
	private hasFrame = false;
	private lastTimeupdate = 0;
	private raf = 0;
	private pumpTimer: ReturnType<typeof setInterval> | undefined;

	constructor(canvas: HTMLCanvasElement) {
		super();
		this.canvas = canvas;
		this.draw = canvas.getContext('2d', { alpha: false })!;
	}

	static get supported(): boolean {
		return typeof VideoDecoder !== 'undefined' && typeof AudioDecoder !== 'undefined';
	}

	// The HTMLMediaElement slice the player uses.
	get paused() {
		return this._paused;
	}
	get ended() {
		return this._ended;
	}
	get duration() {
		return this._duration;
	}
	get currentTime() {
		if (this.seekTarget !== null) return this.seekTarget;
		return this.clock();
	}
	set currentTime(t: number) {
		if (!this.opened) return;
		this.seekTo(t);
	}
	get volume() {
		return this._volume;
	}
	set volume(v: number) {
		this._volume = v;
		this.applyVolume();
	}
	get muted() {
		return this._muted;
	}
	set muted(m: boolean) {
		this._muted = m;
		this.applyVolume();
	}
	get buffered() {
		const ranges = this.opened ? [[this.rangeStart, this.bufferedEnd]] : [];
		return {
			length: ranges.length,
			start: (i: number) => ranges[i][0],
			end: (i: number) => ranges[i][1]
		};
	}
	async play(): Promise<void> {
		if (this._ended) this.seekTo(0);
		if (!this._paused) return;
		this._paused = false;
		this.emit('play');
		this.syncClock();
	}
	pause() {
		if (this._paused) return;
		this._paused = true;
		this.syncClock();
		this.emit('pause');
		this.emit('timeupdate');
	}

	/** Starts loading an HLS (fMP4) URL at `start` seconds, paused. */
	async open(url: string, start = 0) {
		this.reset();
		const gen = this.generation;
		const signal = this.abort.signal;
		this.opened = true;
		this.ensureAudio();
		try {
			const playlist = await readPlaylist(url, signal);
			const init = await fetchOk(playlist.init, signal, 'bytes');
			if (gen !== this.generation) return;
			this.segments = playlist.segments;
			this.tracks = parseInit(init);
			this.videoTrack = this.tracks.find((t) => t.kind === 'video');
			this.audioTrack = this.tracks.find((t) => t.kind === 'audio');
			await this.configure();
			if (gen !== this.generation) return;
			this._duration = playlist.duration;
			this.emit('durationchange');
			this.emit('loadedmetadata');
			this.seekTo(Math.min(start, Math.max(0, playlist.duration - 1)), false);
			this.pumpTimer = setInterval(() => this.tick(), 100);
			this.raf = requestAnimationFrame(this.frame);
		} catch (e) {
			if (gen === this.generation) this.fail(e);
		}
	}

	/** Stops everything and frees the decoders (the media element's removeAttribute('src') + load()). */
	reset() {
		this.generation++;
		this.abort.abort();
		this.abort = new AbortController();
		clearInterval(this.pumpTimer);
		cancelAnimationFrame(this.raf);
		this.flush();
		for (const d of [this.videoDecoder, this.audioDecoder])
			if (d && d.state !== 'closed') d.close();
		this.videoDecoder = this.audioDecoder = null;
		this.segments = [];
		this.tracks = [];
		this.videoTrack = this.audioTrack = undefined;
		this.opened = false;
		this.hasFrame = false;
		this._paused = true;
		this._ended = false;
		this._duration = NaN;
		this.mediaAt = this.rangeStart = this.bufferedEnd = 0;
		this.seekTarget = null;
		this.stalled = false;
		this.syncClock();
		this.draw.fillStyle = '#000';
		this.draw.fillRect(0, 0, this.canvas.width, this.canvas.height);
	}

	private emit(type: string) {
		this.dispatchEvent(new Event(type));
	}

	private fail(e: unknown) {
		console.error('[car mode]', e);
		this.error = e instanceof Error ? e.message : String(e);
		this.emit('error');
	}
	error = '';

	private ensureAudio() {
		if (this.audio) return;
		this.audio = new AudioContext({ latencyHint: 'playback' });
		this.gain = this.audio.createGain();
		this.gain.connect(this.audio.destination);
		this.audio.suspend();
		this.clockAt = this.audio.currentTime;
		this.applyVolume();
	}

	private applyVolume() {
		if (this.gain) this.gain.gain.value = this._muted ? 0 : this._volume;
		this.emit('volumechange');
	}

	private async configure() {
		const v = this.videoTrack;
		const a = this.audioTrack;
		if (!v) throw new Error('The stream has no video');
		this.videoConfig = {
			codec: v.codec,
			description: v.description,
			codedWidth: v.width,
			codedHeight: v.height
		};
		if (!(await VideoDecoder.isConfigSupported(this.videoConfig)).supported)
			throw new Error(`This browser can’t decode ${v.codec} with WebCodecs`);
		this.audioConfig = a && {
			codec: a.codec,
			description: a.description,
			sampleRate: a.sampleRate,
			numberOfChannels: a.channels
		};
		// Without a usable audio decoder, play the picture silently.
		if (this.audioConfig && !(await AudioDecoder.isConfigSupported(this.audioConfig)).supported) {
			console.warn('[car mode] no WebCodecs decoder for', a?.codec, '- playing without sound');
			this.audioConfig = undefined;
		}
		this.videoDecoder = new VideoDecoder({
			output: (f) => this.onFrame(f),
			error: (e) => this.fail(e)
		});
		this.videoDecoder.configure(this.videoConfig);
		if (this.audioConfig) {
			this.audioDecoder = new AudioDecoder({
				output: (d) => this.onAudio(d),
				error: (e) => this.fail(e)
			});
			this.audioDecoder.configure(this.audioConfig);
		}
	}

	/** Drops everything buffered and decoding (seek, reset). */
	private flush() {
		for (const f of this.frames) f.close();
		this.frames = [];
		this.videoQueue = [];
		this.audioQueue = [];
		for (const s of this.sources) {
			s.onended = null;
			try {
				s.stop();
			} catch {
				// Not started.
			}
		}
		this.sources.clear();
		this.needKey = true;
		for (const [d, config] of [
			[this.videoDecoder, this.videoConfig],
			[this.audioDecoder, this.audioConfig]
		] as const) {
			if (d && d.state !== 'closed') {
				d.reset();
				d.configure(config as VideoDecoderConfig & AudioDecoderConfig);
			}
		}
	}

	private seekTo(t: number, announce = true) {
		this.generation++;
		this.abort.abort();
		this.abort = new AbortController();
		this.fetching = false;
		this.flush();
		const target = Math.max(0, Math.min(t, (this._duration || t) - 0.05));
		this._ended = false;
		this.seekTarget = target;
		this.syncClock();
		this.setClock(target);
		// The segment the playlist says covers `target`. Jellyfin cuts
		// copied video on keyframes, so real segments run a little off the
		// nominal times; samples before the target are skipped.
		let i = this.segments.findIndex((s) => s.start + s.duration > target);
		if (i < 0) i = this.segments.length - 1;
		if (i > 0 && this.segments[i].start > target - 1) i--;
		this.nextSegment = i;
		this.rangeStart = this.bufferedEnd = target;
		if (announce) this.emit('seeking');
		this.emit('timeupdate');
		this.pump();
	}

	/** Media time now (the picture's, so behind the audio by its output latency). */
	private clock(): number {
		if (!this.audio) return this.mediaAt;
		return this.mediaAt + Math.max(0, this.audio.currentTime - this.clockAt);
	}
	private setClock(t: number) {
		this.mediaAt = t;
		this.clockAt = this.audio?.currentTime ?? 0;
	}

	/** Runs the AudioContext only while time should move. */
	private syncClock() {
		const a = this.audio;
		if (!a) return;
		const run = !this._paused && !this.stalled && !this._ended && this.seekTarget === null;
		if (run && a.state !== 'running') {
			// Restart the clock from where it stood.
			this.setClock(this.clock());
			a.resume().catch(() => this.pause());
		} else if (!run && a.state === 'running') {
			this.setClock(this.clock());
			a.suspend();
		}
	}

	// Fetching and decoding.
	private pump() {
		if (!this.opened || !this.videoDecoder) return;
		const now = this.currentTime;
		if (!this.fetching && this.nextSegment < this.segments.length && this.bufferedEnd < now + AHEAD)
			void this.fetchNext();

		// Video: a few frames ahead. After a seek, decoding starts from the
		// last keyframe at or before the target; the frames between it and
		// the target are decoded (later ones depend on them) but dropped.
		const vd = this.videoDecoder;
		const target = this.seekTarget;
		if (target !== null) {
			const q = this.videoQueue;
			let key = -1;
			for (let i = 0; i < q.length && q[i].pts <= target; i++) if (q[i].key) key = i;
			if (key > 0) q.splice(0, key);
		}
		const depth = target === null ? 4 : 16;
		while (
			this.videoQueue.length &&
			vd.decodeQueueSize < depth &&
			this.frames.length < MAX_FRAMES
		) {
			const s = this.videoQueue.shift()!;
			if (this.needKey && !s.key) continue;
			this.needKey = false;
			vd.decode(
				new EncodedVideoChunk({
					type: s.key ? 'key' : 'delta',
					timestamp: Math.round(s.pts * 1e6),
					duration: Math.round(s.duration * 1e6),
					data: s.data
				})
			);
		}

		// Audio waits for the seek to land, so it's scheduled against the
		// clock the picture starts from.
		const ad = this.audioDecoder;
		if (ad && this.seekTarget === null) {
			while (this.audioQueue.length && this.audioQueue[0].pts < now + AUDIO_AHEAD) {
				const s = this.audioQueue.shift()!;
				if (s.pts + s.duration < now) continue;
				ad.decode(
					new EncodedAudioChunk({
						type: 'key',
						timestamp: Math.round(s.pts * 1e6),
						duration: Math.round(s.duration * 1e6),
						data: s.data
					})
				);
			}
		} else if (!ad) this.audioQueue = [];

		this.checkStall(now);
	}

	private async fetchNext() {
		const gen = this.generation;
		const index = this.nextSegment;
		this.fetching = true;
		try {
			const bytes = await fetchOk(this.segments[index].url, this.abort.signal, 'bytes');
			if (gen !== this.generation) return;
			const samples = parseSegment(bytes, this.tracks);
			const first = samples.find((s) => s.track === this.videoTrack?.id);
			// After a seek, a first segment that starts past the target
			// (Jellyfin cuts on keyframes) moves the target to its first frame.
			const firstAfterSeek = this.rangeStart === this.bufferedEnd;
			if (
				firstAfterSeek &&
				this.seekTarget !== null &&
				first &&
				first.pts > this.seekTarget + 0.05
			) {
				this.seekTarget = first.pts;
				this.setClock(first.pts);
				this.rangeStart = first.pts;
			}
			for (const s of samples) {
				if (s.track === this.videoTrack?.id) this.videoQueue.push(s);
				else if (s.track === this.audioTrack?.id) this.audioQueue.push(s);
				this.bufferedEnd = Math.max(this.bufferedEnd, s.pts + s.duration);
			}
			this.nextSegment = index + 1;
			if (this.nextSegment >= this.segments.length) this.bufferedEnd = this._duration;
			this.emit('progress');
		} catch (e) {
			if (gen === this.generation && !this.abort.signal.aborted) this.fail(e);
		} finally {
			if (gen === this.generation) {
				this.fetching = false;
				this.pump();
			}
		}
	}

	private onFrame(f: VideoFrame) {
		const t = this.seekTarget ?? this.clock();
		// Decoded only to reach the seek target.
		if (this.seekTarget !== null && (f.timestamp + (f.duration ?? 0)) / 1e6 <= t) {
			f.close();
			return;
		}
		let i = this.frames.length;
		while (i > 0 && this.frames[i - 1].timestamp > f.timestamp) i--;
		this.frames.splice(i, 0, f);
	}

	private onAudio(d: AudioData) {
		const ctx = this.audio;
		if (!ctx || !this.gain) return d.close();
		const buffer = ctx.createBuffer(d.numberOfChannels, d.numberOfFrames, d.sampleRate);
		for (let ch = 0; ch < d.numberOfChannels; ch++) {
			const plane = new Float32Array(d.numberOfFrames);
			d.copyTo(plane, { planeIndex: ch, format: 'f32-planar' });
			buffer.copyToChannel(plane, ch);
		}
		const start = d.timestamp / 1e6;
		d.close();
		// Where this buffer falls on the context's clock; a buffer that is
		// partly in the past starts part-way in.
		const when = this.clockAt + (start - this.mediaAt);
		const offset = Math.max(0, ctx.currentTime - when);
		if (offset >= buffer.duration) return;
		const src = ctx.createBufferSource();
		src.buffer = buffer;
		src.connect(this.gain);
		src.start(when + offset, offset);
		src.onended = () => this.sources.delete(src);
		this.sources.add(src);
	}

	// Buffering: hold the clock until there's media to play.
	private checkStall(now: number) {
		if (this.seekTarget !== null) return;
		const atEnd = this.nextSegment >= this.segments.length && !this.fetching;
		const starving = !atEnd && !this.frames.length && this.bufferedEnd < now + 0.25;
		if (starving && !this.stalled) {
			this.stalled = true;
			this.emit('waiting');
			this.syncClock();
		} else if (this.stalled && (atEnd || this.bufferedEnd > now + 2 || this.frames.length >= 2)) {
			this.stalled = false;
			this.syncClock();
		}
		if (
			atEnd &&
			!this._ended &&
			!this.videoQueue.length &&
			!this.frames.length &&
			(this.videoDecoder?.decodeQueueSize ?? 0) === 0 &&
			now >= this.bufferedEnd - 0.1
		) {
			this._ended = true;
			this.syncClock();
			this.emit('timeupdate');
			this.emit('ended');
		}
	}

	// Rendering.
	private frame = () => {
		this.raf = requestAnimationFrame(this.frame);
		this.tick();
	};

	/**
	 * Fetch, decode and show what's due. Runs every animation frame, and on
	 * a timer too, since a hidden page gets no animation frames but should
	 * keep its sound and finish seeks.
	 */
	private tick() {
		this.pump();
		const target = this.seekTarget;
		// A seek completes with the first frame at (or just after) the target.
		if (target !== null) {
			const f = this.frames[0];
			if (!f) return;
			this.seekTarget = null;
			this.setClock(Math.max(target, f.timestamp / 1e6));
			this.paint(this.frames.shift()!);
			this.audioQueue = this.audioQueue.filter((s) => s.pts + s.duration >= this.mediaAt);
			this.syncClock();
			this.emit('seeked');
			this.emit('timeupdate');
			return;
		}
		const now = this.clock() - (this.audio?.outputLatency || this.audio?.baseLatency || 0);
		let show: VideoFrame | undefined;
		while (this.frames.length && this.frames[0].timestamp / 1e6 <= now) {
			show?.close();
			show = this.frames.shift();
		}
		if (show) this.paint(show);
		const ms = performance.now();
		if (!this._paused && ms - this.lastTimeupdate > 250) {
			this.lastTimeupdate = ms;
			this.emit('timeupdate');
		}
	}

	private paint(f: VideoFrame) {
		const w = f.displayWidth;
		const h = f.displayHeight;
		if (this.canvas.width !== w || this.canvas.height !== h) {
			this.canvas.width = w;
			this.canvas.height = h;
		}
		this.draw.drawImage(f, 0, 0, w, h);
		f.close();
		if (!this.hasFrame) {
			this.hasFrame = true;
			this.emit('loadeddata');
		}
	}
}
