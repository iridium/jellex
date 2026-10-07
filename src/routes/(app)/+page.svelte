<script lang="ts">
	import Hub from '#lib/components/Hub.svelte';
	import PosterCard from '#lib/components/PosterCard.svelte';
	import PageHeader from '#lib/components/PageHeader.svelte';
	import Spinner from '#lib/components/Spinner.svelte';
	import MultiselectBar from '#lib/components/MultiselectBar.svelte';
	import { selection } from '#lib/selection.svelte.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
</script>

<svelte:head>
	<title>Home · jellex</title>
</svelte:head>

<!-- Plex swaps the page header for the selection bar while selecting. -->
{#if selection.count}
	<MultiselectBar />
{:else}
	<PageHeader title="Home" slider />
{/if}
{#await data.hubs}
	<div class="loading"><Spinner /></div>
{:then hubs}
	<div class="page-content scroller">
		{#each hubs as hub (hub.title)}
			<Hub title={hub.title} href={hub.href}>
				{#each hub.items as item (item.Id)}
					<PosterCard {item} kind={hub.kind} continueWatching={hub.title === 'Continue Watching'} />
				{/each}
			</Hub>
		{/each}
		{#if hubs.length === 0}
			<p class="empty">Nothing here yet. Add some media to Jellyfin.</p>
		{/if}
	</div>
{:catch}
	<p class="empty">Couldn't load your home screen from Jellyfin.</p>
{/await}

<style>
	/* Plex: PageContent-pageContentScroller (Scroller-vertical). */
	.page-content {
		flex: 1;
		min-height: 0;
		overflow: hidden scroll;
	}
	/* Plex centres the spinner in the whole page, header included. */
	.loading {
		position: absolute;
		inset: 0;
		display: grid;
		place-items: center;
		pointer-events: none;
	}
	.empty {
		padding: 0 var(--page-gutter);
		color: hsla(0, 0%, 100%, 0.45);
	}
</style>
