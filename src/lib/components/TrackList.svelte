<script lang="ts">
	import type { BaseItemDto } from '@jellyfin/sdk/lib/generated-client';
	import { itemMenu, loadPlaylists } from '#lib/actions.svelte.ts';
	import { clock, plural, ticksToSeconds } from '#lib/format.ts';
	import { player } from '#lib/player.svelte.ts';
	import { selection } from '#lib/selection.svelte.ts';
	import Equalizer from './Equalizer.svelte';
	import Icon from './Icon.svelte';
	import Menu from './Menu.svelte';
	import StarRating from './StarRating.svelte';

	// Plex Web's AlbumDisc: "N Tracks", then 40px MetadataTableRows with
	// faint alternating stripes. Hovering a row shows the select circle in
	// the gutter, swaps the track number for a play circle, and reveals
	// the star rating and a ⋮ menu in place of the duration.
	let { tracks }: { tracks: BaseItemDto[] } = $props();

	const allSelected = $derived(tracks.length > 0 && tracks.every((t) => selection.has(t.Id)));
	function selectAll() {
		const select = !allSelected;
		for (const t of tracks) if (selection.has(t.Id) !== select) selection.toggle(t);
	}
</script>

<section class="disc">
	<!-- Plex: PrePlayListTitle; hovering shows a select-all circle. -->
	<div class="title-container" class:all-selected={allSelected}>
		<button
			class="select select-all"
			class:is-selected={allSelected}
			type="button"
			aria-label="Select {plural(tracks.length, 'Track')}"
			onclick={selectAll}
		>
			<span class="select-circle"
				>{#if allSelected}<Icon name="check" size={9} />{/if}</span
			>
		</button>
		<h2 class="title">{plural(tracks.length, 'Track')}</h2>
	</div>
	<div class="track-list">
		{#each tracks as track, i (track.Id)}
			{@const playing = player.mode !== 'closed' && player.current?.Id === track.Id}
			{@const selected = selection.has(track.Id)}
			<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
			<div
				class="row"
				class:alternate={i % 2 === 1}
				class:selected
				onclick={(e) => {
					// While selecting, clicking a row toggles it (Plex).
					if (!selection.count || (e.target as Element).closest('button, a, .menu')) return;
					selection.toggle(track);
				}}
			>
				<!-- Plex: RowSelectButton, in the gutter left of the row. -->
				<span class="select-container" class:shown={selected}>
					<button
						class="select"
						class:is-selected={selected}
						type="button"
						aria-label="Select {track.Name}"
						onclick={() => selection.toggle(track)}
					>
						<span class="select-circle"
							>{#if selected}<Icon name="check" size={9} />{/if}</span
						>
					</button>
				</span>
				<span class="play-cell">
					{#if playing}
						<Equalizer playing={!player.paused} />
					{:else}
						<span class="number">{track.IndexNumber ?? i + 1}</span>
						<button
							class="play-button"
							type="button"
							aria-label="Play"
							onclick={() => player.play(track, { queue: tracks, startIndex: i })}
						>
							<span class="play-circle"><Icon name="play" size={20} /></span>
						</button>
					{/if}
				</span>
				<span class="cell first"><span class="track-title">{track.Name}</span></span>
				<span class="cell rating"><StarRating item={track} /></span>
				<span class="cell actions">
					<span class="duration">{clock(ticksToSeconds(track.RunTimeTicks))}</span>
					<span class="more">
						<Menu variant="plain" align="right" items={itemMenu(track)} onopen={loadPlaylists}>
							{#snippet trigger()}<Icon
									name="more-vertical"
									size={13}
									label="More Actions"
								/>{/snippet}
						</Menu>
					</span>
				</span>
			</div>
		{/each}
	</div>
</section>

<style>
	.disc {
		padding: 0 var(--page-gutter);
	}
	/* Plex: AlbumDisc-titleContainer + PrePlayListTitle. */
	/* Plex's heading sits inline in a 13px line, and PlexCircular's metrics
	   make that line 39.3px tall; the height is set directly since Figtree's
	   differ. */
	.title-container {
		position: relative;
		height: 59.3px;
		padding-bottom: 20px;
	}
	.title {
		margin: 0;
		color: #fff;
		font-family: var(--font-heading-2-font-family);
		font-size: var(--font-heading-2-font-size);
		font-weight: 700;
		line-height: 32px;
	}
	.track-list {
		margin-bottom: 40px;
		border-top: 2px solid #1f2326;
		border-bottom: 2px solid #1f2326;
	}
	/* Plex: ListRow + MetadataTableRow. */
	.row {
		position: relative;
		display: flex;
		height: 40px;
		cursor: default;
	}
	.row.alternate {
		background-color: rgba(255, 255, 255, 0.02);
	}
	.row:hover {
		background-color: rgba(255, 255, 255, 0.06);
	}
	.row.selected {
		background-color: rgba(0, 0, 0, 0.15);
	}
	.row.selected:hover {
		background-color: rgba(255, 255, 255, 0.06);
	}

	/* Plex: RowSelectButton + SelectButton. */
	.select-container {
		position: absolute;
		left: -30px;
		display: flex;
		align-items: center;
		min-width: 30px;
		height: 100%;
	}
	.select {
		display: flex;
		align-items: center;
		width: 100%;
		height: 100%;
		color: hsla(0, 0%, 100%, 0.7);
		opacity: 0;
		transition: color 0.2s;
	}
	.row:hover .select,
	.select-container.shown .select {
		opacity: 1;
	}
	/* Plex: AlbumDisc-selectButton, 30px left of the title. */
	.select-all {
		position: absolute;
		top: 9px;
		left: -30px;
		width: 14px;
		height: 14px;
	}
	.title-container:hover .select-all,
	.title-container.all-selected .select-all {
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

	/* Plex: MetadataTableRow-playCell + PlayButton. */
	.play-cell {
		position: absolute;
		display: flex;
		align-items: center;
		justify-content: center;
		width: 42px;
		height: 100%;
		overflow: hidden;
		color: #eee;
	}
	.play-button {
		display: none;
		line-height: 0;
	}
	.row:hover .number {
		display: none;
	}
	.row:hover .play-button {
		display: block;
	}
	.play-circle {
		position: relative;
		display: inline-grid;
		place-items: center;
		box-sizing: border-box;
		width: 24px;
		height: 24px;
		border: 2px solid hsla(0, 0%, 100%, 0.7);
		border-radius: 50%;
		color: hsla(0, 0%, 100%, 0.7);
		transition: all 0.2s;
	}
	.play-button:hover .play-circle {
		background-color: var(--color-brand-accent);
		border-color: var(--color-brand-accent);
		color: #1f2326;
	}

	/* Plex: TableCell. */
	.cell {
		display: flex;
		align-items: center;
		min-width: 0;
		height: 100%;
	}
	.cell.first {
		flex: 1;
		padding: 0 20px 0 62px;
	}
	.track-title {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		color: #fff;
		user-select: none;
	}
	.cell.rating {
		flex-shrink: 0;
		width: 100px;
		opacity: 0;
	}
	.row:hover .cell.rating {
		opacity: 1;
	}
	/* Plex: MetadataTableRow-actionsCell. The duration, or ⋮ on hover. */
	.cell.actions {
		flex-shrink: 0;
		justify-content: flex-end;
		align-items: stretch;
		width: 82px;
		padding: 0 10px;
	}
	.duration {
		align-self: center;
		color: hsla(0, 0%, 100%, 0.45);
	}
	.more {
		display: none;
	}
	.row:hover .duration,
	.row:has(:global(.trigger.open)) .duration {
		display: none;
	}
	.row:hover .more,
	.row:has(:global(.trigger.open)) .more {
		display: flex;
	}
	.more :global(.trigger) {
		height: 100%;
		padding: 0 10px;
		color: hsla(0, 0%, 100%, 0.7);
		transition: color 0.2s;
	}
	.more :global(.trigger:hover) {
		color: #fff;
	}
</style>
