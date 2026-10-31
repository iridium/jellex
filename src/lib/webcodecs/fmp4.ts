// A minimal fragmented-MP4 demuxer for Jellyfin's fMP4 HLS segments: the
// init segment gives each track's codec, timescale and decoder description;
// media segments give samples (moof/traf/trun over mdat).

export interface Track {
	id: number;
	kind: 'video' | 'audio';
	timescale: number;
	/** WebCodecs codec string. */
	codec: string;
	/** avcC / hvcC / av1C payload, or the AAC AudioSpecificConfig. */
	description?: Uint8Array;
	width: number;
	height: number;
	channels: number;
	sampleRate: number;
	// trex defaults.
	defaultDuration: number;
	defaultSize: number;
	defaultFlags: number;
}

export interface Sample {
	track: number;
	data: Uint8Array;
	/** Seconds. */
	pts: number;
	duration: number;
	key: boolean;
}

interface Box {
	type: string;
	/** Payload start and box end, as offsets into the buffer. */
	start: number;
	end: number;
	boxStart: number;
}

function* boxes(b: Uint8Array, start = 0, end = b.length): Generator<Box> {
	const v = new DataView(b.buffer, b.byteOffset, b.byteLength);
	let off = start;
	while (off + 8 <= end) {
		let size = v.getUint32(off);
		const type = String.fromCharCode(b[off + 4], b[off + 5], b[off + 6], b[off + 7]);
		let header = 8;
		if (size === 1) {
			size = Number(v.getBigUint64(off + 8));
			header = 16;
		} else if (size === 0) size = end - off;
		if (size < header) return;
		yield { type, start: off + header, end: Math.min(off + size, end), boxStart: off };
		off += size;
	}
}

function child(b: Uint8Array, parent: Box, type: string): Box | undefined {
	for (const c of boxes(b, parent.start, parent.end)) if (c.type === type) return c;
}

function path(b: Uint8Array, parent: Box, ...types: string[]): Box | undefined {
	let cur: Box | undefined = parent;
	for (const t of types) cur = cur && child(b, cur, t);
	return cur;
}

const hex = (n: number) => n.toString(16).padStart(2, '0');

function avcCodec(avcC: Uint8Array) {
	return `avc1.${hex(avcC[1])}${hex(avcC[2])}${hex(avcC[3])}`;
}

function hevcCodec(type: string, c: Uint8Array) {
	const space = ['', 'A', 'B', 'C'][c[1] >> 6];
	const tier = c[1] & 0x20 ? 'H' : 'L';
	const profile = c[1] & 0x1f;
	// The compatibility flags are written bit-reversed.
	let flags = ((c[2] << 24) | (c[3] << 16) | (c[4] << 8) | c[5]) >>> 0;
	let reversed = 0;
	for (let i = 0; i < 32; i++) {
		reversed = (reversed << 1) | (flags & 1);
		flags >>>= 1;
	}
	const constraints = Array.from(c.subarray(6, 12));
	while (constraints.length && constraints.at(-1) === 0) constraints.pop();
	return [
		type,
		`${space}${profile}`,
		(reversed >>> 0).toString(16),
		`${tier}${c[12]}`,
		...constraints.map((x) => x.toString(16))
	].join('.');
}

function av1Codec(c: Uint8Array) {
	const profile = c[1] >> 5;
	const level = c[1] & 0x1f;
	const tier = c[2] & 0x80 ? 'H' : 'M';
	const high = (c[2] >> 6) & 1;
	const twelve = (c[2] >> 5) & 1;
	const depth = profile === 2 && high ? (twelve ? 12 : 10) : high ? 10 : 8;
	return `av01.${profile}.${String(level).padStart(2, '0')}${tier}.${String(depth).padStart(2, '0')}`;
}

/** esds → [objectTypeIndication, DecoderSpecificInfo]. */
function parseEsds(b: Uint8Array, box: Box): { oti: number; asc?: Uint8Array } {
	let off = box.start + 4;
	let oti = 0;
	let asc: Uint8Array | undefined;
	const readLen = () => {
		let len = 0;
		for (let i = 0; i < 4; i++) {
			const x = b[off++];
			len = (len << 7) | (x & 0x7f);
			if (!(x & 0x80)) break;
		}
		return len;
	};
	while (off < box.end) {
		const tag = b[off++];
		const len = readLen();
		if (tag === 3) {
			const flags = b[off + 2];
			off += 3;
			if (flags & 0x80) off += 2;
			if (flags & 0x40) off += 1 + b[off];
			if (flags & 0x20) off += 2;
		} else if (tag === 4) {
			oti = b[off];
			off += 13;
		} else if (tag === 5) {
			asc = b.slice(off, off + len);
			off += len;
		} else off += len;
	}
	return { oti, asc };
}

export function parseInit(b: Uint8Array): Track[] {
	const v = new DataView(b.buffer, b.byteOffset, b.byteLength);
	const moov = [...boxes(b)].find((x) => x.type === 'moov');
	if (!moov) throw new Error('No moov in the init segment');
	const tracks: Track[] = [];
	for (const trak of boxes(b, moov.start, moov.end)) {
		if (trak.type !== 'trak') continue;
		const tkhd = child(b, trak, 'tkhd');
		const mdhd = path(b, trak, 'mdia', 'mdhd');
		const hdlr = path(b, trak, 'mdia', 'hdlr');
		const stsd = path(b, trak, 'mdia', 'minf', 'stbl', 'stsd');
		if (!tkhd || !mdhd || !hdlr || !stsd) continue;
		const id = v.getUint32(tkhd.start + (b[tkhd.start] === 1 ? 20 : 12));
		const timescale = v.getUint32(mdhd.start + (b[mdhd.start] === 1 ? 20 : 12));
		const handler = String.fromCharCode(...b.subarray(hdlr.start + 8, hdlr.start + 12));
		const entry = boxes(b, stsd.start + 8, stsd.end).next().value;
		if (!entry) continue;
		const base = {
			id,
			timescale,
			width: 0,
			height: 0,
			channels: 0,
			sampleRate: 0,
			defaultDuration: 0,
			defaultSize: 0,
			defaultFlags: 0
		};
		if (handler === 'vide') {
			const width = v.getUint16(entry.start + 24);
			const height = v.getUint16(entry.start + 26);
			const inner = { ...entry, start: entry.start + 78 };
			let codec = '';
			let description: Uint8Array | undefined;
			const avcC = child(b, inner, 'avcC');
			const hvcC = child(b, inner, 'hvcC');
			const av1C = child(b, inner, 'av1C');
			if (avcC) {
				description = b.slice(avcC.start, avcC.end);
				codec = avcCodec(description);
			} else if (hvcC) {
				description = b.slice(hvcC.start, hvcC.end);
				codec = hevcCodec(entry.type === 'hev1' ? 'hev1' : 'hvc1', description);
			} else if (av1C) {
				description = b.slice(av1C.start, av1C.end);
				codec = av1Codec(description);
			} else continue;
			tracks.push({ ...base, kind: 'video', codec, description, width, height });
		} else if (handler === 'soun') {
			const channels = v.getUint16(entry.start + 16);
			const sampleRate = v.getUint32(entry.start + 24) >>> 16;
			const inner = { ...entry, start: entry.start + 28 };
			if (entry.type === 'mp4a') {
				const esds = child(b, inner, 'esds');
				const { oti, asc } = esds ? parseEsds(b, esds) : { oti: 0x40 };
				if (oti === 0x6b || oti === 0x69) {
					tracks.push({ ...base, kind: 'audio', codec: 'mp3', channels, sampleRate });
				} else {
					const aot = asc ? asc[0] >> 3 : 2;
					tracks.push({
						...base,
						kind: 'audio',
						codec: `mp4a.40.${aot}`,
						description: asc,
						channels,
						sampleRate
					});
				}
			} else if (entry.type === 'Opus') {
				tracks.push({ ...base, kind: 'audio', codec: 'opus', channels, sampleRate: 48000 });
			}
		}
	}
	const mvex = child(b, moov, 'mvex');
	if (mvex)
		for (const trex of boxes(b, mvex.start, mvex.end)) {
			if (trex.type !== 'trex') continue;
			const t = tracks.find((x) => x.id === v.getUint32(trex.start + 4));
			if (!t) continue;
			t.defaultDuration = v.getUint32(trex.start + 12);
			t.defaultSize = v.getUint32(trex.start + 16);
			t.defaultFlags = v.getUint32(trex.start + 20);
		}
	return tracks;
}

/** The samples of one media segment (any number of moof/mdat pairs). */
export function parseSegment(b: Uint8Array, tracks: Track[]): Sample[] {
	const v = new DataView(b.buffer, b.byteOffset, b.byteLength);
	const out: Sample[] = [];
	for (const moof of boxes(b)) {
		if (moof.type !== 'moof') continue;
		for (const traf of boxes(b, moof.start, moof.end)) {
			if (traf.type !== 'traf') continue;
			const tfhd = child(b, traf, 'tfhd');
			if (!tfhd) continue;
			const tf = v.getUint32(tfhd.start) & 0xffffff;
			const track = tracks.find((t) => t.id === v.getUint32(tfhd.start + 4));
			if (!track) continue;
			let o = tfhd.start + 8;
			let baseOffset = moof.boxStart;
			if (tf & 0x1) {
				baseOffset = Number(v.getBigUint64(o));
				o += 8;
			}
			if (tf & 0x2) o += 4;
			let defDuration = track.defaultDuration;
			let defSize = track.defaultSize;
			let defFlags = track.defaultFlags;
			if (tf & 0x8) ((defDuration = v.getUint32(o)), (o += 4));
			if (tf & 0x10) ((defSize = v.getUint32(o)), (o += 4));
			if (tf & 0x20) defFlags = v.getUint32(o);

			const tfdt = child(b, traf, 'tfdt');
			let dts = 0;
			if (tfdt)
				dts =
					b[tfdt.start] === 1
						? Number(v.getBigUint64(tfdt.start + 4))
						: v.getUint32(tfdt.start + 4);

			let dataEnd = baseOffset;
			for (const trun of boxes(b, traf.start, traf.end)) {
				if (trun.type !== 'trun') continue;
				const version = b[trun.start];
				const rf = v.getUint32(trun.start) & 0xffffff;
				const count = v.getUint32(trun.start + 4);
				let p = trun.start + 8;
				let dataOffset = dataEnd;
				if (rf & 0x1) ((dataOffset = baseOffset + v.getInt32(p)), (p += 4));
				let firstFlags: number | undefined;
				if (rf & 0x4) ((firstFlags = v.getUint32(p)), (p += 4));
				for (let i = 0; i < count; i++) {
					let duration = defDuration;
					let size = defSize;
					let flags = i === 0 && firstFlags !== undefined ? firstFlags : defFlags;
					let cts = 0;
					if (rf & 0x100) ((duration = v.getUint32(p)), (p += 4));
					if (rf & 0x200) ((size = v.getUint32(p)), (p += 4));
					if (rf & 0x400) {
						const f = v.getUint32(p);
						p += 4;
						if (!(i === 0 && firstFlags !== undefined)) flags = f;
					}
					if (rf & 0x800) ((cts = version ? v.getInt32(p) : v.getUint32(p)), (p += 4));
					out.push({
						track: track.id,
						data: b.subarray(dataOffset, dataOffset + size),
						pts: (dts + cts) / track.timescale,
						duration: duration / track.timescale,
						// sample_is_non_sync_sample; audio samples are all sync.
						key: track.kind === 'audio' || !((flags >> 16) & 1)
					});
					dataOffset += size;
					dts += duration;
				}
				dataEnd = dataOffset;
			}
		}
	}
	return out;
}
