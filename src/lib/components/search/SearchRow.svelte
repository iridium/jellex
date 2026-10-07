<script lang="ts">
	import { fadeIn } from '#lib/motion.ts';
	import type { BaseItemDto } from '@jellyfin/sdk/lib/generated-client';
	import { itemMenu, loadPlaylists } from '#lib/actions.svelte.ts';
	import { imageUrl } from '#lib/images.ts';
	import { player } from '#lib/player.svelte.ts';
	import { describe } from '#lib/search.ts';
	import { selection } from '#lib/selection.svelte.ts';
	import Icon from '../Icon.svelte';
	import Menu from '../Menu.svelte';

	// Plex Web's SearchResultListRow: a 60×90 poster, a bold title, context
	// lines and the server name (accent), with Play and ⋮ on the right.
	let {
		item,
		serverName,
		wide = false,
		onnavigate
	}: { item: BaseItemDto; serverName: string; wide?: boolean; onnavigate?: () => void } = $props();

	const square = $derived(
		['MusicAlbum', 'MusicArtist', 'Audio', 'Playlist'].includes(item.Type ?? '')
	);
	const src = $derived(imageUrl(item, square ? 'square' : 'poster', 90));
	const person = $derived(item.Type === 'Person');
	const selected = $derived(selection.has(item.Id));
</script>

<div class="row" class:wide class:selected>
	<a
		class="underlay"
		href="/items/{item.Id}"
		aria-label={item.Name}
		onclick={(e) => {
			if (wide && selection.count) {
				e.preventDefault();
				selection.toggle(item);
			} else onnavigate?.();
		}}
	></a>
	{#if wide && !person}
		<!-- Plex: SearchResultCell-selectButtonContainer (results page only). -->
		<button
			class="select"
			class:is-selected={selected}
			type="button"
			aria-label="Select {item.Name}"
			onclick={() => selection.toggle(item)}
		>
			<span class="select-circle"
				>{#if selected}<Icon name="check" size={9} />{/if}</span
			>
		</button>
	{/if}
	<div class="overlay">
		<div class="poster" class:square class:round={person}>
			{#if src}<img {src} alt="" loading="lazy" use:fadeIn />{/if}
		</div>
		<div class="text">
			<h3>{item.Name}</h3>
			{#each describe(item) as line, i (i)}
				{#if line}<span class="line">{line}</span>{/if}
			{/each}
			<span class="server">{serverName}</span>
		</div>
		{#if !person}
			<div class="actions">
				<button
					class="action play"
					type="button"
					aria-label="Play"
					onclick={() => player.play(item, { askResume: true })}
				>
					<Icon name="play-circle" size={24} />
				</button>
				<span class="action more">
					<Menu variant="plain" align="right" items={itemMenu(item)} onopen={loadPlaylists}>
						{#snippet trigger()}<Icon
								name="more-vertical-48"
								size={24}
								label="More Actions"
							/>{/snippet}
					</Menu>
				</span>
			</div>
		{/if}
	</div>
</div>

<style>
	/* Plex: SearchResultListRow. */
	/* Plex: SearchResultListRow. Rows are divided by a 1px top border,
	   hidden on the first row and around a hovered or selected row, which
	   gets a rounded tint. */
	.row {
		position: relative;
		margin: 0 16px;
		border-top: 1px solid var(--color-primary-background-10);
		cursor: pointer;
		transition:
			background-color var(--duration-fast) ease-in-out,
			border-radius var(--duration-fast) ease-in-out;
	}
	/* The results page wraps each row in its own list item, so every row
	   is a first child there and no dividers show. */
	.row.wide,
	.row:first-child,
	.row:hover,
	.row:hover + :global(.row),
	.row.selected,
	.row.selected + :global(.row) {
		border-top-color: transparent;
	}
	.row:hover,
	.row.selected {
		background-color: var(--color-primary-background-10);
		border-radius: var(--border-radius-m);
	}
	.underlay {
		position: absolute;
		inset: 0;
	}
	/* Plex: SearchResultCell-selectButtonContainer + SelectButton. */
	.select {
		position: absolute;
		top: 0;
		bottom: 0;
		left: 0;
		z-index: 1;
		display: flex;
		align-items: center;
		justify-content: center;
		width: 32px;
		color: hsla(0, 0%, 100%, 0.7);
		opacity: 0;
	}
	.row:hover .select,
	.select.is-selected {
		opacity: 1;
	}
	.select-circle {
		display: grid;
		place-items: center;
		box-sizing: border-box;
		width: 14px;
		height: 14px;
		border: 2px solid hsla(0, 0%, 100%, 0.7);
		border-radius: 50%;
		transition: all 0.2s;
	}
	.select:hover .select-circle {
		border-color: #fff;
	}
	.select.is-selected .select-circle {
		color: rgba(0, 0, 0, 0.75);
		background-color: var(--color-brand-accent);
		border-color: var(--color-brand-accent);
		box-shadow: 0 0 4px rgba(0, 0, 0, 0.6);
		transform: scale(1.4);
	}
	.overlay {
		position: relative;
		display: flex;
		align-items: center;
		height: 113px;
		padding: 12px;
		pointer-events: none;
	}
	.wide .overlay {
		padding-left: 32px;
	}
	.poster {
		flex-shrink: 0;
		width: 60px;
		height: 90px;
		overflow: hidden;
		border-radius: 4px;
		background-color: rgba(255, 255, 255, 0.1);
	}
	.poster.square {
		height: 60px;
	}
	.poster.round {
		height: 60px;
		border-radius: 50%;
	}
	.poster img {
		width: 100%;
		height: 100%;
		object-fit: cover;
	}
	.text {
		display: flex;
		flex-direction: column;
		flex: 1;
		min-width: 0;
		margin-left: 16px;
		color: rgba(255, 255, 255, 0.8);
		font-size: 14px;
		line-height: 20px;
	}
	h3 {
		margin: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		color: #fff;
		font-family: var(--font-heading);
		font-size: 16px;
		font-weight: 700;
		line-height: 24px;
	}
	.line,
	.server {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.server {
		color: var(--color-brand-accent);
	}
	.actions {
		display: flex;
		align-items: center;
		flex-shrink: 0;
		pointer-events: auto;
	}
	/* Plex: SearchResultItemContentActions. */
	/* The icon sits at the button's top padding, not centred: 45px tall in
	   the popover, 47.3px on the results page. */
	.action {
		display: flex;
		align-items: flex-start;
		justify-content: center;
		width: 48px;
		height: 45px;
		padding: 8px;
	}
	.wide .action {
		height: 47.3px;
	}
	.play {
		color: var(--color-brand-accent);
	}
	.play:hover {
		color: var(--color-accent-light);
	}
	.more :global(.trigger) {
		justify-content: center;
		width: 48px;
		height: 45px;
		color: hsla(0, 0%, 100%, 0.7);
	}
	.more :global(.trigger:hover) {
		color: #fff;
	}
</style>
