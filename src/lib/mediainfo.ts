// Plex Web's Media Info ("Get Info") data. Plex Web builds the modal from
// the server's Media/Part/Stream attributes: it walks them in the order PMS
// sends them, skips an ignore list, renames a few, formats some values and
// appends units (Plex Web 4.160, `tt`/`rt`/`nt` in the main bundle). This
// is a port of that code, fed with PMS-shaped objects built from
// Jellyfin's media source the same way the old jellex PMS emulation did,
// with attributes in the order a real PMS sends them for a local item
// (ref/…/MediaProviderResponses/shazam_local_item.json).
import type { BaseItemDto, MediaSourceInfo, MediaStream } from '@jellyfin/sdk/lib/generated-client';

type Attrs = [string, unknown][];
export interface Props {
	properties: { name: string; value: string }[];
}
export interface StreamProps extends Props {
	streamType: number;
}
export interface PartProps extends Props {
	file: string;
	Stream: StreamProps[];
}
export interface MediaProps extends Props {
	Part: PartProps[];
}

// Plex Web: Ye (ignored attributes).
const IGNORED = new Set([
	'id',
	'key',
	'index',
	'videoCodec',
	'audioCodec',
	'audioChannels',
	'default',
	'selected',
	'languageCode',
	'accessible',
	'exists',
	'provider',
	'lines',
	'minLines',
	'follow',
	'requiredBandwidths',
	'deepAnalysisVersion',
	'deepAnalysisDate',
	'Overlay'
]);

// Plex Web: 25886 (size), 28249 (duration), 57990 (upper case),
// 60071 (resolution), 90999 (yes/no), getLastSegment (file).
const SIZE_UNITS = ['B', 'KB', 'MB', 'GB', 'TB'];
function size(e: number) {
	if (e === 0) return `${(0).toFixed(2)} ${SIZE_UNITS[0]}`;
	const r = Math.floor(Math.log(e) / Math.log(1024));
	return `${(e / Math.pow(1024, r)).toFixed(2)} ${SIZE_UNITS[r]}`;
}
function durationMs(e: number) {
	if (!(e > 0)) return '0:00';
	const total = Math.floor(e / 1000);
	const s = total % 60;
	const m = Math.floor(total / 60) % 60;
	const h = Math.floor(total / 3600);
	let o = s < 10 ? ':0' + s : ':' + s;
	o = m < 10 && h > 0 ? '0' + m + o : m + o;
	if (h > 0) o = h + ':' + o;
	return o;
}
const upper = (e: unknown) => (e ? String(e).toUpperCase() : '');
const resolution = (e: unknown) =>
	e ? (/^\d+$/.test(String(e)) ? e + 'p' : String(e).toUpperCase()) : '';
const yesNo = (e: unknown) => (e && String(e) !== 'false' && String(e) !== '0' ? 'Yes' : 'No');
const lastSegment = (e: unknown) =>
	String(e ?? '')
		.split(/[\\/]/)
		.pop() ?? '';

// Plex Web: Ze (formatters), Xe (renames), et (suffixes), qe (level).
const FORMAT: Record<string, (v: never) => string> = {
	size: size as (v: never) => string,
	duration: durationMs as (v: never) => string,
	codec: upper,
	format: upper,
	container: upper,
	videoResolution: resolution,
	bitrateMode: upper,
	accessible: yesNo,
	exists: yesNo,
	optimizedForStreaming: yesNo,
	timed: yesNo,
	file: lastSegment
};
const RENAME: Record<string, string> = {
	optimizedForStreaming: 'Web Optimized',
	has64bitOffsets: 'Has 64bit Offsets',
	cabac: 'CABAC',
	postURL: 'URL',
	codecID: 'Codec ID'
};
const SUFFIX: Record<string, string> = {
	frameRate: ' fps',
	dialogNorm: ' dB',
	bitrate: ' kbps',
	samplingRate: ' Hz'
};
const LEVEL_DIVISOR: Record<string, number> = { h264: 10, hevc: 30 };
function level(e: unknown, codec: unknown) {
	if (e == null || Number.isNaN(e)) return '';
	const r = parseInt(String(e), 10);
	const n = LEVEL_DIVISOR[String(codec || 'h264')];
	return Number.isFinite(r) && Number.isFinite(n) ? (r / n).toFixed(1) : String(e);
}
/** Plex Web: $e ("samplingRate" → "Sampling Rate"). */
function humanize(key: string) {
	let t = '';
	for (let i = 0; i < key.length; i++) {
		const c = key.charAt(i);
		const u = c.toUpperCase();
		t += i === 0 ? u : c === u ? ' ' + u : c;
	}
	return t;
}

/** Plex Web: tt (`child` is the key it keeps aside, e.g. "streamType"). */
function toProps(attrs: Attrs, child?: string): Props {
	const obj = Object.fromEntries(attrs);
	const properties: Props['properties'] = [];
	for (const [key, raw] of attrs) {
		if (key === child || IGNORED.has(key)) continue;
		let value: unknown = raw;
		if (key === 'level') value = level(value, obj.codec);
		if (FORMAT[key]) value = FORMAT[key](value as never);
		if (!value) continue;
		let text = String(value);
		if (SUFFIX[key]) text += SUFFIX[key];
		properties.push({ name: RENAME[key] ?? humanize(key), value: text });
	}
	return { properties };
}

// Jellyfin → PMS-shaped attributes (as the old jellex emulation sent them).
const SUBTITLE_CODECS: Record<string, string> = {
	subrip: 'srt',
	srt: 'srt',
	webvtt: 'vtt',
	vtt: 'vtt',
	ass: 'ass',
	ssa: 'ssa',
	mov_text: 'mov_text',
	pgssub: 'pgs',
	hdmv_pgs_subtitle: 'pgs',
	dvdsub: 'vobsub',
	dvd_subtitle: 'vobsub'
};
function videoResolution(w: number, h: number) {
	if (w >= 3200 || h >= 1800) return '4k';
	if (w >= 1700 || h >= 1000) return '1080';
	if (w >= 1200 || h >= 700) return '720';
	if (h >= 540) return '576';
	if (h >= 400) return '480';
	return 'sd';
}
function videoFrameRate(f: number) {
	if (!f) return '';
	if (f < 24.5) return '24p';
	if (f < 26) return 'PAL';
	if (f < 31) return 'NTSC';
	if (f < 51) return '50p';
	return '60p';
}
const round2 = (f: number) => Math.round(f * 100) / 100;
const kbps = (b: number | null | undefined) => (b ? Math.floor(b / 1000) : undefined);
const lower = (s: string | null | undefined) => s?.toLowerCase() || undefined;
// "und" (undetermined) has no language; PMS leaves the attributes out.
const known = (code: string | null | undefined) => (code && code !== 'und' ? code : undefined);
function languageName(code: string | null | undefined) {
	code = known(code);
	if (!code) return undefined;
	try {
		const tag = Intl.getCanonicalLocales(code)[0];
		return new Intl.DisplayNames(['en'], { type: 'language' }).of(tag);
	} catch {
		return undefined;
	}
}
function languageTag(code: string | null | undefined) {
	code = known(code);
	if (!code) return undefined;
	try {
		return Intl.getCanonicalLocales(code)[0];
	} catch {
		return undefined;
	}
}

function streamAttrs(st: MediaStream): StreamProps | null {
	const type = st.Type === 'Video' ? 1 : st.Type === 'Audio' ? 2 : st.Type === 'Subtitle' ? 3 : 0;
	if (!type) return null;
	const language = [
		['language', languageName(st.Language)],
		['languageTag', languageTag(st.Language)],
		['languageCode', st.Language]
	] as Attrs;
	const tail = [
		['title', st.Title],
		['displayTitle', st.DisplayTitle],
		['extendedDisplayTitle', st.DisplayTitle]
	] as Attrs;
	let attrs: Attrs;
	if (type === 1) {
		attrs = [
			['id', st.Index],
			['streamType', 1],
			['default', st.IsDefault],
			['codec', st.Codec],
			['index', st.Index],
			['bitrate', kbps(st.BitRate)],
			...language,
			['bitDepth', st.BitDepth],
			['colorPrimaries', st.ColorPrimaries],
			['colorSpace', st.ColorSpace],
			['colorTrc', st.ColorTransfer],
			['frameRate', st.RealFrameRate ? round2(st.RealFrameRate) : undefined],
			['height', st.Height],
			['level', st.Level || undefined],
			['profile', lower(st.Profile)],
			['refFrames', st.RefFrames],
			['title', st.Title],
			['width', st.Width],
			['displayTitle', st.DisplayTitle],
			['extendedDisplayTitle', st.DisplayTitle]
		];
	} else if (type === 2) {
		attrs = [
			['id', st.Index],
			['streamType', 2],
			['default', st.IsDefault],
			['codec', st.Codec],
			['index', st.Index],
			['channels', st.Channels],
			['bitrate', kbps(st.BitRate)],
			...language,
			['audioChannelLayout', st.ChannelLayout],
			['bitDepth', st.BitDepth],
			['profile', lower(st.Profile)],
			['samplingRate', st.SampleRate],
			...tail
		];
	} else {
		const codec = st.Codec?.toLowerCase() ?? '';
		attrs = [
			['id', st.Index],
			['streamType', 3],
			['default', st.IsDefault],
			['forced', st.IsForced],
			['codec', SUBTITLE_CODECS[codec] ?? codec],
			['index', st.IsExternal ? undefined : st.Index],
			['bitrate', kbps(st.BitRate)],
			...language,
			...tail
		];
	}
	return { ...toProps(attrs, 'streamType'), streamType: type };
}

/** Plex Web: nt, for one Jellyfin media source. */
export function mediaInfo(
	item: BaseItemDto,
	source: MediaSourceInfo,
	hasTrickplay: boolean
): MediaProps {
	const streams = source.MediaStreams ?? [];
	const video = streams.find((s) => s.Type === 'Video');
	const audio = streams.find((s) => s.Type === 'Audio');
	const container = source.Container?.split(',')[0];
	const w = video?.Width ?? 0;
	const h = video?.Height ?? 0;
	const duration = source.RunTimeTicks ? Math.floor(source.RunTimeTicks / 10_000) : undefined;
	const media: Attrs = [
		['id', source.Id],
		['duration', duration],
		['bitrate', kbps(source.Bitrate)],
		...(video
			? ([
					['width', w || undefined],
					['height', h || undefined],
					['aspectRatio', h ? round2(w / h) : undefined]
				] as Attrs)
			: []),
		['audioChannels', audio?.Channels],
		['audioCodec', audio?.Codec],
		['videoCodec', video?.Codec],
		...(video ? ([['videoResolution', videoResolution(w, h)]] as Attrs) : []),
		['container', container],
		...(video
			? ([
					['videoFrameRate', videoFrameRate(video.RealFrameRate ?? 0)],
					['videoProfile', lower(video.Profile)]
				] as Attrs)
			: [])
	];
	const part: Attrs = [
		['accessible', true],
		['exists', true],
		['id', source.Id],
		['key', source.Id],
		['duration', duration],
		['file', source.Path],
		['size', source.Size],
		['container', container],
		['indexes', hasTrickplay ? 'sd' : undefined],
		...(video ? ([['videoProfile', lower(video.Profile)]] as Attrs) : [])
	];
	// Plex Web: rt — streams sorted by type (stable).
	const streamProps = streams
		.map(streamAttrs)
		.filter((s): s is StreamProps => !!s)
		.sort((a, b) => a.streamType - b.streamType);
	return {
		...toProps(media),
		Part: [{ ...toProps(part), file: source.Path ?? '', Stream: streamProps }]
	};
}
