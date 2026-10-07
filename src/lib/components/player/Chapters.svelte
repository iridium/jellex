<script lang="ts">
	import { clock } from '#lib/format.ts';
	import { player } from '#lib/player.svelte.ts';
	import { session } from '#lib/session.svelte.ts';

	// Plex Web's VideoChapters strip: "Chapter selection" above the bar, with
	// 174×97 thumbnails (Jellyfin chapter images, or a trickplay frame).
	const chapters = $derived(player.item?.Chapters ?? []);
	const currentIndex = $derived(
		chapters.reduce(
			(acc, c, i) => ((c.StartPositionTicks ?? 0) / 1e7 <= player.currentTime ? i : acc),
			0
		)
	);
	function image(i: number) {
		const c = chapters[i];
		if (c.ImageTag && player.item?.Id) {
			const api = session.requireApi;
			return {
				image: `${api.basePath}/Items/${player.item.Id}/Images/Chapter/${i}?fillWidth=348&quality=90&tag=${c.ImageTag}`
			};
		}
		return player.trickplay((c.StartPositionTicks ?? 0) / 1e7 + 1, 174);
	}
</script>

<div class="chapters">
	<div class="title">Chapter selection</div>
	<div class="strip no-scrollbar">
		{#each chapters as c, i (i)}
			{@const thumb = image(i)}
			<button
				class="cell"
				type="button"
				onclick={() => player.seek((c.StartPositionTicks ?? 0) / 1e7)}
			>
				<span
					class="thumb"
					class:active={i === currentIndex}
					style:background-image={thumb ? `url(${thumb.image})` : undefined}
					style:background-size={thumb && 'size' in thumb ? thumb.size : 'cover'}
					style:background-position={thumb && 'position' in thumb ? thumb.position : 'center'}
				></span>
				<span class="name">{c.Name}</span>
				<span class="time">{clock((c.StartPositionTicks ?? 0) / 1e7)}</span>
			</button>
		{/each}
	</div>
</div>

<style>
	/* Plex: VideoChapters + AudioVideoStripeContainer. */
	.chapters {
		position: absolute;
		z-index: 3;
		left: 0;
		right: 0;
		bottom: 100px;
		height: 272px;
		padding: 5px 15px;
		background-color: rgba(54, 56, 59, 0.9);
		box-shadow: 0 0 4px 0 rgba(0, 0, 0, 0.5);
	}
	.title {
		padding: 25px 5px 15px;
		color: #fff;
		font-family: var(--font-heading);
		font-size: 15px;
		line-height: 24px;
	}
	.strip {
		display: flex;
		gap: 30px;
		padding: 0 5px;
		overflow: auto hidden;
	}
	/* Plex: VideoChapterCell. */
	.cell {
		display: flex;
		flex-direction: column;
		flex-shrink: 0;
		width: 174px;
		text-align: left;
		color: hsla(0, 0%, 100%, 0.7);
	}
	.thumb {
		width: 174px;
		height: 97px;
		background-color: rgba(0, 0, 0, 0.45);
		background-repeat: no-repeat;
		transition: box-shadow 0.2s;
	}
	.cell:hover .thumb {
		box-shadow: 0 0 0 2px #fff;
	}
	.thumb.active {
		box-shadow: 0 0 0 2px var(--color-brand-accent);
	}
	.name {
		padding-top: 10px;
		color: #fff;
		line-height: 22.29px;
	}
	.time {
		color: hsla(0, 0%, 100%, 0.45);
		line-height: 22.29px;
	}
</style>
