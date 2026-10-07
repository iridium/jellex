<script lang="ts">
	import { icons, type IconName } from '#lib/icons.ts';

	// Plex's icons are drawn on a 48-unit grid and sized by font-size, the way
	// Plex Web does it: a 24px icon sits in a container with font-size 24px.
	let {
		name,
		size = '1em',
		label
	}: { name: IconName; size?: string | number; label?: string } = $props();

	const icon = $derived(icons[name]);
	const px = $derived(typeof size === 'number' ? `${size}px` : size);
</script>

<svg
	viewBox={icon.viewBox}
	width={px}
	height={px}
	fill="currentColor"
	role={label ? 'img' : undefined}
	aria-label={label}
	aria-hidden={label ? undefined : 'true'}
>
	<!-- eslint-disable-next-line svelte/no-at-html-tags -- trusted, static icon data -->
	{@html icon.body}
</svg>

<style>
	svg {
		flex-shrink: 0;
		display: block;
	}
</style>
