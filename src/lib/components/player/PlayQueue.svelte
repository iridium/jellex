<script lang="ts">
	import { fadeIn } from '#lib/motion.ts';
	import type { BaseItemDto } from '@jellyfin/sdk/lib/generated-client';
	import { plural, shortDuration, ticksToSeconds } from '#lib/format.ts';
	import { imageUrl } from '#lib/images.ts';
	import { modals } from '#lib/modals.svelte.ts';
	import { player } from '#lib/player.svelte.ts';
	import Equalizer from '../Equalizer.svelte';
	import Icon from '../Icon.svelte';

	// Plex Web's AudioVideoPlayQueue: the "Play Queue" heading with its count
	// and Add to Playlist, then 68px rows striped every other one. The playing
	// row shows the equalizer; hovering any other reveals its drag handle and
	// a remove button in place of the duration.
	let dragFrom = $state<number | null>(null);
	/** Where the dragged row would land: before the row at this index. */
	let dropAt = $state<number | null>(null);

	// Plex: a 64px-wide card, 64×36 for episodes, otherwise 36 wide.
	function card(i: BaseItemDto) {
		if (i.Type === 'Episode') return { w: 64, h: 36, src: imageUrl(i, 'landscape', 64) };
		if (i.Type === 'Audio') return { w: 36, h: 36, src: imageUrl(i, 'square', 36) };
		return { w: 36, h: 54, src: imageUrl(i, 'poster', 36) };
	}
	function go() {
		if (player.mode === 'full') player.minimize();
	}
	function dragOver(e: DragEvent, i: number) {
		if (dragFrom == null) return;
		e.preventDefault();
		const r = (e.currentTarget as HTMLElement).getBoundingClientRect();
		dropAt = e.clientY - r.top > r.height / 2 ? i + 1 : i;
	}
	function drop() {
		if (dragFrom != null && dropAt != null) {
			const to = dropAt > dragFrom ? dropAt - 1 : dropAt;
			if (to !== dragFrom) player.moveInQueue(dragFrom, to);
		}
		dragFrom = dropAt = null;
	}
</script>

<div class="queue">
	<div class="heading-container">
		<h1>Play Queue</h1>
		<div class="top">
			<span class="count">{plural(player.queue.length, 'item')}</span>
			<button
				class="add"
				type="button"
				title="Add to Playlist"
				aria-label="Add to Playlist"
				onclick={() => modals.open({ kind: 'add-to-playlist', items: player.queue })}
			>
				<Icon name="add-to" size={24} />
			</button>
		</div>
		<div class="divider"></div>
	</div>
	<div class="content scroller" role="list">
		{#each player.queue as entry, i (`${entry.Id}-${i}`)}
			{@const isCurrent = i === player.index}
			{@const c = card(entry)}
			<div
				class="drag-source"
				class:dragging={dragFrom === i}
				class:drag-before={dropAt === i}
				class:drag-after={dropAt === i + 1}
				role="listitem"
				ondragover={(e) => dragOver(e, i)}
				ondrop={drop}
			>
				<div class="item" class:current={isCurrent} class:item-dragging={dragFrom === i}>
					<div class="left">
						{#if isCurrent}
							<Equalizer playing={!player.paused} />
						{:else}
							<span
								class="move"
								draggable="true"
								role="button"
								tabindex="-1"
								aria-label="Move"
								ondragstart={(e) => {
									dragFrom = i;
									const row = (e.currentTarget as HTMLElement).closest('.drag-source');
									if (row && e.dataTransfer) e.dataTransfer.setDragImage(row, 30, 34);
								}}
								ondragend={() => (dragFrom = dropAt = null)}
							>
								<Icon name="reorder" size={14} />
							</span>
						{/if}
					</div>
					<div class="metadata">
						<div class="card-container">
							<div class="card" style:width="{c.w}px" style:height="{c.h}px">
								{#if c.src}<img src={c.src} alt="" loading="lazy" use:fadeIn />{/if}
								<button
									class="play-button"
									type="button"
									aria-label="Play {entry.Name}"
									onclick={() => player.jumpTo(i)}
								>
									<span class="play-circle"><Icon name="play" size={20} /></span>
								</button>
							</div>
						</div>
						<div class="titles">
							{#if entry.Type === 'Episode'}
								<a class="title" href="/items/{entry.SeriesId ?? entry.Id}" onclick={go}
									>{entry.SeriesName ?? entry.Name}</a
								>
								<span class="title secondary">
									{#if entry.ParentIndexNumber != null}<a
											href="/items/{entry.SeasonId ?? entry.Id}"
											onclick={go}>S{entry.ParentIndexNumber}</a
										><span class="sep">·</span>{/if}{#if entry.IndexNumber != null}<a
											href="/items/{entry.Id}"
											onclick={go}>E{entry.IndexNumber}</a
										><span class="sep">—</span>{/if}<a href="/items/{entry.Id}" onclick={go}
										>{entry.Name}</a
									>
								</span>
							{:else if entry.Type === 'Audio'}
								<span class="title">{entry.Name}</span>
								<span class="title secondary">
									{#if entry.AlbumArtists?.[0]?.Id}<a
											href="/items/{entry.AlbumArtists[0].Id}"
											onclick={go}>{entry.AlbumArtists[0].Name}</a
										>{:else}{entry.AlbumArtist ?? ''}{/if}{#if entry.Album}<span class="sep">—</span
										>{#if entry.AlbumId}<a href="/items/{entry.AlbumId}" onclick={go}
												>{entry.Album}</a
											>{:else}{entry.Album}{/if}{/if}
								</span>
							{:else}
								<a class="title" href="/items/{entry.Id}" onclick={go}>{entry.Name}</a>
								{#if entry.ProductionYear}
									<span class="title secondary">{entry.ProductionYear}</span>
								{/if}
							{/if}
						</div>
						<div class="duration">
							{entry.RunTimeTicks ? shortDuration(ticksToSeconds(entry.RunTimeTicks)) : ''}
						</div>
					</div>
					{#if !isCurrent}
						<div class="remove-controls">
							<button
								class="remove"
								type="button"
								title="Remove"
								aria-label="Remove"
								onclick={() => player.removeFromQueue(i)}
							>
								<Icon name="remove" size={14} />
							</button>
						</div>
					{/if}
				</div>
			</div>
		{/each}
	</div>
</div>

<style>
	/* Plex: AudioVideoPlayQueue. */
	.queue {
		position: absolute;
		top: 0;
		left: 0;
		display: flex;
		flex-direction: column;
		width: 100%;
		height: 100%;
		padding: 20px 60px 0;
	}
	/* Plex: AudioVideoFullPlayerContentHeading. */
	h1 {
		margin: 0;
		padding: 0 20px;
		color: #fff;
		font-family: var(--font-heading);
		font-size: 24px;
		font-weight: 700;
		line-height: 41.14px;
	}
	.top {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 0 20px;
	}
	.count {
		margin-right: 20px;
		color: hsla(0, 0%, 100%, 0.45);
		font-size: 15px;
		line-height: 25.71px;
	}
	.add {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 48px;
		height: 32px;
		padding: 0 12px;
		color: hsla(0, 0%, 100%, 0.8);
	}
	.add:hover {
		color: #fff;
	}
	.divider {
		height: 2px;
		margin-top: 20px;
		background-color: rgba(0, 0, 0, 0.3);
	}
	.content {
		position: relative;
		flex-grow: 1;
		min-height: 0;
		overflow: hidden auto;
	}

	/* Plex: AudioVideoPlayQueueItemDragSource. */
	.drag-source:nth-child(odd) {
		background-color: hsla(0, 0%, 100%, 0.02);
	}
	.drag-source:hover {
		background-color: hsla(0, 0%, 100%, 0.06);
	}
	.drag-source.dragging {
		background-color: rgba(0, 0, 0, 0.15);
	}
	.drag-source.drag-before {
		box-shadow: inset 0 4px 0 -2px var(--color-brand-accent);
	}
	.drag-source.drag-after {
		box-shadow: 0 2px 0 0 var(--color-brand-accent);
	}

	/* Plex: AudioVideoPlayQueueItem. */
	.item {
		position: relative;
		display: flex;
		align-items: center;
		justify-content: space-between;
		height: 68px;
		padding: 0 20px 0 0;
		font-size: 13px;
	}
	.left {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 54px;
		height: 100%;
	}
	.move {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 100%;
		height: 100%;
		color: hsla(0, 0%, 100%, 0.7);
		cursor: grab;
		opacity: 0;
		transition: opacity 0.2s;
	}
	.move:hover {
		color: #fff;
	}
	.item:hover .move,
	.item-dragging .move,
	.item:hover .remove-controls {
		opacity: 1;
	}
	.remove-controls {
		position: absolute;
		top: 0;
		right: 0;
		opacity: 0;
		transition: opacity 0.2s;
	}
	.remove {
		display: flex;
		align-items: center;
		height: 68px;
		padding: 0 20px;
		color: #f53;
	}

	/* Plex: AudioVideoPlayerPlayQueueMetadata. */
	.metadata {
		display: flex;
		align-items: center;
		justify-content: space-between;
		width: 100%;
		height: 100%;
		overflow: hidden;
	}
	.card-container {
		flex-shrink: 0;
		width: 64px;
		margin: 0 14px 0 0;
	}
	/* Plex: MetadataPosterButtonCard. */
	.card {
		position: relative;
		margin: 0 auto;
		background-color: hsla(0, 0%, 100%, 0.1);
	}
	.card img {
		display: block;
		width: 100%;
		height: 100%;
		object-fit: cover;
	}
	.play-button {
		position: absolute;
		top: 50%;
		left: 50%;
		z-index: 3;
		width: 100%;
		height: 100%;
		line-height: 0;
		opacity: 0;
		transform: translate(-50%, -50%);
	}
	.card:hover .play-button {
		opacity: 1;
	}
	/* Plex: PlayButton. */
	.play-circle {
		position: absolute;
		top: 50%;
		left: 50%;
		display: flex;
		align-items: center;
		justify-content: center;
		width: 24px;
		height: 24px;
		border: 2px solid hsla(0, 0%, 100%, 0.7);
		border-radius: 50%;
		color: hsla(0, 0%, 100%, 0.7);
		transform: translate(-50%, -50%);
		transition: all 0.2s;
	}
	.play-button:hover .play-circle {
		background-color: var(--color-brand-accent);
		border-color: var(--color-brand-accent);
		color: #1f2326;
	}
	.titles {
		display: flex;
		flex-direction: column;
		width: 100%;
		overflow: hidden;
	}
	/* Plex: MetadataPosterTitle. */
	.title {
		display: block;
		height: 20px;
		min-width: 0;
		max-width: 100%;
		overflow: hidden;
		color: #fff;
		line-height: 20px;
		text-overflow: ellipsis;
		white-space: nowrap;
		user-select: none;
	}
	.title.secondary,
	.title.secondary a {
		color: hsla(0, 0%, 100%, 0.45);
	}
	a.title:hover,
	.title a:hover {
		text-decoration: underline;
	}
	.title.secondary a {
		transition: color 0.2s;
	}
	.title.secondary a:hover {
		color: #fff;
	}
	/* Plex: DashSeparator. */
	.sep {
		margin: 0 4px;
	}
	.duration {
		padding-left: 20px;
		color: hsla(0, 0%, 100%, 0.45);
		white-space: nowrap;
		transition: opacity 0.1s;
	}
	.item:hover .duration {
		opacity: 0;
	}
	.item.current:hover .duration {
		opacity: 1;
	}
</style>
