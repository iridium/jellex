<script lang="ts">
	import type { BaseItemDto } from '@jellyfin/sdk/lib/generated-client';
	import { rate } from '#lib/actions.svelte.ts';
	import Icon from './Icon.svelte';

	// Plex Web's user rating: five 16px stars (half steps). Hovering
	// previews, clicking rates, clicking the current rating clears it.
	// Stored as Jellyfin's 0–10 user rating.
	let { item, size = 16 }: { item: BaseItemDto; size?: number } = $props();

	const saved = $derived((item.UserData?.Rating ?? 0) / 2);
	let hover = $state<number | null>(null);
	let local = $state<number | null>(null);
	let pop = $state(0);
	const shown = $derived(hover ?? local ?? saved);

	function valueAt(e: PointerEvent, i: number) {
		const r = (e.currentTarget as HTMLElement).getBoundingClientRect();
		return i + ((e.clientX - r.left) / r.width < 0.5 ? 0.5 : 1);
	}
	async function choose(v: number) {
		const next = v === (local ?? saved) ? 0 : v;
		local = next;
		pop++;
		await rate(item, next || null);
		local = null;
	}
</script>

<span
	class="rating"
	class:previewing={hover != null}
	role="group"
	aria-label="Rate"
	onpointerleave={() => (hover = null)}
>
	{#each [0, 1, 2, 3, 4] as i (i)}
		{@const fill = Math.max(0, Math.min(1, shown - i))}
		<button
			class="star"
			type="button"
			role="radio"
			aria-checked={shown > i}
			aria-label="{i + 1} star{i ? 's' : ''}"
			style:width="{size}px"
			style:height="{size}px"
			onpointermove={(e) => (hover = valueAt(e, i))}
			onclick={(e) => choose(valueAt(e as unknown as PointerEvent, i))}
		>
			<span class="layer empty" class:on={fill === 0}><Icon name="star" {size} /></span>
			{#if fill > 0}
				{#key pop}
					<span class="layer full" class:half={fill < 1}><Icon name="star-filled" {size} /></span>
				{/key}
				{#if fill < 1}<span class="layer half-empty"><Icon name="star" {size} /></span>{/if}
			{/if}
		</button>
	{/each}
</span>

<style>
	.rating {
		display: inline-flex;
		align-items: center;
		gap: 4px;
	}
	.star {
		position: relative;
		display: inline-flex;
		cursor: pointer;
	}
	.layer {
		position: absolute;
		inset: 0;
		display: flex;
	}
	.empty {
		color: var(--color-surface-foreground-30);
		transition: color var(--duration-fast);
	}
	.full {
		color: var(--color-text-primary);
	}
	.full.half {
		clip-path: inset(0 50% 0 0);
	}
	.half-empty {
		color: var(--color-text-primary);
		clip-path: inset(0 0 0 50%);
		opacity: 0.3;
	}
	.rating:hover .empty {
		color: hsla(0, 0%, 100%, 0.45);
	}
</style>
