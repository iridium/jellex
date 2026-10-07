<script lang="ts">
	import { fadeIn } from '#lib/motion.ts';
	import type { BaseItemDto } from '@jellyfin/sdk/lib/generated-client';
	import { duration, episodeCode, plural, ticksToSeconds } from '#lib/format.ts';
	import { imageUrl } from '#lib/images.ts';
	import { player } from '#lib/player.svelte.ts';
	import Equalizer from '../Equalizer.svelte';
	import Icon from '../Icon.svelte';

	// Plex Web's AudioVideoPlayQueue: "Play Queue" with its count, then 68px
	// rows; the playing one shows the equalizer, past ones fade, hovering a
	// row reveals its drag handle and remove button.
	let dragFrom = $state<number | null>(null);
	let dragOver = $state<number | null>(null);

	function subtitle(i: BaseItemDto) {
		if (i.Type === 'Episode') return [i.SeriesName, episodeCode(i)].filter(Boolean).join(' · ');
		if (i.Type === 'Audio') return [i.AlbumArtist, i.Album].filter(Boolean).join(' — ');
		return i.ProductionYear ? String(i.ProductionYear) : '';
	}
	function art(i: BaseItemDto) {
		return imageUrl(
			i,
			i.Type === 'Episode' ? 'landscape' : i.Type === 'Audio' ? 'square' : 'poster',
			100
		);
	}
	function drop(to: number) {
		if (dragFrom != null && dragFrom !== to) player.moveInQueue(dragFrom, to);
		dragFrom = dragOver = null;
	}
</script>

<div class="queue">
	<div class="top">
		<div>
			<h1>Play Queue</h1>
			<div class="count">{plural(player.queue.length, 'item')}</div>
		</div>
	</div>
	<div class="content scroller">
		{#each player.queue as entry, i (`${entry.Id}-${i}`)}
			{@const isCurrent = i === player.index}
			<div
				class="row"
				class:current={isCurrent}
				class:past={i < player.index}
				class:drag-over={dragOver === i}
				role="listitem"
				ondragover={(e) => (e.preventDefault(), (dragOver = i))}
				ondrop={() => drop(i)}
			>
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
							ondragstart={() => (dragFrom = i)}
							ondragend={() => (dragFrom = dragOver = null)}
						>
							<Icon name="list" size={14} />
						</span>
					{/if}
				</div>
				<button class="main" type="button" onclick={() => !isCurrent && player.jumpTo(i)}>
					<span
						class="poster"
						class:wide={entry.Type === 'Episode'}
						class:square={entry.Type === 'Audio'}
					>
						{#if art(entry)}<img src={art(entry)} alt="" loading="lazy" use:fadeIn />{/if}
					</span>
					<span class="titles">
						<span class="name">{entry.Name}</span>
						<span class="sub">{subtitle(entry)}</span>
					</span>
				</button>
				<span class="duration"
					>{entry.RunTimeTicks ? duration(ticksToSeconds(entry.RunTimeTicks)) : ''}</span
				>
				{#if !isCurrent}
					<button
						class="remove"
						type="button"
						aria-label="Remove from Play Queue"
						onclick={() => player.removeFromQueue(i)}
					>
						<Icon name="close" size={14} />
					</button>
				{/if}
			</div>
		{/each}
	</div>
</div>

<style>
	/* Plex: AudioVideoPlayQueue. */
	.queue {
		position: absolute;
		inset: 0;
		display: flex;
		flex-direction: column;
		padding: 20px 60px 0;
	}
	.top {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 0 20px 27px;
	}
	h1 {
		margin: 0;
		color: #fff;
		font-family: var(--font-heading);
		font-size: 24px;
		font-weight: 700;
		line-height: 40px;
	}
	.count {
		margin-top: 4px;
		line-height: 20px;
		color: hsla(0, 0%, 100%, 0.45);
		font-size: 15px;
	}
	.content {
		flex: 1;
		min-height: 0;
		overflow: hidden auto;
		border-top: 2px solid rgba(0, 0, 0, 0.2);
	}
	/* Plex: AudioVideoPlayQueueItem. */
	.row {
		position: relative;
		display: flex;
		align-items: center;
		height: 68px;
		padding-right: 20px;
		transition: opacity 0.2s;
	}
	.row.current {
		background-color: rgba(255, 255, 255, 0.04);
	}
	.row.past {
		opacity: 0.4;
	}
	.row:hover {
		opacity: 1;
		background-color: rgba(255, 255, 255, 0.04);
	}
	.row.drag-over {
		box-shadow: inset 0 2px 0 var(--color-brand-accent);
	}
	.left {
		display: flex;
		align-items: center;
		justify-content: center;
		flex-shrink: 0;
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
	.row:hover .move {
		opacity: 1;
	}
	.move:hover {
		color: #fff;
	}
	.main {
		display: flex;
		align-items: center;
		flex: 1;
		min-width: 0;
		height: 100%;
		text-align: left;
		cursor: default;
	}
	.poster {
		flex-shrink: 0;
		width: 64px;
		height: 54px;
		margin-left: 12px;
	}
	.poster img {
		height: 54px;
		width: 36px;
		object-fit: cover;
		border-radius: 2px;
	}
	.poster.wide img {
		width: 64px;
		height: 36px;
		margin-top: 9px;
	}
	.poster.square img {
		width: 54px;
	}
	.titles {
		display: flex;
		flex-direction: column;
		min-width: 0;
	}
	.name,
	.sub {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		line-height: 20px;
	}
	.name {
		color: #fff;
	}
	.sub {
		color: hsla(0, 0%, 100%, 0.45);
	}
	.duration {
		flex-shrink: 0;
		color: hsla(0, 0%, 100%, 0.45);
		transition: opacity 0.1s;
	}
	.remove {
		position: absolute;
		top: 0;
		right: 0;
		height: 68px;
		padding: 0 20px;
		color: hsla(0, 0%, 100%, 0.7);
		opacity: 0;
		transition: opacity 0.2s;
	}
	.row:hover .remove {
		opacity: 1;
	}
	.row:hover:has(.remove) .duration {
		opacity: 0;
	}
	.remove:hover {
		color: #fff;
	}
</style>
