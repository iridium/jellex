<script lang="ts">
	import type { BaseItemDto } from '@jellyfin/sdk/lib/generated-client';
	import { untrack } from 'svelte';
	import type { ImageKind } from '#lib/images.ts';
	import { posterSize } from '#lib/ui.svelte.ts';
	import type { ListStyle } from '#lib/ui.svelte.ts';
	import ListRow from './ListRow.svelte';
	import PosterCard from './PosterCard.svelte';

	// Plex Web's library grid (a virtual list of poster rows). Only rows near
	// the viewport are rendered; items load 100 at a time, and missing ones
	// show Plex's shimmering placeholder card until their page arrives.
	let {
		scroller,
		kind,
		fetch,
		total = $bindable(null),
		layout = 'grid',
		addedLine = false
	}: {
		/** The element that scrolls the grid. */
		scroller: HTMLElement;
		kind: ImageKind;
		/** Loads items [start, start + limit) and the total count. */
		fetch: (start: number, limit: number) => Promise<{ items: BaseItemDto[]; total: number }>;
		total?: number | null;
		/** Plex's list style: poster grid, Detail View rows or Table View rows. */
		layout?: ListStyle;
		/** Cards get a third line with the date added (Plex's hub lists). */
		addedLine?: boolean;
	} = $props();

	const PAGE = 100;
	const GAP = 30;
	const OVERSCAN = 2;

	const cardWidth = $derived(
		kind === 'landscape' ? Math.round((posterSize.width * 280) / 165) : posterSize.width
	);
	const cardHeight = $derived(
		Math.floor(cardWidth * (kind === 'square' ? 1 : kind === 'landscape' ? 0.5625 : 1.5))
	);
	// Grid: card, 8px margin, two 24px text lines (three with the date
	// added), then the row gap.
	// Detail View rows are 136px, Table View rows 40px.
	const rowHeight = $derived(
		layout === 'detail'
			? 136
			: layout === 'table'
				? 40
				: cardHeight + 8 + (addedLine ? 72 : 48) + GAP
	);

	let grid: HTMLDivElement | undefined = $state();
	let width = $state(0);
	let viewTop = $state(0);
	let viewHeight = $state(0);
	let pages = $state(new Map<number, BaseItemDto[]>());
	let generation = 0;
	const pending = new Set<number>();

	const cols = $derived(
		layout === 'grid' ? Math.max(1, Math.floor((width + GAP) / (cardWidth + GAP))) : 1
	);
	const rows = $derived(total == null ? 0 : Math.ceil(total / cols));

	const range = $derived.by(() => {
		if (total == null) return { start: 0, end: 0 };
		const first = Math.max(0, Math.floor(viewTop / rowHeight) - OVERSCAN);
		const last = Math.min(rows, Math.ceil((viewTop + viewHeight) / rowHeight) + OVERSCAN);
		return { start: first * cols, end: Math.min(total, last * cols) };
	});

	function item(i: number): BaseItemDto | undefined {
		return pages.get(Math.floor(i / PAGE))?.[i % PAGE];
	}

	async function load(page: number) {
		if (pages.has(page) || pending.has(page)) return;
		pending.add(page);
		const gen = generation;
		try {
			const res = await fetch(page * PAGE, PAGE);
			if (gen !== generation) return;
			total = res.total;
			pages = new Map(pages).set(page, res.items);
		} finally {
			pending.delete(page);
		}
	}

	// A new query (new fetch function) starts over from the top.
	$effect(() => {
		void fetch;
		untrack(() => {
			generation++;
			pending.clear();
			pages = new Map();
			total = null;
			load(0);
		});
	});

	// Load every page the visible range touches.
	$effect(() => {
		if (total == null) return;
		const { start, end } = range;
		untrack(() => {
			for (let p = Math.floor(start / PAGE); p <= Math.floor(Math.max(start, end - 1) / PAGE); p++)
				load(p);
		});
	});

	function measure() {
		if (!grid) return;
		const top = grid.getBoundingClientRect().top - scroller.getBoundingClientRect().top;
		viewTop = Math.max(0, -top);
		viewHeight = scroller.clientHeight;
	}

	$effect(() => {
		if (!grid) return;
		const ro = new ResizeObserver(() => {
			width = grid!.clientWidth;
			measure();
		});
		ro.observe(grid);
		scroller.addEventListener('scroll', measure, { passive: true });
		measure();
		return () => {
			ro.disconnect();
			scroller.removeEventListener('scroll', measure);
		};
	});

	/** Scrolls so the item at index is in the first visible row. */
	export function jumpTo(index: number) {
		if (!grid) return;
		const gridTop =
			grid.getBoundingClientRect().top - scroller.getBoundingClientRect().top + scroller.scrollTop;
		scroller.scrollTo({ top: gridTop + Math.floor(index / cols) * rowHeight - 16 });
	}

	const indices = $derived(
		Array.from({ length: range.end - range.start }, (_, k) => range.start + k)
	);
</script>

<div
	class="poster-grid"
	bind:this={grid}
	style:height="{Math.max(0, rows * rowHeight - (layout === 'grid' ? GAP : 0))}px"
	style:--card-width="{cardWidth}px"
	style:--card-height="{cardHeight}px"
>
	{#each indices as i (i)}
		{@const it = item(i)}
		<div
			class="cell"
			class:row-cell={layout !== 'grid'}
			style:transform="translate({(i % cols) * (cardWidth + GAP)}px, {Math.floor(i / cols) *
				rowHeight}px)"
			style:height={layout !== 'grid' ? `${rowHeight}px` : undefined}
		>
			{#if it}
				{#if layout === 'grid'}
					<PosterCard item={it} {kind} width={cardWidth} {addedLine} />
				{:else}
					<ListRow item={it} style={layout} index={i} />
				{/if}
			{:else if layout === 'grid'}
				<div class="placeholder"></div>
			{/if}
		</div>
	{/each}
</div>

<style>
	.poster-grid {
		position: relative;
	}
	.cell {
		position: absolute;
		top: 0;
		left: 0;
		width: var(--card-width);
	}
	.cell.row-cell {
		right: 0;
		width: auto;
	}
	/* Plex: MetadataPosterCardPlaceholder. */
	.placeholder {
		height: var(--card-height);
		border-radius: 4px;
		background-color: rgba(0, 0, 0, 0.45);
		box-shadow: 0 0 4px rgba(0, 0, 0, 0.3);
		animation: card-shimmer 0.6s ease-in infinite alternate;
	}
	@keyframes card-shimmer {
		0% {
			opacity: 1;
		}
		to {
			opacity: 0.75;
		}
	}
</style>
