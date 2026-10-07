<script lang="ts">
	import { afterNavigate } from '$app/navigation';
	import {
		addToEntries,
		loadPlaylists,
		markWatched,
		removeFromContinueWatching
	} from '#lib/actions.svelte.ts';
	import { resolveQueue } from '#lib/playback.ts';
	import { player } from '#lib/player.svelte.ts';
	import { selection } from '#lib/selection.svelte.ts';
	import Icon from './Icon.svelte';
	import Menu from './Menu.svelte';

	// Plex Web's PageHeaderMultiselectActions: replaces the toolbar while
	// items are selected — the count, actions for all of them, Deselect All.
	afterNavigate(() => selection.clear());

	const items = $derived(selection.items);
	async function watchAll(watched: boolean) {
		const all = selection.items;
		selection.clear();
		for (const i of all) await markWatched(i, watched);
	}
	const addTo = $derived(addToEntries(items));

	// Shows, seasons and albums expand to what they play.
	async function playable() {
		return (
			await Promise.all(items.map((i) => (i.IsFolder || !i.MediaType ? resolveQueue(i) : [i])))
		).flat();
	}
	async function play(shuffle = false) {
		const queue = await playable();
		if (!queue.length) return;
		if (shuffle)
			for (let i = queue.length - 1; i > 0; i--) {
				const j = Math.floor(Math.random() * (i + 1));
				[queue[i], queue[j]] = [queue[j], queue[i]];
			}
		selection.clear();
		player.play(queue[0], { queue });
	}
	async function queue(next: boolean) {
		player.addToQueue(await playable(), next);
		selection.clear();
	}
	const inProgress = $derived(items.filter((i) => (i.UserData?.PlaybackPositionTicks ?? 0) > 0));
	// Plex's More: Shuffle, Play Next, Add to Queue, and Remove from
	// Continue Watching when something selected is in progress.
	const more = $derived([
		{ label: 'Shuffle', onselect: () => play(true) },
		{ label: 'Play Next', onselect: () => queue(true) },
		{ label: 'Add to Queue', onselect: () => queue(false) },
		...(inProgress.length
			? [
					{
						label: 'Remove from Continue Watching',
						onselect: async () => {
							const list = inProgress;
							selection.clear();
							for (const i of list) await removeFromContinueWatching(i);
						}
					}
				]
			: [])
	]);
</script>

{#if selection.count}
	<div class="multiselect">
		<div class="count"><Icon name="check" size={12} /> {selection.count} items selected</div>
		<div class="actions">
			<button class="action" type="button" aria-label="Play" title="Play" onclick={() => play()}>
				<Icon name="play-solid" size={24} />
			</button>
			<button
				class="action"
				type="button"
				aria-label="Mark as Watched"
				title="Mark as Watched"
				onclick={() => watchAll(true)}
			>
				<Icon name="watched" size={24} />
			</button>
			<span class="action menu-action">
				<Menu variant="plain" onopen={loadPlaylists} items={addTo}>
					{#snippet trigger()}<Icon name="add-to" size={24} label="Add to..." />{/snippet}
				</Menu>
			</span>
			<span class="action menu-action">
				<Menu variant="plain" items={more}>
					{#snippet trigger()}<Icon name="more-horizontal" size={24} label="More" />{/snippet}
				</Menu>
			</span>
		</div>
		<div class="deselect">
			<button class="link" type="button" onclick={() => selection.clear()}
				><Icon name="close" size={12} /> Deselect All</button
			>
		</div>
	</div>
{/if}

<style>
	.multiselect {
		display: flex;
		align-items: center;
		justify-content: space-between;
		flex-shrink: 0;
		height: 50px;
		padding: 0 20px;
		border: 2px solid var(--color-brand-accent);
		border-radius: 4px;
		background-color: rgba(0, 0, 0, 0.45);
		color: hsla(0, 0%, 100%, 0.75);
	}
	.count {
		display: flex;
		align-items: center;
		gap: 6px;
		width: 282px;
		color: var(--color-brand-accent);
		font-size: 13px;
	}
	.actions {
		display: flex;
		gap: 8px;
	}
	.action {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		min-width: 56px;
		height: 40px;
		padding: 0 16px;
		border-radius: 4px;
		color: #fff;
		transition: background-color var(--duration-fast);
	}
	.action:hover {
		background-color: var(--color-background-control-focus);
	}
	.menu-action {
		padding: 0;
	}
	.menu-action :global(.trigger) {
		justify-content: center;
		min-width: 56px;
		color: #fff;
	}
	.deselect {
		display: flex;
		justify-content: flex-end;
		width: 282px;
		font-size: 12px;
	}
	.deselect button {
		display: inline-flex;
		align-items: center;
		gap: 4px;
	}
</style>
