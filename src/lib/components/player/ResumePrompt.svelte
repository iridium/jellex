<script lang="ts">
	import { clock, duration, ticksToSeconds } from '#lib/format.ts';
	import { player } from '#lib/player.svelte.ts';
	import Modal from '../Modal.svelte';

	// Plex Web's "Resume Playback" ListModal. Choosing plays the modal's exit
	// motion first, as Plex does.
	const prompt = $derived(player.resumePrompt!);
	const pos = $derived(ticksToSeconds(prompt.item.UserData?.PlaybackPositionTicks));
	const left = $derived(Math.max(0, ticksToSeconds(prompt.item.RunTimeTicks) - pos));
	let modal: Modal | undefined = $state();
	let first: HTMLButtonElement | undefined = $state();
	let answer: boolean | null = null;
	$effect(() => first?.focus());

	function choose(fromStart: boolean) {
		answer = fromStart;
		modal?.close();
	}
</script>

<Modal bind:this={modal} title="Resume Playback" onclose={() => prompt.resolve(answer)}>
	<div class="list">
		<button class="item" type="button" bind:this={first} onclick={() => choose(false)}>
			Resume from {clock(pos)}<span class="dash">—</span>{duration(left)} left
		</button>
		<button class="item" type="button" onclick={() => choose(true)}>Start from the beginning</button
		>
	</div>
</Modal>

<style>
	/* Plex: ListModal-listModalInnerBody + ListModalItem. */
	.list {
		padding: 20px 0;
	}
	.item {
		display: block;
		width: 100%;
		padding: 10px 30px;
		color: hsla(0, 0%, 100%, 0.7);
		font-size: 15px;
		line-height: 25.71px;
		text-align: left;
		transition: color 0.2s;
	}
	.item:nth-child(odd) {
		background-color: hsla(0, 0%, 100%, 0.02);
	}
	.item:hover {
		color: #fff;
		background-color: hsla(0, 0%, 100%, 0.06);
	}
	.item:focus-visible,
	.item:focus {
		box-shadow: inset 0 0 0 2px var(--color-keyboard-focus);
		outline: none;
	}
	.dash {
		margin: 0 4px;
	}
</style>
