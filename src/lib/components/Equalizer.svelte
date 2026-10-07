<script lang="ts">
	// Plex Web's EqualizerIcon, ported: three bars (barWidth = floor((width -
	// gap * (count - 1)) / count)) whose scaleY transitions to a random value
	// in [0.2, 1) over a random 300–500ms, re-rolled 100ms before each
	// transition ends. Paused, the bars rest at scaleY(0.2).
	let {
		playing = true,
		width = 14,
		height = 14,
		barCount = 3,
		barGap = 2,
		barColor = 'var(--color-accent-light)'
	}: {
		playing?: boolean;
		width?: number;
		height?: number;
		barCount?: number;
		barGap?: number;
		barColor?: string;
	} = $props();

	const barWidth = $derived(Math.floor((width - barGap * (barCount - 1)) / barCount));
	const rand = (a: number, b: number) => a + Math.random() * (b - a);
	interface Bar {
		scale: number;
		duration: number;
	}
	const style = (paused: boolean): Bar => ({
		scale: paused ? 0.2 : rand(0.2, 1),
		duration: rand(300, 500)
	});
	let bars = $state<Bar[]>([]);

	$effect(() => {
		const paused = !playing;
		const n = barCount;
		// Only writes to `bars`: reading it here would make the effect depend
		// on what it writes and re-run itself.
		const next = Array.from({ length: n }, () => style(paused));
		bars = next;
		if (paused) return;
		const timers: ReturnType<typeof setTimeout>[] = [];
		// Plex re-rolls each bar 100ms before its transition ends.
		const schedule = (i: number, duration: number) => {
			timers[i] = setTimeout(() => {
				const bar = style(false);
				bars[i] = bar;
				schedule(i, bar.duration);
			}, duration - 100);
		};
		for (let i = 0; i < n; i++) schedule(i, next[i].duration);
		return () => timers.forEach(clearTimeout);
	});
</script>

<div
	class="equalizer"
	style:width="{width}px"
	style:height="{height}px"
	role="img"
	aria-label="Now playing"
>
	{#each bars as bar, i (i)}
		<div
			class="bar"
			style:left="{i * (barWidth + barGap)}px"
			style:width="{barWidth}px"
			style:background-color={barColor}
			style:transition-duration="{bar.duration}ms"
			style:transform="scaleY({bar.scale})"
		></div>
	{/each}
</div>

<style>
	/* Plex: EqualizerIcon. */
	.equalizer {
		position: relative;
	}
	.bar {
		position: absolute;
		height: 100%;
		transform-origin: bottom center;
		transition-property: transform;
	}
</style>
