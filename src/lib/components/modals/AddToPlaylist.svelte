<script lang="ts">
	import type { BaseItemDto } from '@jellyfin/sdk/lib/generated-client';
	import {
		addToPlaylist,
		createPlaylist,
		loadPlaylists,
		matchingPlaylists
	} from '#lib/actions.svelte.ts';
	import { imageUrl } from '#lib/images.ts';
	import Modal from '../Modal.svelte';

	// Plex Web's AddToCuratedListModal: the playlists that can take the
	// items (60px rows with a 40px thumbnail), then a footer with a name
	// field ("New Playlist", preselected) and Create.
	let { items, onclose }: { items: BaseItemDto[]; onclose: () => void } = $props();

	let modal: Modal | undefined = $state();
	let name = $state('New Playlist');
	let busy = $state(false);
	let input: HTMLInputElement | undefined = $state();
	const playlists = $derived(matchingPlaylists(items));

	$effect(() => {
		loadPlaylists();
	});
	$effect(() => {
		input?.focus();
		input?.select();
	});

	async function add(playlist: BaseItemDto) {
		busy = true;
		try {
			await addToPlaylist(playlist, items);
			modal?.close();
		} finally {
			busy = false;
		}
	}

	async function create(e: SubmitEvent) {
		e.preventDefault();
		if (!name.trim() || busy) return;
		busy = true;
		try {
			await createPlaylist(name.trim(), items);
			modal?.close();
		} finally {
			busy = false;
		}
	}
</script>

<Modal bind:this={modal} title="Add to Playlist" {onclose} bare>
	<!-- Plex: CuratedList-scroller. -->
	<div class="scroller">
		{#each playlists as playlist (playlist.Id)}
			{@const src = imageUrl(playlist, 'square', 40)}
			<div class="row">
				<button class="item" type="button" disabled={busy} onclick={() => add(playlist)}>
					<span class="image"
						>{#if src}<img {src} alt="" />{/if}</span
					>
					<span class="title">{playlist.Name}</span>
				</button>
			</div>
		{/each}
	</div>
	<!-- Plex: ModalFooter + AddToCuratedListModal-form. -->
	<div class="footer">
		<form onsubmit={create}>
			<input bind:this={input} bind:value={name} aria-label="Playlist name" autocomplete="off" />
			<button class="create" type="submit" disabled={busy || !name.trim()}>Create</button>
		</form>
	</div>
</Modal>

<style>
	.scroller {
		height: 400px;
		overflow: hidden auto;
	}
	/* Plex: ListRow + CuratedListItem. */
	.row {
		position: relative;
		height: 60px;
	}
	.item {
		position: absolute;
		inset: 0;
		display: flex;
		align-items: center;
		padding: 0 30px;
		color: hsla(0, 0%, 100%, 0.7);
		transition: color 0.2s;
	}
	.item:hover {
		color: #fff;
	}
	.image {
		flex-shrink: 0;
		width: 40px;
		height: 40px;
		overflow: hidden;
		border-radius: 4px;
		background-color: rgba(0, 0, 0, 0.45);
		box-shadow: 0 0 4px rgba(0, 0, 0, 0.3);
	}
	.image img {
		width: 100%;
		height: 100%;
		object-fit: cover;
	}
	.title {
		min-width: 0;
		max-width: 100%;
		padding: 15px;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.footer {
		padding: 15px 30px;
	}
	form {
		display: flex;
		flex: 1;
	}
	input {
		max-width: 100%;
		height: 30px;
		padding: 2px 10px;
		border: 0;
		border-radius: 4px;
		background-color: #eee;
		color: #555;
		font: inherit;
		font-size: 13px;
	}
	/* Plex: Button primary. */
	.create {
		flex-shrink: 0;
		height: 34px;
		margin-left: 15px;
		padding: 5px 20px;
		border-radius: 4px;
		background-color: var(--color-brand-accent);
		color: #1f2326;
		font-size: 16px;
		font-weight: 600;
	}
	.create:disabled {
		opacity: 0.5;
	}
</style>
