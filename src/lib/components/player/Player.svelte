<script lang="ts">
	import { settings } from '#lib/settings.svelte.ts';
	import { fadeIn } from '#lib/motion.ts';
	import { imageUrl } from '#lib/images.ts';
	import { player } from '#lib/player.svelte.ts';
	import { colorsFrom } from '#lib/ultrablur.ts';
	import Icon from '../Icon.svelte';
	import Spinner from '../Spinner.svelte';
	import Chapters from './Chapters.svelte';
	import ControlsBar from './ControlsBar.svelte';
	import MediaInfo from './MediaInfo.svelte';
	import PlaybackSettings from './PlaybackSettings.svelte';
	import PlayerButton from './PlayerButton.svelte';
	import PlayQueue from './PlayQueue.svelte';
	import ResumePrompt from './ResumePrompt.svelte';

	// Plex Web's Player: AudioVideoFullPlayer (full screen, bars that hide
	// when idle) and the mini player (the same bottom bar under the page,
	// with the video or cover as its poster). One media element serves
	// both, so playback carries on while browsing.
	let root: HTMLDivElement | undefined = $state();
	let media: HTMLVideoElement | undefined = $state();
	let idle = $state(false);
	let overBars = $state(false);
	let showInfo = $state(false);
	let skipButton: HTMLButtonElement | undefined = $state();
	let idleTimer: ReturnType<typeof setTimeout> | undefined;

	const SUBTITLE_KEY = 'jellex.subtitleSize';
	let subtitleSize = $state(
		Number(
			(() => {
				try {
					return localStorage.getItem(SUBTITLE_KEY);
				} catch {
					return null;
				}
			})()
		) || 1
	);
	$effect(() => {
		try {
			localStorage.setItem(SUBTITLE_KEY, String(subtitleSize));
		} catch {
			// Not persisted.
		}
	});

	const full = $derived(player.mode === 'full');
	// Plex's fullPlayerContainer background: #232426 until video frames show
	// (or the mini bar's rgba(0,0,0,.3) when expanding), then transparent,
	// with its 0.2s background transition.
	let fromMini = $state(false);
	let lastMode = player.mode;
	$effect.pre(() => {
		const m = player.mode;
		if (m === 'full' && lastMode !== 'full') fromMini = lastMode === 'mini';
		lastMode = m;
	});
	let settled = $state(false);
	$effect(() => {
		if (!full) return void (settled = false);
		const raf = requestAnimationFrame(() => (settled = true));
		return () => cancelAnimationFrame(raf);
	});
	const mini = $derived(player.mode === 'mini');
	const audio = $derived(player.isAudio);
	const item = $derived(player.current);
	const marker = $derived(player.activeMarker);
	// Plex: AudioVideoFullMusic sizes the square cover to fit the area
	// between the bars: min(500, width - 80) when the area is taller than
	// wide, else min(500, height - 170), leaving room for the titles.
	let musicW = $state(0);
	let musicH = $state(0);
	const coverSize = (w: number, h: number) =>
		Math.max(0, w < h ? Math.min(500, w - 80) : Math.min(500, h - 170));
	const art = $derived(item && audio ? imageUrl(item, 'square', 500) : undefined);
	const tint = $derived(item && audio ? colorsFrom(item) : null);
	const barsHidden = $derived(
		full && !audio && idle && !player.paused && player.panel === 'none' && !overBars
	);

	$effect(() => {
		if (media) player.attach(media);
	});

	// The mini player takes 100px from the bottom of the app.
	$effect(() => {
		document.documentElement.style.setProperty('--mini-player-height', mini ? '100px' : '0px');
	});

	// Plex focuses the skip button when it appears, ready for Enter.
	$effect(() => {
		if (marker && full) skipButton?.focus({ preventScroll: true });
	});

	function poke() {
		idle = false;
		clearTimeout(idleTimer);
		// Plex hides the controls after 2s without pointer movement.
		idleTimer = setTimeout(() => (idle = true), 2000);
	}

	// Plex also hides them as soon as the pointer leaves the window (e.g.
	// off the screen's edge in fullscreen); moving back in shows them.
	$effect(() => {
		const leave = () => {
			clearTimeout(idleTimer);
			idle = true;
			overBars = false;
		};
		document.documentElement.addEventListener('mouseleave', leave);
		return () => document.documentElement.removeEventListener('mouseleave', leave);
	});

	function toggleFullscreen() {
		if (document.fullscreenElement) document.exitFullscreen().catch(() => {});
		else root?.requestFullscreen().catch(() => {});
	}

	function onKey(e: KeyboardEvent) {
		if (player.mode === 'closed' || player.resumePrompt || showInfo) return;
		const t = e.target as HTMLElement;
		if (t.closest('input, textarea, select, [contenteditable]')) return;
		const fullOnly = ['ArrowUp', 'ArrowDown', 'f', 'Escape'];
		if (!full && fullOnly.includes(e.key)) return;
		switch (e.key) {
			case ' ':
			case 'k':
				// Space is play/pause anywhere in the player (not "press the
				// focused button"); on the page around the mini player, a
				// focused link or button keeps its own Space.
				if (e.key === ' ' && t.closest('button, a') && !t.closest('.player')) return;
				e.preventDefault();
				// A key press would make the clicked control show its focus
				// ring; Plex shows none, so let go of focus in the player.
				if (t.closest('.player')) (document.activeElement as HTMLElement | null)?.blur();
				player.toggle();
				break;
			case 'ArrowLeft':
				player.skip(-10);
				break;
			case 'ArrowRight':
				player.skip(30);
				break;
			case 'ArrowUp':
				player.setVolume(player.volume + 0.1);
				break;
			case 'ArrowDown':
				player.setVolume(player.volume - 0.1);
				break;
			case 'm':
				player.toggleMute();
				break;
			case 'f':
				toggleFullscreen();
				break;
			case 'Escape':
				if (document.fullscreenElement) return;
				if (player.panel !== 'none') player.panel = 'none';
				else player.minimize();
				break;
			default:
				return;
		}
		e.preventDefault();
		if (full) poke();
	}

	// Settings > Player > Subtitle Position: top, middle or bottom lines.
	function placeCues() {
		const track = Array.from(media?.textTracks ?? []).find((t) => t.mode === 'showing');
		const cues = track?.cues;
		if (!cues) return;
		const where = settings.value.subtitlePosition;
		for (const cue of Array.from(cues) as VTTCue[]) {
			if (where === 'top') {
				cue.snapToLines = true;
				cue.line = 0;
			} else if (where === 'middle') {
				cue.snapToLines = false;
				cue.line = 50;
				cue.lineAlign = 'center';
			} else {
				cue.snapToLines = true;
				cue.line = 'auto';
			}
		}
	}
	$effect(() => {
		settings.value.subtitlePosition;
		placeCues();
	});
</script>

<svelte:window onkeydown={onKey} />
<svelte:document onfullscreenchange={() => (player.fullscreen = !!document.fullscreenElement)} />

<div
	class="player"
	class:closed={player.mode === 'closed'}
	class:full
	class:mini
	class:audio
	class:cursor-hidden={barsHidden}
	bind:this={root}
	style:--subtitle-scale={subtitleSize}
	onpointermove={() => full && poke()}
	role="presentation"
>
	{#if full}
		<!-- Plex: Player-fullPlayerContainer. -->
		<div
			class="full-container"
			class:from-mini={fromMini && !settled}
			class:clear={!audio && player.hasFrames && (settled || !fromMini)}
			style:--color-ultrablur-tl={tint?.tl}
			style:--color-ultrablur-tr={tint?.tr}
			style:--color-ultrablur-br={tint?.br}
			style:--color-ultrablur-bl={tint?.bl}
		>
			{#if audio || player.panel === 'queue'}
				<div class="ultrablur local"></div>
			{/if}

			{#if audio && player.panel !== 'queue' && item}
				<!-- Plex's music view: the cover, then title and artist. -->
				<div class="music" bind:clientWidth={musicW} bind:clientHeight={musicH}>
					{#if musicW && musicH}
						{@const size = coverSize(musicW, musicH)}
						<div class="music-content">
							<div class="cover" style:width="{size}px" style:height="{size}px">
								{#if art}<img src={art} alt="" use:fadeIn />{/if}
							</div>
							<div class="music-titles">
								<div class="music-title">{item.Name}</div>
								<div class="music-title secondary">
									{#if item.AlbumArtists?.[0]?.Id}
										<a href="/items/{item.AlbumArtists[0].Id}" onclick={() => player.minimize()}
											>{item.AlbumArtists[0].Name}</a
										>
									{:else}{item.AlbumArtist ?? ''}{/if}{#if item.Album}<span class="dash">—</span
										>{#if item.AlbumId}<a
												href="/items/{item.AlbumId}"
												onclick={() => player.minimize()}>{item.Album}</a
											>{:else}{item.Album}{/if}{/if}
								</div>
							</div>
						</div>
					{/if}
				</div>
			{/if}

			{#if !audio && player.panel !== 'queue'}
				<!-- Plex: PlayPauseOverlay. -->
				<div
					class="click-target"
					role="presentation"
					onclick={() => player.toggle()}
					ondblclick={toggleFullscreen}
					onmousedown={(e) => e.detail > 1 && e.preventDefault()}
				></div>
			{/if}

			{#if player.panel === 'queue'}
				<div class="content"><PlayQueue /></div>
			{/if}

			{#if player.loading}
				<div class="spinner"><Spinner size="large" /></div>
			{:else if player.error}
				<div class="spinner"><p class="error">{player.error}</p></div>
			{/if}

			{#if marker && player.panel === 'none'}
				<!-- Plex: AudioVideoFullPlayer-overlayButton. -->
				<button
					class="skip"
					type="button"
					bind:this={skipButton}
					onclick={() => player.skipMarker(marker)}
				>
					{marker.type === 'intro' ? 'Skip Intro' : 'Skip Credits'}
				</button>
			{/if}

			<!-- Plex: FullPlayerTopControls. -->
			<div
				class="top-bar"
				class:is-hidden={barsHidden}
				class:clear={audio || player.panel === 'queue'}
			>
				<PlayerButton icon="minimize" label="Minimize Player" onclick={() => player.minimize()} />
				<PlayerButton
					icon={player.fullscreen ? 'fullscreen-exit' : 'fullscreen'}
					label={player.fullscreen ? 'Exit Fullscreen' : 'Enter Fullscreen'}
					onclick={toggleFullscreen}
				/>
			</div>

			{#if player.panel === 'settings'}
				<PlaybackSettings bind:subtitleSize />
			{:else if player.panel === 'chapters'}
				<Chapters />
			{/if}
		</div>
	{/if}

	<video
		bind:this={media}
		style:--subtitle-scale={settings.value.subtitleSize / 100}
		style:--subtitle-color={settings.value.subtitleColor}
		class="media"
		playsinline
		crossorigin="anonymous"
		onclick={() => mini && player.expand()}
	>
		<!-- A new track per stream: the old one is disabled on teardown. -->
		{#key player.stream}
			{#if player.stream?.subtitleUrl}
				<track
					kind="subtitles"
					src={player.stream.subtitleUrl}
					default
					label="Subtitles"
					onload={placeCues}
				/>
			{/if}
		{/key}
	</video>

	{#if player.mode !== 'closed'}
		<div
			class="bar-wrap"
			class:is-hidden={barsHidden}
			role="presentation"
			onpointerenter={() => (overBars = true)}
			onpointerleave={() => (overBars = false)}
		>
			<ControlsBar {mini} onInfo={() => (showInfo = true)}>
				{#snippet poster()}
					<!-- Plex: MiniPlayerPoster. The video itself sits over this box. -->
					<div class="mini-poster" class:square={audio}>
						{#if audio && item}
							{@const cover = imageUrl(item, 'square', 160)}
							{#if cover}<img src={cover} alt="" use:fadeIn />{/if}
						{/if}
					</div>
				{/snippet}
			</ControlsBar>
		</div>
	{/if}

	{#if mini}
		<!-- Over the poster (and the video playing in it). -->
		<button
			class="expand"
			class:square={audio}
			type="button"
			aria-label="Expand Player"
			onclick={() => player.expand()}
		>
			<Icon name="chevron-up" size={18} />
		</button>
	{/if}

	{#if player.resumePrompt}
		<ResumePrompt />
	{/if}
	{#if showInfo}
		<MediaInfo onclose={() => (showInfo = false)} />
	{/if}
</div>

<style>
	/* Double-clicking the video toggles fullscreen; nothing in the player
	   should get selected (highlighted) by it. */
	.player {
		user-select: none;
		-webkit-user-select: none;
		-webkit-tap-highlight-color: transparent;
	}
	.player .media:focus,
	.player .click-target:focus {
		outline: none;
	}
	.player.closed .media {
		display: none;
	}
	.cursor-hidden,
	.cursor-hidden * {
		cursor: none !important;
	}

	/* Plex: Player-fullPlayerContainer (z-index 1013). */
	.full-container {
		position: fixed;
		inset: 0;
		z-index: 1013;
		overflow: hidden;
		background-color: #232426;
		transition: background 0.2s;
	}
	.full-container.from-mini {
		background-color: rgba(0, 0, 0, 0.3);
	}
	/* Video plays underneath this layer once it has frames. */
	.full-container.clear {
		background-color: transparent;
	}
	.ultrablur.local {
		position: absolute;
		z-index: 0;
	}
	.click-target {
		position: absolute;
		inset: 0;
		z-index: 1;
		cursor: pointer;
	}
	.content {
		position: absolute;
		top: 60px;
		left: 0;
		right: 0;
		bottom: 100px;
		z-index: 2;
	}
	.spinner {
		position: absolute;
		inset: 0;
		z-index: 2;
		display: grid;
		place-items: center;
		pointer-events: none;
	}
	.error {
		max-width: 480px;
		padding: 12px 16px;
		border-radius: 4px;
		background: #2d2d2d;
		color: var(--color-text-alert);
		text-align: center;
	}

	/* Plex: AudioVideoFullPlayer-content + AudioVideoFullMusic. */
	.music {
		position: absolute;
		top: 60px;
		left: 0;
		right: 0;
		bottom: 100px;
		z-index: 1;
	}
	.music-content {
		position: absolute;
		top: 50%;
		left: 50%;
		max-width: 80%;
		transform: translate(-50%, -50%);
		transition: transform 0.2s;
	}
	.cover {
		margin: 0 auto;
		border-radius: 4px;
		background-color: rgba(0, 0, 0, 0.45);
		box-shadow: 0 0 4px rgba(0, 0, 0, 0.3);
		overflow: hidden;
	}
	.cover img {
		width: 100%;
		height: 100%;
		object-fit: cover;
	}
	.music-titles {
		margin-top: 40px;
		text-align: center;
	}
	/* Plex: MetadataPosterTitle, single line. */
	.music-title {
		display: block;
		height: 30px;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		color: #fff;
		font-size: 24px;
		line-height: 20px;
		user-select: none;
	}
	.music-title.secondary {
		height: 20px;
		font-size: 15px;
	}
	.music-title a {
		color: #fff;
	}
	.music-title a:hover {
		text-decoration: underline;
	}
	/* Plex: DashSeparator. */
	.dash {
		margin: 0 4px;
	}

	/* Plex: AudioVideoFullPlayer-topBar. */
	.top-bar {
		position: absolute;
		top: 0;
		left: 0;
		right: 0;
		z-index: 3;
		display: flex;
		align-items: center;
		justify-content: space-between;
		height: 60px;
		padding: 0 10px;
		background-color: rgba(35, 36, 38, 0.45);
		transition:
			transform 0.2s,
			background 0.2s;
	}
	.top-bar.clear {
		background-color: rgba(35, 36, 38, 0);
	}
	.top-bar.is-hidden {
		transform: translateY(-60px);
	}

	/* Skip Intro / Skip Credits. */
	/* Plex: AudioVideoFullPlayer-overlayButton — fixed 120px up, shown and
	   hidden without animation, whether or not the controls are showing. */
	.skip {
		position: absolute;
		right: 20px;
		bottom: 120px;
		z-index: 3;
		padding: 8px 30px;
		border-radius: 4px;
		background-color: #666;
		box-shadow: 0 0 4px rgba(0, 0, 0, 0.45);
		color: #fff;
		font-size: 16px;
		font-weight: 600;
		line-height: 24px;
		transition:
			background-color 0.2s,
			color 0.2s;
	}
	.skip:hover {
		background-color: #707070;
	}
	.skip:focus-visible {
		box-shadow: inset 0 0 0 2px var(--color-keyboard-focus);
	}

	/* The media element: full screen, the mini player's poster, or hidden. */
	.media {
		position: fixed;
		background: #000;
		object-fit: contain;
	}
	.player.full .media {
		inset: 0;
		z-index: 1012;
		width: 100%;
		height: 100%;
		pointer-events: none;
	}
	.player.mini .media {
		left: 8px;
		bottom: 8px;
		z-index: 1015;
		width: 142px;
		height: 80px;
		object-fit: cover;
		cursor: pointer;
		box-shadow: 0 0 4px 0 rgba(0, 0, 0, 0.3);
	}
	.player.audio .media {
		width: 0;
		height: 0;
		opacity: 0;
	}
	/* Settings > Player: subtitle size and colour for text subtitles. */
	.media::cue {
		color: var(--subtitle-color, #fff);
		font-size: calc(var(--subtitle-scale, 1) * 100%);
	}

	/* The bottom bar: inside the full player, or the mini player. */
	.bar-wrap {
		position: fixed;
		left: 0;
		right: 0;
		bottom: 0;
		z-index: 1014;
		transition: transform 0.2s;
	}
	/* Plex: Player-miniPlayerContainer. */
	.player.mini .bar-wrap {
		background-color: rgba(0, 0, 0, 0.3);
	}
	.bar-wrap.is-hidden {
		transform: translateY(100%);
	}
	.player.full .bar-wrap {
		z-index: 1016;
	}

	/* Plex: MiniPlayerPoster. */
	.mini-poster {
		position: relative;
		flex-shrink: 0;
		width: 142px;
		height: 80px;
		margin-left: 8px;
		box-shadow: 0 0 4px 0 rgba(0, 0, 0, 0.3);
	}
	.mini-poster.square {
		width: 80px;
		background-color: rgba(0, 0, 0, 0.45);
	}
	.mini-poster img {
		width: 100%;
		height: 100%;
		object-fit: cover;
	}
	.expand {
		position: fixed;
		left: 8px;
		bottom: 8px;
		width: 142px;
		height: 80px;
		z-index: 1016;
		display: grid;
		place-items: center;
		background-color: rgba(0, 0, 0, 0.75);
		color: #fff;
		opacity: 0;
		transition: opacity 0.2s;
	}
	.expand.square {
		width: 80px;
	}
	.expand:hover,
	.expand:focus-visible {
		opacity: 1;
	}
</style>
