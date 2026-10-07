<script lang="ts">
	import raw from '#lib/assets/wordmark.svg?raw';

	// The jellex wordmark, inlined so its "x" gradient follows the theme
	// (Settings > Theme sets --wordmark-from / --wordmark-to).
	let { height }: { height: number } = $props();
	const svg = raw
		.replace('stop-color="#AA5CC3"', 'style="stop-color: var(--wordmark-from, #aa5cc3)"')
		.replace('stop-color="#00A4DC"', 'style="stop-color: var(--wordmark-to, #00a4dc)"')
		.replace(/ width="[\d.]+" height="[\d.]+"/, '');
</script>

<span class="wordmark" style:height="{height}px" aria-hidden="true">{@html svg}</span>

<style>
	.wordmark {
		display: inline-flex;
	}
	.wordmark :global(svg) {
		height: 100%;
		width: auto;
	}
</style>
