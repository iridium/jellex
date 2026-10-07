<script lang="ts">
	import { fadeIn } from '#lib/motion.ts';
	import type { BaseItemDto } from '@jellyfin/sdk/lib/generated-client';
	import { itemMenu, loadPlaylists } from '#lib/actions.svelte.ts';
	import { duration, ticksToSeconds } from '#lib/format.ts';
	import { imageUrl } from '#lib/images.ts';
	import { selection } from '#lib/selection.svelte.ts';
	import Icon from './Icon.svelte';
	import Menu from './Menu.svelte';

	// Plex Web's list styles: MetadataDetailsRow (Detail View, 136px rows
	// with a 64×96 poster) and MetadataTableRow (Table View, 40px rows).
	// Both have Plex's row select button in the left gutter.
	let { item, style, index }: { item: BaseItemDto; style: 'detail' | 'table'; index: number } =
		$props();

	const square = $derived(['MusicAlbum', 'MusicArtist', 'Playlist'].includes(item.Type ?? ''));
	const poster = $derived(
		style === 'detail' ? imageUrl(item, square ? 'square' : 'poster', 96) : undefined
	);
	const sub = $derived(
		[
			item.ProductionYear,
			item.RunTimeTicks ? duration(ticksToSeconds(item.RunTimeTicks)) : null
		].filter(Boolean)
	);
	const selected = $derived(selection.has(item.Id));
</script>

<div class="row {style}-row" class:alternate={index % 2 === 1} class:selected>
	<button
		class="select"
		type="button"
		aria-label="Select {item.Name}"
		onclick={() => selection.toggle(item)}
	>
		<span class="circle"
			>{#if selected}<Icon name="check" size={9} />{/if}</span
		>
	</button>
	<!-- While selecting, clicking a row toggles it (Plex). -->
	<a
		class="underlay"
		href="/items/{item.Id}"
		aria-label={item.Name}
		onclick={(e) => {
			if (!selection.count) return;
			e.preventDefault();
			selection.toggle(item);
		}}
	></a>
	<div class="overlay">
		{#if style === 'detail'}
			<div class="poster" class:square>
				{#if poster}<img src={poster} alt="" loading="lazy" use:fadeIn />{/if}
			</div>
			<div class="titles">
				<span class="title">{item.Name}</span>
				<span class="subtitle"
					>{#each sub as s, i (i)}{#if i}<span class="dot">·</span>{/if}{s}{/each}</span
				>
			</div>
		{:else}
			<div class="cell first"><a class="title" href="/items/{item.Id}">{item.Name}</a></div>
		{/if}
		<div class="actions">
			<Menu variant="plain" align="right" items={itemMenu(item)} onopen={loadPlaylists}>
				{#snippet trigger()}<Icon name="more-vertical" size={14} label="More Actions" />{/snippet}
			</Menu>
		</div>
	</div>
</div>

<style>
	/* Plex: ListRow. */
	.row {
		position: relative;
		width: 100%;
		height: 100%;
	}
	.row.alternate {
		background-color: rgba(255, 255, 255, 0.02);
	}
	.row:hover,
	.row.selected {
		background-color: rgba(255, 255, 255, 0.06);
	}
	.underlay {
		position: absolute;
		inset: 0;
	}
	.overlay {
		position: relative;
		display: flex;
		align-items: center;
		width: 100%;
		height: 100%;
		pointer-events: none;
	}
	.overlay :global(a),
	.overlay :global(button) {
		pointer-events: auto;
	}
	/* Plex: RowSelectButton, in the 30px gutter left of the row. */
	.select {
		position: absolute;
		top: 0;
		left: -30px;
		display: flex;
		align-items: center;
		width: 30px;
		height: 100%;
		opacity: 0;
		transition: opacity 0.2s;
	}
	.row:hover .select,
	.row.selected .select {
		opacity: 1;
	}
	.circle {
		display: grid;
		place-items: center;
		box-sizing: border-box;
		width: 14px;
		height: 14px;
		border: 2px solid hsla(0, 0%, 100%, 0.7);
		border-radius: 50%;
		transition: all 0.2s;
	}
	.select:hover .circle {
		border-color: #fff;
	}
	.selected .circle {
		background-color: var(--color-brand-accent);
		border-color: var(--color-brand-accent);
		color: rgba(0, 0, 0, 0.75);
	}

	/* Plex: MetadataDetailsRow. */
	.detail-row .overlay {
		padding: 20px;
	}
	.poster {
		flex-shrink: 0;
		width: 64px;
		height: 96px;
		margin-right: 20px;
		overflow: hidden;
		border-radius: 4px;
		background-color: rgba(0, 0, 0, 0.45);
		box-shadow: 0 0 4px rgba(0, 0, 0, 0.3);
	}
	.poster.square {
		height: 64px;
	}
	.poster img {
		width: 100%;
		height: 100%;
		object-fit: cover;
	}
	.titles {
		display: flex;
		flex-direction: column;
		flex: 1;
		min-width: 0;
		padding-right: 20px;
	}
	.detail-row .title {
		color: #fff;
		font-size: 15px;
		line-height: 25.71px;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.subtitle {
		color: hsla(0, 0%, 100%, 0.45);
		font-size: 13px;
		line-height: 22.29px;
	}
	.dot {
		margin: 0 4px;
	}

	/* Plex: MetadataTableRow. */
	.cell.first {
		flex: 1;
		min-width: 0;
		padding: 0 20px 0 62px;
	}
	.table-row .title {
		color: #fff;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.table-row .title:hover {
		text-decoration: underline;
	}
	.actions {
		display: flex;
		justify-content: flex-end;
		flex-shrink: 0;
		width: 82px;
		padding: 0 10px;
		opacity: 0;
		transition: opacity 0.2s;
	}
	.detail-row .actions {
		width: auto;
	}
	.row:hover .actions {
		opacity: 1;
	}
	.actions :global(.trigger) {
		justify-content: center;
		width: 33px;
		height: 40px;
		color: hsla(0, 0%, 100%, 0.7);
	}
	.actions :global(.trigger:hover) {
		color: #fff;
	}
</style>
