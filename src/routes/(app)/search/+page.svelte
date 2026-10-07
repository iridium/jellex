<script lang="ts">
	import Icon from '#lib/components/Icon.svelte';
	import SearchRow from '#lib/components/search/SearchRow.svelte';
	import Spinner from '#lib/components/Spinner.svelte';
	import { tabs } from '#lib/search.ts';
	import { selection } from '#lib/selection.svelte.ts';
	import MultiselectBar from '#lib/components/MultiselectBar.svelte';
	import type { PageProps } from './$types';

	// Plex Web's search results page: a heading, category chips, then rows.
	let { data }: PageProps = $props();
	const tabLabel = $derived(tabs.find((t) => t.key === data.pivot)!.label);
	const heading = $derived(
		data.pivot === 'top'
			? `Top Results for "${data.query}"`
			: `Search results for "${data.query}" in ${tabLabel}`
	);
</script>

<svelte:head>
	<title>{data.query} · Search · jellex</title>
</svelte:head>

{#if selection.count}<MultiselectBar />{/if}
<div class="page-content scroller">
	<div class="header-block">
		<!-- Plex: PrimaryPageHeader. -->
		<div class="page-header"><h1>{heading}</h1></div>
		<!-- Plex: SearchResultsPageHeader-chipsContainer. -->
		<ul class="chips">
			{#each tabs as tab (tab.key)}
				<li>
					<a
						class="chip"
						class:selected={tab.key === data.pivot}
						href="/search?pivot={tab.key}&query={encodeURIComponent(data.query)}"
						aria-current={tab.key === data.pivot ? 'page' : undefined}>{tab.label}</a
					>
				</li>
			{/each}
		</ul>
	</div>

	{#await data.results}
		<div class="state"><Spinner /></div>
	{:then results}
		{#if results.length}
			<div class="rows">
				{#each results as item (item.Id)}
					<SearchRow {item} serverName={data.serverName} wide />
				{/each}
			</div>
		{:else}
			<!-- Plex's empty search state. -->
			<div class="empty">
				<span class="empty-icon"><Icon name="search" size={48} /></span>
				<h2>No Results Found</h2>
				<p>Try adjusting your search term or filter to find what you’re looking for.</p>
			</div>
		{/if}
	{/await}
</div>

<style>
	.page-content {
		flex: 1;
		min-height: 0;
		overflow: hidden auto;
	}
	/* Plex: the header and chips stack with a 16px gap. */
	.header-block {
		display: flex;
		flex-direction: column;
		gap: 16px;
		padding: 16px 0;
	}
	.page-header {
		display: flex;
		align-items: center;
		height: 50px;
		padding: 0 25px 0 var(--page-gutter);
	}
	h1 {
		margin: 0;
		color: rgba(255, 255, 255, 0.8);
		font-family: var(--font-heading-2-font-family);
		font-size: var(--font-heading-2-font-size);
		font-weight: 700;
		line-height: 32px;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.chips {
		display: flex;
		flex-wrap: wrap;
		gap: 8px;
		align-items: flex-start;
		height: 44px;
		margin: 0 0 16px var(--page-gutter);
		padding: 2px 0 2px 1px;
		list-style: none;
	}
	.chip {
		display: inline-flex;
		align-items: center;
		height: 28px;
		padding: 0 12px;
		border-radius: 9999px;
		background-color: rgba(255, 255, 255, 0.1);
		color: #fff;
		font-size: 14px;
		font-weight: 600;
		line-height: 20px;
		transition: background-color var(--duration-fast);
	}
	.chip:hover {
		background-color: rgba(255, 255, 255, 0.18);
	}
	.chip.selected {
		color: var(--color-brand-accent);
	}
	.rows {
		padding: 0 40px 40px 40px;
	}
	.state {
		display: grid;
		place-items: center;
		padding: 80px 0;
	}
	.empty {
		display: flex;
		flex-direction: column;
		align-items: center;
		padding-top: 180px;
		text-align: center;
	}
	.empty-icon {
		display: grid;
		place-items: center;
		width: 124px;
		height: 124px;
		margin-bottom: 40px;
		border-radius: 50%;
		background-color: #191a1c;
		color: var(--color-brand-accent);
	}
	.empty h2 {
		margin: 0 0 32px;
		color: #fff;
		font-family: var(--font-heading-2-font-family);
		font-size: var(--font-heading-2-font-size);
		font-weight: 700;
		line-height: 32px;
	}
	.empty p {
		margin: 0;
		color: rgba(255, 255, 255, 0.8);
		font-size: 16px;
	}
</style>
