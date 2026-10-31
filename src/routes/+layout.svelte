<script lang="ts">
	import './layout.css';
	import favicon from '#lib/assets/favicon.svg';
	import ModalHost from '#lib/components/modals/ModalHost.svelte';
	import Player from '#lib/components/player/Player.svelte';
	import { session } from '#lib/session.svelte.ts';
	import { applyTheme, settings } from '#lib/settings.svelte.ts';
	import type { LayoutProps } from './$types';

	let { children }: LayoutProps = $props();

	// Settings > Theme.
	$effect(() => applyTheme(settings.value.accent));

	// The app has rendered: drop the boot screen from app.html.
	$effect(() => {
		document.getElementById('preloader')?.remove();
	});
</script>

<svelte:head>
	<link rel="icon" href={favicon} />
	<title>jellex</title>
</svelte:head>

<div class="ultrablur"></div>
{@render children()}
{#if session.signedIn}
	<Player />
	<ModalHost />
{/if}
