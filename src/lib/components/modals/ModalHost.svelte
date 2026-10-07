<script lang="ts">
	import { modals } from '#lib/modals.svelte.ts';
	import MediaInfo from '../player/MediaInfo.svelte';
	import AddToPlaylist from './AddToPlaylist.svelte';

	const m = $derived(modals.current);
</script>

{#if m?.kind === 'info'}
	<MediaInfo item={m.item} onclose={() => modals.close()} />
{:else if m?.kind === 'add-to-playlist'}
	<AddToPlaylist items={m.items} onclose={() => modals.close()} />
{/if}
