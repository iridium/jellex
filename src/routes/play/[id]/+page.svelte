<script lang="ts">
	import { goto } from '$app/navigation';
	import { player } from '#lib/player.svelte.ts';
	import type { PageProps } from './$types';

	// A deep link into the player: starts playback in the app-wide player,
	// then shows the item's page underneath it (as Plex does).
	let { data }: PageProps = $props();

	$effect(() => {
		const d = data;
		player.play(d.item, {
			queue: d.queue,
			shuffle: d.shuffle,
			fromStart: d.fromStart,
			audio: d.audio,
			subtitle: d.subtitle
		});
		goto(`/items/${d.item.Id}`, { replace: true });
	});
</script>
