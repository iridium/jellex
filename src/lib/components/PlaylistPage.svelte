<script lang="ts">
	import { fadeIn } from '#lib/motion.ts';
	import type { BaseItemDto } from '@jellyfin/sdk/lib/generated-client';
	import { getLibraryApi } from '@jellyfin/sdk/lib/utils/api/library-api';
	import { getPlaylistApi } from '@jellyfin/sdk/lib/utils/api/playlist-api';
	import { goto, invalidateAll } from '$app/navigation';
	import { itemMenu, loadPlaylists } from '#lib/actions.svelte.ts';
	import { clock, plural, ticksToSeconds } from '#lib/format.ts';
	import { imageUrl } from '#lib/images.ts';
	import { player } from '#lib/player.svelte.ts';
	import { session } from '#lib/session.svelte.ts';
	import Icon from './Icon.svelte';
	import Menu from './Menu.svelte';
	import Modal from './Modal.svelte';

	// Plex Web's playlist page: breadcrumb with Play / Shuffle / Edit / ⋮,
	// the 215px cover and title, then "N Videos" and 68px rows that can be
	// reordered, played, and removed.
	let {
		playlist,
		entries,
		serverName
	}: { playlist: BaseItemDto; entries: BaseItemDto[]; serverName: string } = $props();

	const cover = $derived(imageUrl(playlist, 'square', 215));
	const audio = $derived(playlist.MediaType === 'Audio');
	const countLabel = $derived(plural(entries.length, audio ? 'Track' : 'Video'));
	let renaming = $state(false);
	let confirmDelete = $state(false);
	let name = $state('');
	let dragFrom = $state<number | null>(null);
	let dragOver = $state<number | null>(null);

	function sub(i: BaseItemDto) {
		if (i.Type === 'Episode')
			return [
				i.SeriesName,
				i.ParentIndexNumber != null ? `S${i.ParentIndexNumber} · E${i.IndexNumber}` : ''
			]
				.filter(Boolean)
				.join(' · ');
		if (i.Type === 'Audio') return [i.AlbumArtist, i.Album].filter(Boolean).join(' — ');
		return i.ProductionYear ? String(i.ProductionYear) : '';
	}
	function thumb(i: BaseItemDto) {
		return imageUrl(i, i.Type === 'Episode' ? 'landscape' : audio ? 'square' : 'poster', 64);
	}
	async function remove(i: BaseItemDto) {
		await getPlaylistApi(session.requireApi).removeItemFromPlaylist({
			playlistId: playlist.Id!,
			entryIds: [i.PlaylistItemId!]
		});
		await invalidateAll();
	}
	async function move(from: number, to: number) {
		dragFrom = dragOver = null;
		if (from === to) return;
		await getPlaylistApi(session.requireApi).moveItem({
			playlistId: playlist.Id!,
			itemId: entries[from].PlaylistItemId!,
			newIndex: to
		});
		await invalidateAll();
	}
	async function rename(e: SubmitEvent) {
		e.preventDefault();
		if (!name.trim()) return;
		await getPlaylistApi(session.requireApi).updatePlaylist({
			playlistId: playlist.Id!,
			updatePlaylistDto: { Name: name.trim() }
		});
		renaming = false;
		await invalidateAll();
	}
	async function destroy() {
		await getLibraryApi(session.requireApi).deleteItem({ itemId: playlist.Id! });
		confirmDelete = false;
		await goto('/', { replace: true });
	}
	const play = (shuffle = false, start = 0) =>
		player.play(playlist, { queue: entries, startIndex: start, shuffle });
</script>

<!-- Plex: PrePlayPageHeader with ToolbarButtons. -->
<div class="page-header">
	<div class="crumbs">
		<a class="crumb link" href="/">{serverName}</a>
		<span class="crumb current">{playlist.Name}</span>
	</div>
	<div class="toolbar">
		<button class="tool" type="button" aria-label="Play" title="Play" onclick={() => play()}
			><Icon name="play-toolbar" size={20} /></button
		>
		<button
			class="tool"
			type="button"
			aria-label="Shuffle"
			title="Shuffle"
			onclick={() => play(true)}><Icon name="shuffle-toolbar" size={20} /></button
		>
		<button
			class="tool"
			type="button"
			aria-label="Edit"
			title="Edit"
			onclick={() => ((name = playlist.Name ?? ''), (renaming = true))}
			><Icon name="edit" size={20} /></button
		>
		<span class="tool">
			<Menu
				variant="plain"
				align="right"
				items={[{ label: 'Delete', danger: true, onselect: () => (confirmDelete = true) }]}
			>
				{#snippet trigger()}<Icon name="more-toolbar" size={20} label="More..." />{/snippet}
			</Menu>
		</span>
	</div>
</div>

<div class="page-content scroller">
	<div class="inner">
		<div class="metadata">
			<div class="cover">
				{#if cover}<img src={cover} alt="" use:fadeIn />{/if}
				<button
					class="cover-play"
					type="button"
					aria-label="Play {playlist.Name}"
					onclick={() => play()}
				>
					<span class="play-circle"><Icon name="play" size={42} /></span>
				</button>
			</div>
			<div class="titles">
				<div class="title">{playlist.Name}</div>
				<div class="divider"></div>
				{#if playlist.Overview}<p class="overview">{playlist.Overview}</p>{/if}
			</div>
		</div>

		<div class="list">
			<div class="list-header">{countLabel}</div>
			<div class="top-divider"></div>
			{#each entries as entry, i (entry.PlaylistItemId ?? i)}
				<div
					class="row"
					class:alternate={i % 2 === 1}
					class:drag-over={dragOver === i}
					role="listitem"
					ondragover={(e) => (e.preventDefault(), (dragOver = i))}
					ondrop={() => dragFrom != null && move(dragFrom, i)}
				>
					<span
						class="move"
						draggable="true"
						role="button"
						tabindex="-1"
						aria-label="Move"
						ondragstart={() => (dragFrom = i)}
						ondragend={() => (dragFrom = dragOver = null)}><Icon name="list" size={14} /></span
					>
					<span class="index">
						<span class="number">{i + 1}</span>
						<button
							class="row-play"
							type="button"
							aria-label="Play {entry.Name}"
							onclick={() => play(false, i)}
						>
							<Icon name="play" size={24} />
						</button>
					</span>
					<span class="card" class:wide={entry.Type === 'Episode'} class:square={audio}>
						{#if thumb(entry)}<img src={thumb(entry)} alt="" loading="lazy" use:fadeIn />{/if}
					</span>
					<span class="text">
						<a class="name" href="/items/{entry.Id}">{entry.Name}</a>
						<span class="sub">{sub(entry)}</span>
					</span>
					<span class="duration"
						>{entry.RunTimeTicks ? clock(ticksToSeconds(entry.RunTimeTicks)) : ''}</span
					>
					<span class="controls">
						<span class="control more">
							<Menu variant="plain" align="right" items={itemMenu(entry)} onopen={loadPlaylists}>
								{#snippet trigger()}<Icon
										name="more-vertical"
										size={14}
										label="More Actions"
									/>{/snippet}
							</Menu>
						</span>
						<button
							class="control remove"
							type="button"
							aria-label="Remove"
							title="Remove"
							onclick={() => remove(entry)}
						>
							<Icon name="remove" size={14} />
						</button>
					</span>
				</div>
			{/each}
		</div>
	</div>
</div>

{#if renaming}
	<Modal title="Edit Playlist" onclose={() => (renaming = false)}>
		<form class="form" onsubmit={rename}>
			<label for="pl-name">Title</label>
			<!-- svelte-ignore a11y_autofocus -->
			<input id="pl-name" bind:value={name} autofocus />
			<div class="form-footer">
				<button class="btn secondary" type="button" onclick={() => (renaming = false)}
					>Cancel</button
				>
				<button class="btn primary" type="submit">Save</button>
			</div>
		</form>
	</Modal>
{/if}
{#if confirmDelete}
	<Modal title="Delete Playlist" onclose={() => (confirmDelete = false)}>
		<div class="form">
			<p>Are you sure you want to delete “{playlist.Name}”? This can’t be undone.</p>
			<div class="form-footer">
				<button class="btn secondary" type="button" onclick={() => (confirmDelete = false)}
					>Cancel</button
				>
				<button class="btn danger" type="button" onclick={destroy}>Delete</button>
			</div>
		</div>
	</Modal>
{/if}

<style>
	.page-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		flex-shrink: 0;
		height: 50px;
		padding: 0 25px 0 var(--page-gutter);
		color: hsla(0, 0%, 100%, 0.75);
		font-size: 16px;
	}
	.crumbs {
		display: flex;
		align-items: center;
		min-width: 0;
	}
	.crumb {
		margin-right: 25px;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		line-height: 24px;
	}
	.crumb.link {
		color: hsla(0, 0%, 100%, 0.75);
	}
	.crumb.link:hover {
		color: #fff;
	}
	.toolbar {
		display: flex;
	}
	/* Plex: ToolbarButton (50×50, 20px icons). */
	.tool {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 50px;
		height: 50px;
		color: hsla(0, 0%, 100%, 0.7);
		transition: color 0.2s;
	}
	.tool:hover,
	.tool :global(.trigger:hover) {
		color: #fff;
	}
	.tool :global(.trigger) {
		justify-content: center;
		width: 50px;
		height: 50px;
		color: hsla(0, 0%, 100%, 0.7);
	}

	.page-content {
		flex: 1;
		min-height: 0;
		overflow: hidden auto;
		padding-right: 5px;
	}
	/* Plex: PageContent-innerPageContent. */
	.inner {
		padding: 48px;
	}
	.metadata {
		display: flex;
	}
	.cover {
		position: relative;
		flex-shrink: 0;
		width: 215px;
		height: 215px;
		overflow: hidden;
		border-radius: 4px;
		background-color: rgb(34, 36, 37);
		box-shadow: 0 0 4px rgba(0, 0, 0, 0.3);
	}
	.cover img {
		width: 100%;
		height: 100%;
		object-fit: cover;
	}
	.cover-play {
		position: absolute;
		inset: 0;
		display: grid;
		place-items: center;
		background: radial-gradient(
			farthest-corner at 50% 50%,
			rgba(50, 50, 50, 0.5) 50%,
			#323232 100%
		);
		opacity: 0;
		transition: opacity 0.2s;
	}
	.cover:hover .cover-play {
		opacity: 1;
	}
	.play-circle {
		display: block;
		width: 42px;
		height: 42px;
		border: 2px solid hsla(0, 0%, 100%, 0.7);
		border-radius: 50%;
		background-color: rgba(0, 0, 0, 0.45);
		color: hsla(0, 0%, 100%, 0.7);
		overflow: hidden;
		transition: all 0.2s;
	}
	.play-circle :global(svg) {
		margin: -2px;
	}
	.cover-play:hover .play-circle {
		background-color: var(--color-brand-accent);
		border-color: var(--color-brand-accent);
		color: #1f2326;
	}
	/* Plex: PrePlayMetadataContent. */
	.titles {
		flex: 1;
		min-width: 0;
		margin-left: 48px;
		padding-bottom: 20px;
	}
	.title {
		padding: 0 20px;
		color: #eee;
		font-family: var(--font-heading);
		font-size: 24px;
		line-height: 41.14px;
	}
	.divider,
	.top-divider {
		height: 2px;
		background-color: rgba(0, 0, 0, 0.3);
	}
	.divider {
		margin: 24px 0;
	}
	.overview {
		margin: 0;
		padding: 0 20px;
		color: #eee;
		font-size: 14px;
	}

	/* Plex: PlaylistItemTable. */
	.list {
		margin-top: 45px;
	}
	.list-header {
		display: flex;
		align-items: center;
		height: 60px;
		margin-bottom: 5px;
		padding: 0 20px;
		color: #eee;
		font-family: var(--font-heading);
		font-size: 15px;
		font-weight: 700;
	}
	.top-divider {
		margin-top: 10px;
	}
	/* Plex: PlaylistItemRow. */
	.row {
		position: relative;
		display: flex;
		align-items: center;
		height: 68px;
		padding-right: 20px;
		background-color: rgba(255, 255, 255, 0.02);
	}
	.row.alternate {
		background-color: transparent;
	}
	.row.drag-over {
		box-shadow: inset 0 2px 0 var(--color-brand-accent);
	}
	.move {
		display: flex;
		align-items: center;
		width: 39px;
		height: 100%;
		padding: 0 5px 0 20px;
		color: hsla(0, 0%, 100%, 0.7);
		cursor: grab;
		opacity: 0;
		transition: opacity 0.2s;
	}
	.index {
		position: relative;
		display: grid;
		place-items: center;
		width: 68px;
		color: hsla(0, 0%, 100%, 0.45);
	}
	.row-play {
		position: absolute;
		display: grid;
		place-items: center;
		width: 24px;
		height: 24px;
		border: 2px solid hsla(0, 0%, 100%, 0.7);
		border-radius: 50%;
		color: hsla(0, 0%, 100%, 0.7);
		opacity: 0;
		overflow: hidden;
		transition: all 0.2s;
	}
	.row-play:hover {
		background-color: var(--color-brand-accent);
		border-color: var(--color-brand-accent);
		color: #1f2326;
	}
	.row:hover .move,
	.row:hover .row-play,
	.row:hover .controls {
		opacity: 1;
	}
	.row:hover .number {
		opacity: 0;
	}
	.card {
		flex-shrink: 0;
		display: flex;
		justify-content: center;
		width: 64px;
		margin-right: 14px;
	}
	.card img {
		width: 36px;
		height: 54px;
		object-fit: cover;
		background-color: rgba(255, 255, 255, 0.1);
	}
	.card.wide img {
		width: 64px;
		height: 36px;
	}
	.card.square img {
		width: 54px;
	}
	.text {
		display: flex;
		flex-direction: column;
		flex: 1;
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
		align-self: flex-start;
		max-width: 100%;
		color: #fff;
	}
	.name:hover {
		text-decoration: underline;
	}
	.sub {
		color: hsla(0, 0%, 100%, 0.45);
	}
	.duration {
		flex-shrink: 0;
		padding-left: 20px;
		color: hsla(0, 0%, 100%, 0.45);
		transition: opacity 0.2s;
	}
	.row:hover .duration {
		opacity: 0;
	}
	.controls {
		position: absolute;
		top: 0;
		right: 10px;
		display: flex;
		height: 100%;
		opacity: 0;
		transition: opacity 0.2s;
	}
	.control {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 34px;
		height: 100%;
		color: hsla(0, 0%, 100%, 0.7);
	}
	.control :global(.trigger) {
		justify-content: center;
		width: 34px;
		color: hsla(0, 0%, 100%, 0.7);
	}
	.control:hover,
	.control :global(.trigger:hover) {
		color: #fff;
	}
	/* Plex: Link-danger. */
	.remove {
		color: #f53;
	}
	.remove:hover {
		color: #ff7a5c;
	}

	.form {
		padding: 20px 30px;
	}
	.form label {
		display: block;
		margin-bottom: 6px;
		color: hsla(0, 0%, 100%, 0.45);
		font-size: 13px;
	}
	.form input {
		width: 100%;
		height: 36px;
		padding: 0 12px;
		border: 0;
		border-radius: 4px;
		outline: 0;
		background-color: var(--color-background-control);
		color: #fff;
		font: inherit;
	}
	.form p {
		margin: 0;
		color: #eee;
	}
	.form-footer {
		display: flex;
		justify-content: flex-end;
		gap: 8px;
		margin-top: 20px;
	}
	.btn {
		min-height: 32px;
		padding: 0 12px;
		border-radius: 4px;
		font-size: 14px;
		font-weight: 600;
	}
	.btn.primary {
		background-color: var(--color-background-accent);
		color: var(--color-text-on-accent);
	}
	.btn.secondary {
		background-color: var(--color-background-control);
		color: #fff;
	}
	.btn.danger {
		background-color: var(--color-background-alert);
		color: #fff;
	}
</style>
