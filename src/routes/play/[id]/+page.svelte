<script lang="ts">
	import { untrack } from 'svelte';
	import { goto } from '$app/navigation';
	import { player } from '#lib/player.svelte.ts';
	import type { PageProps } from './$types';

	// A deep link into the player: starts playback in the app-wide player,
	// then shows the item's page underneath it (as Plex does).
	let { data }: PageProps = $props();

	// Runs once per link: play() reads player state, which must not make
	// the effect re-run (and start playback again) whenever that changes.
	$effect(() => {
		const d = data;
		untrack(() => {
			player.play(d.item, {
				queue: d.queue,
				shuffle: d.shuffle,
				fromStart: d.fromStart,
				audio: d.audio,
				subtitle: d.subtitle
			});
			goto(`/items/${d.item.Id}`, { replace: true });
		});
	});
</script>
