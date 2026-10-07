<script lang="ts">
	import { POSTER_STEPS, posterSize } from '#lib/ui.svelte.ts';

	// Plex Web's MetadataListStylesMenu slider: a 60px track with a 12px
	// thumb that sets the poster size for every list on the page.
	const WIDTH = 60;
	let track: HTMLDivElement | undefined = $state();
	let dragging = $state(false);

	function setFrom(clientX: number) {
		if (!track) return;
		const r = track.getBoundingClientRect();
		posterSize.step = ((clientX - r.left) / r.width) * POSTER_STEPS;
	}
	function down(e: PointerEvent) {
		dragging = true;
		(e.currentTarget as HTMLElement).setPointerCapture(e.pointerId);
		setFrom(e.clientX);
	}
	function move(e: PointerEvent) {
		if (dragging) setFrom(e.clientX);
	}
	function key(e: KeyboardEvent) {
		if (e.key === 'ArrowLeft' || e.key === 'ArrowDown') posterSize.step--;
		else if (e.key === 'ArrowRight' || e.key === 'ArrowUp') posterSize.step++;
		else return;
		e.preventDefault();
	}
</script>

<div
	class="slider"
	bind:this={track}
	style:width="{WIDTH}px"
	onpointerdown={down}
	onpointermove={move}
	onpointerup={() => (dragging = false)}
	role="presentation"
>
	<div class="track"></div>
	<button
		class="thumb"
		class:dragging
		type="button"
		role="slider"
		aria-label="Poster size"
		aria-valuemin={0}
		aria-valuemax={POSTER_STEPS}
		aria-valuenow={posterSize.step}
		style:left="{(posterSize.step / POSTER_STEPS) * WIDTH}px"
		onkeydown={key}
	></button>
</div>

<style>
	.slider {
		position: relative;
		height: 12px;
		cursor: pointer;
		touch-action: none;
	}
	/* Plex: Slider-track. */
	.track {
		position: absolute;
		top: 5px;
		left: 0;
		right: 0;
		height: 2px;
		background-color: rgba(255, 255, 255, 0.2);
	}
	/* Plex: Slider-thumb. */
	.thumb {
		position: absolute;
		top: 0;
		width: 12px;
		height: 12px;
		margin-left: -6px;
		border-radius: 50%;
		background-color: #ddd;
		box-shadow: 0 0 2px 0 rgba(0, 0, 0, 0.35);
		transition:
			left 0.1s,
			top 0.1s,
			opacity 0.2s,
			background-color 0.2s;
	}
	.thumb:hover,
	.thumb.dragging {
		background-color: #fff;
	}
	.thumb.dragging {
		transition: none;
	}
</style>
