<script lang="ts">
	import { clock } from '#lib/format.ts';
	import { player } from '#lib/player.svelte.ts';

	// Plex Web's SeekBar: a 4px track along the top edge of the bottom bar
	// (24px tall hit area), buffered and played fills, a thumb that appears
	// on hover, and a time tooltip with a trickplay thumbnail.
	let bar: HTMLDivElement | undefined = $state();
	let hoverX = $state<number | null>(null);
	let dragging = $state(false);

	const duration = $derived(player.duration || 0);
	const played = $derived(duration ? Math.min(1, player.currentTime / duration) : 0);
	const buffered = $derived(duration ? Math.min(1, player.buffered / duration) : 0);
	const width = $derived(bar?.clientWidth ?? 1);
	const hoverTime = $derived(hoverX == null ? 0 : (hoverX / width) * duration);
	// Plex: SeekBarThumbnail is 176px wide; its tooltip is "small" (60px)
	// below an hour and "large" (90px) from an hour on.
	const THUMB_WIDTH = 176;
	let lastX = $state(0);
	$effect(() => {
		if (hoverX != null) lastX = hoverX;
	});
	const previewTime = $derived((lastX / width) * duration);
	const thumb = $derived(player.isAudio ? null : player.trickplay(previewTime, THUMB_WIDTH));
	const large = $derived(duration >= 3600);

	function x(e: PointerEvent) {
		const r = bar!.getBoundingClientRect();
		return Math.max(0, Math.min(r.width, e.clientX - r.left));
	}
	function down(e: PointerEvent) {
		if (!duration) return;
		dragging = true;
		(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
		hoverX = x(e);
	}
	function move(e: PointerEvent) {
		hoverX = x(e);
	}
	function up(e: PointerEvent) {
		if (!dragging) return;
		dragging = false;
		player.seek((x(e) / width) * duration);
	}
	const shownPlayed = $derived(dragging && hoverX != null ? hoverX / width : played);
</script>

<div
	class="seek-bar"
	class:dragging
	bind:this={bar}
	onpointerdown={down}
	onpointermove={move}
	onpointerup={up}
	onpointerleave={() => !dragging && (hoverX = null)}
	role="slider"
	tabindex="-1"
	aria-label="Seek"
	aria-valuemin={0}
	aria-valuemax={Math.round(duration)}
	aria-valuenow={Math.round(player.currentTime)}
>
	<div class="track">
		<div class="fill buffer" style:transform="scaleX({buffered})"></div>
		<div
			class="fill played"
			class:no-transition={dragging}
			style:transform="scaleX({shownPlayed})"
		></div>
	</div>
	<div class="thumb" style:left="{shownPlayed * 100}%"></div>

	{#if duration}
		{@const w = thumb?.width ?? THUMB_WIDTH}
		{@const h = thumb?.height ?? 0}
		<div
			class="preview"
			class:is-hidden={hoverX == null}
			style:width="{w}px"
			style:height="{h}px"
			style:left="{Math.max(0, Math.min(width - w, lastX - w / 2))}px"
		>
			{#if thumb}
				<div
					class="thumbnail"
					style:width="{thumb.width}px"
					style:height="{thumb.height}px"
					style:background-image="url({thumb.image})"
					style:background-size={thumb.size}
					style:background-position={thumb.position}
				></div>
			{/if}
			<div class="tooltip" class:large style:left="{(w - (large ? 90 : 60)) / 2}px">
				{clock(previewTime)}
			</div>
		</div>
	{/if}
</div>

<style>
	/* Plex: SeekBar + Slider. */
	.seek-bar {
		position: absolute;
		top: -10px;
		left: 0;
		width: 100%;
		height: 24px;
		cursor: pointer;
		touch-action: none;
		user-select: none;
		z-index: 2;
	}
	/* Clicking focuses the bar; a key press then must not ring it. */
	.seek-bar:focus,
	.seek-bar:focus-visible {
		outline: none;
		box-shadow: none;
	}
	.track {
		position: absolute;
		top: 10px;
		left: 0;
		right: 0;
		height: 4px;
		overflow: hidden;
	}
	.track::before {
		content: '';
		position: absolute;
		inset: 0;
		background-color: rgba(0, 0, 0, 0.75);
		transition: background-color 0.6s 0.2s;
	}
	.seek-bar:hover .track::before {
		background-color: hsla(0, 0%, 100%, 0.15);
	}
	.fill {
		position: absolute;
		inset: 0;
		transform-origin: center left;
		transition: transform 0.1s;
	}
	.buffer {
		background-color: color-mix(in srgb, var(--color-accent-dark) 30%, transparent);
	}
	.played {
		background-color: var(--color-accent-dark);
	}
	.no-transition {
		transition: none;
	}
	.thumb {
		position: absolute;
		top: 6px;
		width: 12px;
		height: 12px;
		margin-left: -6px;
		border-radius: 50%;
		background-color: #ddd;
		box-shadow: 0 0 2px rgba(0, 0, 0, 0.35);
		opacity: 0;
		pointer-events: none;
		transition:
			left 0.1s,
			opacity 0.2s,
			background-color 0.2s;
	}
	.seek-bar:hover .thumb,
	.dragging .thumb {
		background-color: #fff;
		opacity: 1;
	}
	.dragging .thumb {
		transition: none;
	}

	/* Plex: SeekBarThumbnail. The container sits on the seek bar's top edge;
	   it fades in with motionEasing after 0.2s and out without the delay. */
	.preview {
		position: absolute;
		bottom: 24px;
		z-index: 1115;
		pointer-events: none;
		opacity: 1;
		transition: opacity 0.2s cubic-bezier(0.6, 0.4, 0.2, 1.4) 0.2s;
	}
	.preview.is-hidden {
		opacity: 0;
		transition-delay: 0s;
	}
	.thumbnail {
		background-color: rgba(0, 0, 0, 0.45);
		background-repeat: no-repeat;
		box-shadow: 0 0 4px 0 rgba(0, 0, 0, 0.45);
	}
	.tooltip {
		position: absolute;
		bottom: 4px;
		width: 60px;
		height: 23px;
		border-radius: 4px;
		background-color: #000;
		box-shadow: 0 0 4px 0 rgba(0, 0, 0, 0.5);
		color: #fff;
		font-size: 13px;
		line-height: 23px;
		text-align: center;
	}
	.tooltip.large {
		width: 90px;
	}
</style>
