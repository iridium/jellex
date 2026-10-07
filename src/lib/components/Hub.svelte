<script lang="ts">
	import type { Snippet } from 'svelte';
	import { smoothScroll } from '#lib/motion.ts';
	import Icon from './Icon.svelte';

	// Plex Web's VirtualHubScroller: a title row with page arrows on the
	// right, then one row of cards (passed as children) that scrolls a page
	// of whole cards at a time.
	let { title, href, children }: { title: string; href?: string; children: Snippet } = $props();

	const GAP = 30;
	let scroller: HTMLDivElement | undefined = $state();
	let atStart = $state(true);
	let atEnd = $state(true);

	function update() {
		if (!scroller) return;
		atStart = scroller.scrollLeft <= 1;
		atEnd = scroller.scrollLeft + scroller.clientWidth >= scroller.scrollWidth - 1;
	}

	function page(dir: 1 | -1) {
		if (!scroller) return;
		const card = (scroller.firstElementChild as HTMLElement | null)?.offsetWidth ?? 165;
		const step = card + GAP;
		const gutter = parseFloat(getComputedStyle(scroller).paddingLeft) || 40;
		const visible = Math.max(1, Math.floor((scroller.clientWidth - 2 * gutter + GAP) / step));
		// Plex's Scroller: a 400ms easeInOut tween of scrollLeft.
		smoothScroll(scroller, { left: scroller.scrollLeft + dir * visible * step });
	}

	$effect(() => {
		if (!scroller) return;
		update();
		const ro = new ResizeObserver(update);
		ro.observe(scroller);
		return () => ro.disconnect();
	});
</script>

<section class="hub">
	<div class="hub-header">
		{#if href}
			<a class="hub-title" {href}><h2>{title}</h2></a>
		{:else}
			<h2 class="hub-title">{title}</h2>
		{/if}
		{#if !(atStart && atEnd)}
			<div class="hub-actions">
				<button
					class="link scroll-button"
					class:disabled={atStart}
					type="button"
					aria-label="Previous Page"
					disabled={atStart}
					onclick={() => page(-1)}
				>
					<Icon name="chevron-left" size={18} />
				</button>
				<button
					class="link scroll-button"
					class:disabled={atEnd}
					type="button"
					aria-label="Next Page"
					disabled={atEnd}
					onclick={() => page(1)}
				>
					<Icon name="chevron-right" size={18} />
				</button>
			</div>
		{/if}
	</div>
	<div class="hub-scroller no-scrollbar" bind:this={scroller} onscroll={update}>
		{@render children()}
	</div>
</section>

<style>
	.hub {
		margin: 40px 0;
	}
	.hub:first-of-type {
		margin-top: 0;
	}
	.hub-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		min-height: 25px;
		margin-bottom: 10px;
		padding: 0 var(--page-gutter);
	}
	.hub-title,
	h2 {
		margin: 0;
		color: #fff;
		font-family: var(--font-heading-2-font-family);
		font-size: var(--font-heading-2-font-size);
		font-weight: var(--font-heading-2-font-weight);
		line-height: var(--font-heading-2-line-height);
	}
	a.hub-title:hover {
		text-decoration: underline;
		text-decoration-color: #fff;
	}
	.hub-actions {
		display: flex;
		flex-shrink: 0;
		margin-left: auto;
		font-size: 18px;
		line-height: 24px;
	}
	.scroll-button {
		display: flex;
		margin-left: 10px;
		padding: 0 5px;
	}
	.scroll-button:last-of-type {
		padding-right: 0;
	}
	.scroll-button.disabled {
		opacity: 0.15;
		cursor: default;
	}
	.hub-scroller {
		display: flex;
		gap: 30px;
		padding: 4px var(--page-gutter);
		overflow: scroll hidden;
	}
</style>
