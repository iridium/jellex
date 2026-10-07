<script lang="ts">
	import { getLibraryApi } from '@jellyfin/sdk/lib/utils/api/library-api';
	import { clock, episodeCode } from '#lib/format.ts';
	import { player } from '#lib/player.svelte.ts';
	import { session } from '#lib/session.svelte.ts';
	import Icon from '../Icon.svelte';
	import Menu from '../Menu.svelte';
	import PlayerButton from './PlayerButton.svelte';
	import StarRating from '../StarRating.svelte';
	import SeekBar from './SeekBar.svelte';
	import type { Snippet } from 'svelte';

	// Plex Web's player BottomBar: seek bar on the top edge, then
	// PlayerControls: metadata (left), transport (centre), tools (right) and
	// the vertical volume slider. The same bar serves the full and mini
	// players; the mini player passes its poster in.
	let {
		poster,
		mini = false,
		onInfo
	}: { poster?: Snippet; mini?: boolean; onInfo: () => void } = $props();

	const item = $derived(player.current);
	const audio = $derived(player.isAudio);
	let showRemaining = $state(false);

	const title = $derived(item?.Type === 'Episode' ? (item.SeriesName ?? item.Name) : item?.Name);
	const titleHref = $derived(
		item?.Type === 'Episode' && item.SeriesId
			? `/items/${item.SeriesId}`
			: item?.Type === 'Audio'
				? undefined
				: `/items/${item?.Id}`
	);
	const time = $derived(
		showRemaining
			? `${clock(player.currentTime)} / -${clock(Math.max(0, player.duration - player.currentTime))}`
			: `${clock(player.currentTime)} / ${clock(player.duration)}`
	);
	const hasChapters = $derived(!audio && (player.item?.Chapters?.length ?? 0) > 1);

	// Vertical volume slider.
	let volumeTrack: HTMLDivElement | undefined = $state();
	let draggingVolume = $state(false);
	function volumeFrom(e: PointerEvent) {
		const r = volumeTrack!.getBoundingClientRect();
		player.setVolume(1 - (e.clientY - r.top) / r.height);
	}

	function download() {
		const api = session.requireApi;
		window.open(
			getLibraryApi(api).getDownloadUrl({ itemId: player.current!.Id! }),
			'_blank',
			'noopener'
		);
	}
	function go() {
		if (player.mode === 'full') player.minimize();
	}
</script>

<div class="bottom-bar">
	<div class="controls-container">
		<SeekBar />
		<div class="controls">
			<div class="group left">
				{#if mini && poster}{@render poster()}{/if}
				{#if item}
					<div class="metadata">
						{#if titleHref}
							<a class="title" href={titleHref} onclick={go}>{title}</a>
						{:else}
							<div class="title">{title}</div>
						{/if}
						<div class="title secondary">
							{#if item.Type === 'Episode'}
								<a href="/items/{item.Id}" onclick={go}
									>{[episodeCode(item), item.Name].filter(Boolean).join(' — ')}</a
								>
							{:else if audio}
								{#if item.AlbumArtists?.[0]?.Id}
									<a href="/items/{item.AlbumArtists[0].Id}" onclick={go}
										>{item.AlbumArtists[0].Name}</a
									>
								{:else}{item.AlbumArtist ?? ''}{/if}{#if item.Album}{' — '}{#if item.AlbumId}<a
											href="/items/{item.AlbumId}"
											onclick={go}>{item.Album}</a
										>{:else}{item.Album}{/if}{/if}
							{:else}
								{item.ProductionYear ?? ''}
							{/if}
						</div>
						<!-- Plex: the time, then (music only) the track's rating, which
						     shows on hover or once the track has one. -->
						<div class="third-line">
							<button
								class="duration"
								type="button"
								onclick={() => (showRemaining = !showRemaining)}>{time}</button
							>{#if audio}<span class="rating" class:rated={!!item.UserData?.Rating}
									><StarRating {item} /></span
								>{/if}
						</div>
					</div>
				{/if}
			</div>

			<!-- Plex keeps the transport centred in the mini player too. -->
			<div class="group center balance">
				<PlayerButton
					icon="previous"
					label="Previous"
					disabled={!player.hasPrevious && player.currentTime < 5}
					onclick={() => player.previous()}
				/>
				{#if !audio}
					<PlayerButton
						icon="skip-back-10"
						label="Skip Back 10 Seconds"
						onclick={() => player.skip(-10)}
					/>
				{/if}
				<PlayerButton
					icon={player.paused ? 'play-48' : 'pause'}
					label={player.paused ? 'Play' : 'Pause'}
					primary
					onclick={() => player.toggle()}
				/>
				{#if !audio}
					<PlayerButton
						icon="skip-forward-30"
						label="Skip Forward 30 Seconds"
						onclick={() => player.skip(30)}
					/>
				{/if}
				<PlayerButton
					icon="next"
					label="Next"
					disabled={!player.hasNext}
					onclick={() => player.next()}
				/>
				<PlayerButton icon="stop" label="Close Player" onclick={() => player.close()} />
			</div>

			<div class="group right">
				<span class="more">
					<Menu
						variant="plain"
						align="right"
						placement="above"
						items={[
							{ label: 'Save File', onselect: download },
							{ label: 'Get Info', onselect: onInfo }
						]}
					>
						{#snippet trigger()}<Icon
								name="more-vertical"
								size={14}
								label="More Actions"
							/>{/snippet}
					</Menu>
				</span>
				<PlayerButton
					icon={player.repeat === 'one' ? 'repeat-one' : 'repeat'}
					label={player.repeat === 'one' ? 'Repeat One' : 'Repeat'}
					active={player.repeat !== 'off'}
					onclick={() => player.cycleRepeat()}
				/>
				<PlayerButton
					icon="shuffle"
					label="Shuffle"
					active={player.shuffle}
					onclick={() => player.toggleShuffle()}
				/>
				{#if hasChapters}
					<PlayerButton
						icon="chapters"
						label="Chapters"
						active={player.panel === 'chapters'}
						onclick={() => player.togglePanel('chapters')}
					/>
				{/if}
				<PlayerButton
					icon="filters"
					label="Settings"
					active={player.panel === 'settings'}
					onclick={() => player.togglePanel('settings')}
				/>
				<PlayerButton
					icon="play-queue"
					label="Play Queue"
					active={player.panel === 'queue'}
					onclick={() => player.togglePanel('queue')}
				/>
				<span class="volume-button">
					<PlayerButton
						icon={player.muted || player.volume === 0 ? 'volume-muted' : 'volume'}
						label={player.muted ? 'Unmute Volume' : 'Mute Volume'}
						onclick={() => player.toggleMute()}
					/>
				</span>
			</div>
		</div>

		<!-- Plex: VolumeSlider (vertical). -->
		<div
			class="volume-slider"
			class:dragging={draggingVolume}
			role="slider"
			tabindex="-1"
			aria-label="Volume"
			aria-valuemin={0}
			aria-valuemax={100}
			aria-valuenow={Math.round((player.muted ? 0 : player.volume) * 100)}
			onpointerdown={(e) => {
				draggingVolume = true;
				(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
				volumeFrom(e);
			}}
			onpointermove={(e) => draggingVolume && volumeFrom(e)}
			onpointerup={() => (draggingVolume = false)}
		>
			<div class="volume-track" bind:this={volumeTrack}>
				<div class="volume-fill" style:transform="scaleY({player.muted ? 0 : player.volume})"></div>
			</div>
			<div
				class="volume-thumb"
				style:bottom="calc({(player.muted ? 0 : player.volume) * 100}% - 6px)"
			></div>
		</div>
	</div>
</div>

<style>
	/* Clicking focuses the slider; a key press then must not ring it. */
	.volume-slider:focus,
	.volume-slider:focus-visible {
		outline: none;
		box-shadow: none;
	}
	/* Plex: BottomBar + ControlsContainer. */
	.bottom-bar {
		position: relative;
		background-color: rgba(35, 36, 38, 0.8);
		box-shadow: 0 0 4px 0 rgba(0, 0, 0, 0.5);
	}
	.controls-container {
		position: relative;
		display: flex;
		flex-direction: column;
		justify-content: center;
		height: 100px;
		min-width: 640px;
		padding-top: 4px;
		font-size: 13px;
		line-height: 1.71428571;
		color: #eee;
	}
	/* Plex: PlayerControls. */
	.controls {
		display: flex;
		align-items: center;
		justify-content: space-between;
	}
	.group {
		display: flex;
		align-items: center;
	}
	.left {
		flex: 1 0 0;
		min-width: 0;
	}
	.center {
		flex: 0 1 0;
		justify-content: center;
		margin: 0 20px;
	}
	.center.balance {
		padding-left: 35px;
	}
	.right {
		flex: 1 0 0;
		justify-content: flex-end;
		padding-right: 20px;
	}

	/* Plex: PlayerControlsMetadata + MetadataPosterTitle. */
	.metadata {
		display: flex;
		flex-direction: column;
		min-width: 0;
		margin-left: 20px;
		overflow: hidden;
		white-space: nowrap;
	}
	.title {
		display: block;
		height: 20px;
		overflow: hidden;
		text-overflow: ellipsis;
		color: #fff;
		line-height: 20px;
		user-select: none;
	}
	a.title:hover,
	.secondary a:hover {
		text-decoration: underline;
	}
	.secondary {
		color: hsla(0, 0%, 100%, 0.45);
	}
	.third-line {
		display: block;
		white-space: nowrap;
	}
	/* Plex: PlayerControlsMetadata-ratingContainer. */
	.rating {
		display: inline;
		opacity: 0;
		pointer-events: none;
		transition: opacity 0.2s ease;
	}
	.metadata:hover .rating,
	.rating.rated {
		opacity: 1;
		pointer-events: auto;
	}
	/* Plex: DurationRemaining. */
	.duration {
		display: inline-block;
		margin-right: 20px;
		overflow: hidden;
		text-overflow: ellipsis;
		color: hsla(0, 0%, 100%, 0.45);
		line-height: 1.71428571;
		transition: color 0.2s;
	}
	.duration:hover {
		color: #fff;
	}

	.more {
		display: flex;
	}
	.more :global(.trigger) {
		justify-content: center;
		width: 30px;
		height: 30px;
		border-radius: 15px;
		color: hsla(0, 0%, 100%, 0.7);
	}
	.more :global(.trigger:hover) {
		color: #fff;
	}
	.volume-button {
		display: flex;
		margin-right: 12px;
	}

	/* Plex: PlayerControls-volumeSlider (VerticalSlider). */
	.volume-slider {
		position: absolute;
		right: 5px;
		bottom: 18px;
		width: 32px;
		height: 60px;
		cursor: pointer;
		touch-action: none;
	}
	.volume-track {
		position: absolute;
		top: 0;
		bottom: 0;
		left: 15px;
		width: 2px;
		overflow: hidden;
		background-color: hsla(0, 0%, 7%, 0.75);
	}
	.volume-fill {
		position: absolute;
		inset: 0;
		background-color: var(--color-brand-accent);
		transform-origin: center bottom;
		transition: transform 0.1s;
	}
	.volume-thumb {
		position: absolute;
		left: 10px;
		width: 12px;
		height: 12px;
		border-radius: 50%;
		background-color: #ddd;
		box-shadow: 0 0 2px rgba(0, 0, 0, 0.35);
		opacity: 0;
		pointer-events: none;
		transition:
			bottom 0.1s,
			opacity 0.2s,
			background-color 0.2s;
	}
	.volume-slider:hover .volume-thumb,
	.volume-slider.dragging .volume-thumb {
		background-color: #fff;
		opacity: 1;
	}
	.dragging .volume-fill,
	.dragging .volume-thumb {
		transition: none;
	}
</style>
