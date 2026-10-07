// Playback: what to play for an item, and how this browser can play it.
import type {
	BaseItemDto,
	DeviceProfile,
	MediaSourceInfo,
	PlaybackInfoResponse
} from '@jellyfin/sdk/lib/generated-client';
import { getLibraryApi } from '@jellyfin/sdk/lib/utils/api/library-api';
import { getMediaInfoApi } from '@jellyfin/sdk/lib/utils/api/media-info-api';
import { getShowApi } from '@jellyfin/sdk/lib/utils/api/show-api';
import { session } from './session.svelte.ts';

const PLAYABLE = ['Movie', 'Episode', 'Video', 'MusicVideo', 'Audio', 'Trailer'];

/**
 * The queue Play starts for an item: itself if playable, otherwise its
 * contents in order (a series from its next episode, a season from its
 * first unwatched one, an album's tracks).
 */
export async function resolveQueue(item: BaseItemDto, shuffle = false): Promise<BaseItemDto[]> {
	const api = session.requireApi;
	const userId = session.userId;
	const library = getLibraryApi(api);
	const shows = getShowApi(api);
	const type = item.Type ?? '';

	// A whole library (Plex's Play / Shuffle on a library): its movies,
	// episodes or tracks, in title order or shuffled, up to a few hundred.
	if (type === 'CollectionFolder' || type === 'UserView' || shuffle) {
		const kinds =
			item.CollectionType === 'tvshows' || type === 'Series' || type === 'Season'
				? (['Episode'] as const)
				: item.CollectionType === 'music' || type === 'MusicAlbum' || type === 'MusicArtist'
					? (['Audio'] as const)
					: (['Movie', 'Episode', 'Video', 'MusicVideo'] as const);
		const owner = type === 'MusicArtist' ? { albumArtistIds: [item.Id!] } : { parentId: item.Id! };
		const { data } = await library.getItems({
			userId,
			...owner,
			recursive: true,
			includeItemTypes: [...kinds],
			sortBy: shuffle ? ['Random'] : ['SortName'],
			limit: 300
		});
		if (data.Items?.length || !PLAYABLE.includes(type)) return data.Items ?? [];
	}

	if (type === 'Episode') {
		// The episode, then the rest of the series after it.
		const { data } = await shows.getEpisodes({
			seriesId: item.SeriesId!,
			userId,
			startItemId: item.Id!
		});
		const rest = data.Items ?? [];
		return rest.length ? rest : [item];
	}
	if (PLAYABLE.includes(type)) return [item];

	if (type === 'Series' || type === 'Season') {
		const seriesId = type === 'Series' ? item.Id! : item.SeriesId!;
		let start: BaseItemDto | undefined;
		if (type === 'Series') {
			start = (await shows.getNextUp({ seriesId, userId, limit: 1 })).data.Items?.[0];
		} else {
			const eps =
				(await shows.getEpisodes({ seriesId, seasonId: item.Id!, userId })).data.Items ?? [];
			start = eps.find((e) => !e.UserData?.Played) ?? eps[0];
		}
		const { data } = await shows.getEpisodes({
			seriesId,
			userId,
			startItemId: start?.Id ?? undefined
		});
		return data.Items ?? [];
	}

	const params =
		type === 'MusicArtist'
			? {
					albumArtistIds: [item.Id!],
					includeItemTypes: ['Audio' as const],
					sortBy: ['Album' as const, 'ParentIndexNumber' as const, 'IndexNumber' as const]
				}
			: type === 'MusicAlbum'
				? {
						parentId: item.Id!,
						includeItemTypes: ['Audio' as const],
						sortBy: ['ParentIndexNumber' as const, 'IndexNumber' as const, 'SortName' as const]
					}
				: {
						parentId: item.Id!,
						includeItemTypes: [
							'Movie' as const,
							'Episode' as const,
							'Video' as const,
							'Audio' as const,
							'MusicVideo' as const
						]
					};
	const { data } = await library.getItems({ userId, recursive: true, ...params });
	return data.Items ?? [];
}

/** What this browser plays natively, and how Jellyfin should transcode the rest. */
function deviceProfile(): DeviceProfile {
	const v = document.createElement('video');
	const can = (t: string) => v.canPlayType(t) !== '';
	const videoCodecs = ['h264'];
	if (can('video/mp4; codecs="hvc1.1.6.L93.B0"') || can('video/mp4; codecs="hev1.1.6.L93.B0"'))
		videoCodecs.push('hevc');
	if (can('video/mp4; codecs="av01.0.05M.08"')) videoCodecs.push('av1');
	if (can('video/mp4; codecs="vp09.00.10.08"')) videoCodecs.push('vp9');
	const audioCodecs = ['aac', 'mp3'];
	if (can('audio/mp4; codecs="opus"') || can('audio/webm; codecs="opus"')) audioCodecs.push('opus');
	if (can('audio/flac')) audioCodecs.push('flac');
	if (can('audio/mp4; codecs="ac-3"')) audioCodecs.push('ac3');
	if (can('audio/mp4; codecs="ec-3"')) audioCodecs.push('eac3');

	return {
		MaxStreamingBitrate: 120_000_000,
		MaxStaticBitrate: 100_000_000,
		MusicStreamingTranscodingBitrate: 384_000,
		DirectPlayProfiles: [
			{
				Container: 'mp4,m4v,mov',
				Type: 'Video',
				VideoCodec: videoCodecs.join(','),
				AudioCodec: audioCodecs.join(',')
			},
			{ Container: 'webm', Type: 'Video', VideoCodec: 'vp8,vp9,av1', AudioCodec: 'vorbis,opus' },
			{ Container: 'mp3', Type: 'Audio' },
			{ Container: 'aac,m4a,m4b', Type: 'Audio', AudioCodec: 'aac' },
			{ Container: 'flac', Type: 'Audio' },
			{ Container: 'webm,ogg,oga', Type: 'Audio', AudioCodec: 'opus,vorbis' },
			{ Container: 'wav', Type: 'Audio' }
		],
		TranscodingProfiles: [
			{
				Container: 'ts',
				Type: 'Video',
				VideoCodec: 'h264',
				AudioCodec: 'aac,mp3',
				Context: 'Streaming',
				Protocol: 'hls',
				MaxAudioChannels: '2',
				MinSegments: 1,
				BreakOnNonKeyFrames: true
			},
			{
				Container: 'mp3',
				Type: 'Audio',
				AudioCodec: 'mp3',
				Context: 'Streaming',
				Protocol: 'http',
				MaxAudioChannels: '2'
			}
		],
		ContainerProfiles: [],
		CodecProfiles: [],
		SubtitleProfiles: [
			{ Format: 'vtt', Method: 'External' },
			{ Format: 'srt', Method: 'External' },
			{ Format: 'subrip', Method: 'External' },
			{ Format: 'ass', Method: 'External' },
			{ Format: 'ssa', Method: 'External' },
			{ Format: 'pgssub', Method: 'Encode' },
			{ Format: 'dvdsub', Method: 'Encode' },
			{ Format: 'dvbsub', Method: 'Encode' }
		]
	};
}

export interface Stream {
	info: PlaybackInfoResponse;
	source: MediaSourceInfo;
	/** URL for the media element, or an HLS playlist. */
	url: string;
	hls: boolean;
	method: 'DirectPlay' | 'DirectStream' | 'Transcode';
	/** A WebVTT URL for the chosen subtitles, when delivered as a track. */
	subtitleUrl?: string;
}

export async function getStream(
	item: BaseItemDto,
	opts: { startTicks: number; audio?: number; subtitle?: number; maxBitrate?: number }
): Promise<Stream> {
	const api = session.requireApi;
	const { data: info } = await getMediaInfoApi(api).getPostedPlaybackInfo({
		itemId: item.Id!,
		userId: session.userId,
		startTimeTicks: opts.startTicks,
		audioStreamIndex: opts.audio,
		subtitleStreamIndex: opts.subtitle,
		autoOpenLiveStream: true,
		maxStreamingBitrate: opts.maxBitrate,
		// A bitrate cap means Plex's "Convert to …": always transcode.
		enableDirectPlay: !opts.maxBitrate,
		enableDirectStream: !opts.maxBitrate,
		enableTranscoding: true,
		allowVideoStreamCopy: true,
		allowAudioStreamCopy: true,
		playbackInfoDto: {
			DeviceProfile: opts.maxBitrate
				? { ...deviceProfile(), MaxStreamingBitrate: opts.maxBitrate }
				: deviceProfile()
		}
	});
	const source = info.MediaSources?.[0];
	if (!source) throw new Error(info.ErrorCode ?? 'Nothing to play');

	const base = api.basePath;
	const token = api.accessToken;
	const audioOnly = item.MediaType === 'Audio';
	let url: string;
	let method: Stream['method'];
	let hls = false;
	if (source.SupportsDirectPlay) {
		const q = new URLSearchParams({ static: 'true', mediaSourceId: source.Id!, api_key: token });
		if (info.PlaySessionId) q.set('PlaySessionId', info.PlaySessionId);
		if (source.ETag) q.set('Tag', source.ETag);
		url = `${base}/${audioOnly ? 'Audio' : 'Videos'}/${item.Id}/stream?${q}`;
		method = 'DirectPlay';
	} else if (source.TranscodingUrl) {
		url = base + source.TranscodingUrl;
		if (!/[?&](api_key|ApiKey)=/i.test(url)) url += `&api_key=${token}`;
		hls = source.TranscodingSubProtocol === 'hls';
		method = source.SupportsDirectStream ? 'DirectStream' : 'Transcode';
	} else {
		throw new Error('This item can’t be played in this browser');
	}

	const sub = source.MediaStreams?.find((s) => s.Type === 'Subtitle' && s.Index === opts.subtitle);
	const subtitleUrl =
		sub?.DeliveryMethod === 'External' && sub.DeliveryUrl
			? base +
				sub.DeliveryUrl.replace(/\.(srt|subrip|ass|ssa)(\?|$)/i, '.vtt$2') +
				(sub.DeliveryUrl.includes('api_key')
					? ''
					: `${sub.DeliveryUrl.includes('?') ? '&' : '?'}api_key=${token}`)
			: undefined;

	return { info, source, url, hls, method, subtitleUrl };
}
