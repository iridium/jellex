<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import Hub from '#lib/components/Hub.svelte';
	import Icon from '#lib/components/Icon.svelte';
	import { player } from '#lib/player.svelte.ts';
	import { addToEntries, loadPlaylists } from '#lib/actions.svelte.ts';
	import { resolveQueue } from '#lib/playback.ts';
	import type { BaseItemDto } from '@jellyfin/sdk/lib/generated-client';
	import { selection } from '#lib/selection.svelte.ts';
	import { listStyles } from '#lib/ui.svelte.ts';
	import Menu from '#lib/components/Menu.svelte';
	import MultiselectBar from '#lib/components/MultiselectBar.svelte';
	import PageHeader from '#lib/components/PageHeader.svelte';
	import PosterCard from '#lib/components/PosterCard.svelte';
	import Spinner from '#lib/components/Spinner.svelte';
	import VirtualGrid from '#lib/components/VirtualGrid.svelte';
	import {
		fetchGrid,
		filters,
		filterValues,
		filtersFor,
		gridSpec,
		sortsFor,
		typesFor,
		hasCollections,
		hasRecommended,
		indexOfLetter,
		sorts,
		type GridQuery,
		type Pivot
	} from '#lib/library.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	const view = $derived(data.view);
	const base = $derived(`/library/${view.Id}`);
	const tabs = $derived(
		[
			hasRecommended(view.CollectionType) && { pivot: 'recommended', label: 'Recommended' },
			{ pivot: 'library', label: 'Library' },
			hasCollections(view.CollectionType) && { pivot: 'collections', label: 'Collections' }
		].filter(Boolean) as { pivot: Pivot; label: string }[]
	);

	const query: GridQuery = $derived({
		parentId: view.Id!,
		spec: gridSpec(view.CollectionType, data.pivot, data.type),
		sort: data.sort,
		desc: data.desc,
		filter: data.filter,
		genre: data.genre,
		personId: data.personId,
		year: data.year,
		decade: data.decade,
		rating: data.rating
	});
	// Plex's hub list holds the hub's first 200 items.
	const HUB_LIMIT = 200;
	// A new function per query tells the grid to start over.
	const fetch = $derived.by(() => {
		const q = query;
		if (!data.hub) return (start: number, limit: number) => fetchGrid(q, start, limit);
		return async (start: number, limit: number) => {
			const n = Math.max(0, Math.min(limit, HUB_LIMIT - start));
			const page = n ? await fetchGrid(q, start, n) : { items: [], total: 0 };
			return { items: page.items, total: Math.min(page.total, HUB_LIMIT) };
		};
	});
	// What the toolbar's Play, Shuffle, Add to and More act on: the library,
	// or the hub's items.
	let hubItems = $state<BaseItemDto[]>([]);
	$effect(() => {
		const q = query;
		if (!data.hub) return;
		fetchGrid(q, 0, HUB_LIMIT).then((r) => (hubItems = r.items));
	});
	const targets = $derived(data.hub ? hubItems : [view]);
	// Folders (series, albums, the library itself) expand to what they play.
	async function playable(items: BaseItemDto[]) {
		return (
			await Promise.all(items.map((i) => (i.IsFolder || !i.MediaType ? resolveQueue(i) : [i])))
		).flat();
	}
	async function playAll(shuffle = false) {
		if (!data.hub) return player.play(view, { shuffle });
		const queue = await playable(hubItems);
		if (!queue.length) return;
		if (shuffle)
			for (let i = queue.length - 1; i > 0; i--) {
				const j = Math.floor(Math.random() * (i + 1));
				[queue[i], queue[j]] = [queue[j], queue[i]];
			}
		player.play(queue[0], { queue });
	}
	async function queueAll(next: boolean) {
		player.addToQueue(await playable(targets), next);
	}

	let total = $state<number | null>(null);
	// Plex always lists tracks as a table, with no style or size controls.
	const tracks = $derived(data.type === 'tracks');
	const listStyle = $derived(tracks ? 'table' : listStyles.get(view.Id!));
	const types = $derived(
		data.pivot === 'library' && !data.hub ? typesFor(view.CollectionType) : undefined
	);
	const typeDef = $derived(types?.find((t) => t.key === data.type));
	const typeSorts = $derived(sortsFor(data.type));
	const filterSet = $derived(filtersFor(view.CollectionType, data.type));
	let scroller: HTMLDivElement | undefined = $state();
	let grid: VirtualGrid | undefined = $state();

	const sortDef = $derived(sorts.find((s) => s.key === data.sort) ?? sorts[0]);
	const filterDef = $derived(filters.find((f) => f.key === data.filter)!);
	const showJumpBar = $derived(data.sort === 'SortName' && !data.desc);
	const letters = ['#', ...'ABCDEFGHIJKLMNOPQRSTUVWXYZ'];

	function update(changes: Record<string, string | undefined>) {
		const q = new URLSearchParams(page.url.searchParams.toString());
		for (const [k, v] of Object.entries(changes)) {
			if (v == null) q.delete(k);
			else q.set(k, v);
		}
		goto(`?${q}`, { replace: true, reset: false });
	}

	// Plex's filter menu. One filter applies at a time; the trigger shows
	// its value, and Clear Filter (left of the toolbar) removes it.
	const music = $derived(view.CollectionType === 'music');
	const cleared = {
		filter: undefined,
		genre: undefined,
		year: undefined,
		decade: undefined,
		rating: undefined,
		person: undefined,
		personName: undefined
	};
	const filterLabel = $derived(
		data.genre ??
			(data.year != null ? String(data.year) : undefined) ??
			(data.decade != null ? `${data.decade}s` : undefined) ??
			data.rating ??
			data.personName ??
			filterDef.label
	);
	const filtered = $derived(
		data.filter !== 'all' ||
			!!(data.genre || data.year || data.decade || data.rating || data.personId)
	);
	let values: ReturnType<typeof filterValues> | undefined;
	const valuesFor = () => (values ??= filterValues(query));
	$effect(() => {
		view.Id;
		values = undefined;
	});
	const drills = {
		genre: {
			label: 'Genre',
			values: async () =>
				(await valuesFor()).genres.map((g) => [g, g === data.genre, { genre: g }] as const)
		},
		year: {
			label: 'Year',
			values: async () =>
				(await valuesFor()).years.map(
					(y) => [String(y), y === data.year, { year: String(y) }] as const
				)
		},
		decade: {
			label: 'Decade',
			values: async () =>
				(await valuesFor()).decades.map(
					(d) => [`${d}s`, d === data.decade, { decade: String(d) }] as const
				)
		},
		rating: {
			label: 'Content Rating',
			values: async () =>
				(await valuesFor()).ratings.map((r) => [r, r === data.rating, { rating: r }] as const)
		}
	};
	const filterMenu = $derived(
		filterSet
			? [
					...filters
						.filter((f) => filterSet.unplayed || f.key !== 'unwatched')
						.map((f) => ({
							label: f.label,
							active: f.key === data.filter && filterLabel === f.label,
							onselect: () => update({ ...cleared, filter: f.key === 'all' ? undefined : f.key })
						})),
					// Plex separates Unplayed from the drill-ins; music has none.
					...(filterSet.unplayed ? [{ separator: true as const }] : []),
					...filterSet.categories.map((c) => ({
						label: drills[c].label,
						drill: {
							title: drills[c].label,
							placeholder: `Filter ${drills[c].label}`,
							load: async () =>
								(await drills[c].values()).map(([label, active, change]) => ({
									label,
									active,
									onselect: () => update({ ...cleared, ...change })
								}))
						}
					}))
				]
			: []
	);
	function chooseType(key: string) {
		// A new type starts over: its own default sort, no filter.
		update({
			...cleared,
			type: key === types?.[0].key ? undefined : key,
			sort: undefined,
			order: undefined
		});
	}

	function chooseSort(key: string, defaultDesc: boolean) {
		// Choosing the current sort again flips its direction, as in Plex.
		const desc = key === data.sort ? !data.desc : defaultDesc;
		update({ sort: key, order: desc ? 'desc' : 'asc' });
	}

	const libraryMenu = $derived([
		{ label: 'Play', onselect: () => player.play(view) },
		{ label: 'Shuffle', onselect: () => player.play(view, { shuffle: true }) }
	]);

	async function jump(letter: string) {
		const index = await indexOfLetter(query, letter);
		grid?.jumpTo(index);
	}
</script>

{#snippet gridArea()}
	<div class="grid-row">
		<div class="page-content scroller grid-scroller" bind:this={scroller}>
			{#if scroller}
				{#if listStyle === 'table'}
					<!-- Plex: DirectoryListTableHeader. -->
					<div class="table-header">
						<span class="columns-button" aria-label="Change Columns"
							><Icon name="columns" size={16} /></span
						>
						<button
							class="column active"
							type="button"
							onclick={() =>
								update({
									sort: 'SortName',
									order: data.sort === 'SortName' && !data.desc ? 'desc' : 'asc'
								})}
						>
							<span>Title</span>
							{#if data.sort === 'SortName'}<span class="sort-arrow">{data.desc ? '↑' : '↓'}</span
								>{/if}
						</button>
						<span class="column actions-column"></span>
					</div>
				{/if}
				<div class="grid-pad" class:rows-pad={listStyle !== 'grid'}>
					<VirtualGrid
						bind:this={grid}
						{scroller}
						kind={query.spec.kind}
						{fetch}
						bind:total
						layout={listStyle}
						addedLine={!!data.hub}
					/>
				</div>
				{#if total === 0}
					<p class="empty">No items match.</p>
				{/if}
			{/if}
		</div>
		{#if showJumpBar && total}
			<nav class="jump-bar" aria-label="Jump to letter">
				{#each letters as letter (letter)}
					<button class="character" type="button" onclick={() => jump(letter)}>{letter}</button>
				{/each}
			</nav>
		{/if}
	</div>
{/snippet}

<svelte:head>
	<title>{view.Name} · jellex</title>
</svelte:head>

{#if data.hub}
	<PageHeader
		title={data.serverName}
		href="/"
		crumb="Recently Added {view.Name}"
		{listStyle}
		onListStyle={(s) => listStyles.set(view.Id!, s)}
	/>
{:else}
	<PageHeader
		title={view.Name ?? ''}
		detail={data.serverName}
		href={base}
		slider={!tracks}
		menu={libraryMenu}
		listStyle={data.hubs || tracks ? undefined : listStyle}
		onListStyle={tracks ? undefined : (s) => listStyles.set(view.Id!, s)}
	>
		{#snippet center()}
			<nav class="tabs">
				{#each tabs as tab (tab.pivot)}
					<a
						class="tab"
						class:selected={data.pivot === tab.pivot}
						href="{base}?pivot={tab.pivot}"
						aria-current={data.pivot === tab.pivot ? 'page' : undefined}>{tab.label}</a
					>
				{/each}
			</nav>
		{/snippet}
	</PageHeader>
{/if}

{#if data.hubs}
	{#await data.hubs}
		<div class="loading"><Spinner /></div>
	{:then hubs}
		{#if selection.count}<MultiselectBar />{/if}
		<div class="page-content scroller recommended">
			{#each hubs as hub (hub.title)}
				<Hub
					title={hub.title}
					href={hub.title.startsWith('Recently Added') ? `${base}?hub=recent` : undefined}
				>
					{#each hub.items as item (item.Id)}
						<PosterCard
							{item}
							kind={hub.kind}
							continueWatching={hub.title === 'Continue Watching'}
						/>
					{/each}
				</Hub>
			{/each}
			{#if hubs.length === 0}
				<p class="empty">Nothing to recommend yet.</p>
			{/if}
		</div>
	{/await}
{:else}
	{#if selection.count}
		<MultiselectBar />
	{:else}
		<div class="toolbar">
			<div class="toolbar-left">
				{#if filtered}
					<!-- Plex: DirectoryListToolbar's Clear Filter, in the gutter. -->
					<button
						class="clear-filter"
						type="button"
						aria-label="Clear Filter"
						onclick={() => update(cleared)}
					>
						<Icon name="clear-filter" size={16} />
					</button>
				{/if}
				{#if filterSet}
					<Menu label={filterLabel} arrow="large" size="large" items={filterMenu} />
				{/if}
				{#if types && typeDef}
					<!-- Plex: the type menu (Shows / Seasons / Episodes, …). -->
					<Menu
						label={typeDef.label}
						arrow="large"
						items={types.map((t) => ({
							label: t.label,
							active: t.key === data.type,
							onselect: () => chooseType(t.key)
						}))}
					/>
				{/if}
				{#if listStyle !== 'table' && typeSorts.length}
					<Menu
						label="By {sortDef.label}"
						arrow="large"
						items={typeSorts.map((s) => ({
							label: s.label,
							active: s.key === data.sort,
							selectedIcon:
								s.key === 'Random'
									? undefined
									: data.desc
										? ('sort-descending' as const)
										: ('sort-ascending' as const),
							onselect: () => chooseSort(s.key, s.desc)
						}))}
					/>
				{/if}
				{#if total != null}
					<span class="badge">{total}</span>
				{/if}
			</div>
			<!-- Plex: ActionButtonBar. -->
			<div class="action-bar">
				<button
					class="action-button"
					type="button"
					aria-label="Play"
					title="Play"
					onclick={() => playAll()}
				>
					<Icon name="play-solid" size={24} />
				</button>
				<button
					class="action-button"
					type="button"
					aria-label="Shuffle"
					title="Shuffle"
					onclick={() => playAll(true)}
				>
					<Icon name="shuffle" size={24} />
				</button>
				<span class="action-menu">
					<Menu
						variant="plain"
						align="right"
						offset={16}
						items={addToEntries(targets)}
						onopen={loadPlaylists}
					>
						{#snippet trigger()}<Icon name="add-to" size={24} label="Add to..." />{/snippet}
					</Menu>
				</span>
				<span class="action-menu">
					<Menu
						variant="plain"
						align="right"
						offset={16}
						items={[
							{ label: 'Play Next', onselect: () => queueAll(true) },
							{ label: 'Add to Queue', onselect: () => queueAll(false) }
						]}
					>
						{#snippet trigger()}<Icon name="more-horizontal" size={24} label="More" />{/snippet}
					</Menu>
				</span>
			</div>
		</div>
	{/if}
	{@render gridArea()}
{/if}

<style>
	/* Plex: PivotTabsPageHeaderCenter + TabButton. */
	.tabs {
		display: flex;
		height: 40px;
	}
	.tab {
		position: relative;
		max-width: 250px;
		margin: 0 5px;
		padding: 0 16px;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		border-radius: 4px;
		color: hsla(0, 0%, 100%, 0.45);
		font-size: 14px;
		font-weight: 600;
		line-height: 40px;
		transition:
			background-color 0.2s,
			color 0.2s;
	}
	.tab:hover {
		color: #fff;
	}
	.tab.selected {
		color: var(--color-brand-accent);
	}
	.tab.selected::after {
		content: '';
		position: absolute;
		left: 0;
		right: 0;
		bottom: 4px;
		height: 2px;
		border-radius: 1px;
		background-color: var(--color-brand-accent);
	}
	.tab.selected:hover::after {
		background-color: #fff;
		transition: background-color 0.2s;
	}

	.page-content {
		flex: 1;
		min-height: 0;
		overflow: hidden auto;
	}
	.recommended {
		padding-top: 16px;
	}
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

	/* Plex: DirectoryListToolbar. */
	.toolbar {
		display: flex;
		align-items: center;
		justify-content: space-between;
		flex-shrink: 0;
		height: 50px;
		padding: 0 var(--page-gutter);
		color: hsla(0, 0%, 100%, 0.75);
		font-size: 16px;
		line-height: 1.71428571;
	}
	.toolbar-left {
		display: flex;
		align-items: center;
		height: 100%;
		gap: 15px;
		font-size: 15px;
	}
	/* Plex: PageHeaderBadge. */
	.badge {
		padding: 0 8px;
		border-radius: 4px;
		background-color: rgba(0, 0, 0, 0.15);
		color: hsla(0, 0%, 100%, 0.75);
		font-size: 16px;
		line-height: 24px;
	}

	.action-bar {
		display: flex;
		gap: 8px;
	}
	.action-button {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		min-width: 56px;
		min-height: 40px;
		padding: 0 16px;
		border-radius: 4px;
		color: #fff;
		transition: all var(--duration-fast);
	}
	.action-button:hover {
		background-color: var(--color-background-control-focus);
	}
	/* Plex: Clear Filter, 40×32 left of the filter menu. */
	.toolbar-left {
		position: relative;
	}
	.clear-filter {
		position: absolute;
		left: -40px;
		top: 50%;
		display: flex;
		align-items: center;
		height: 32px;
		padding: 0 12px;
		transform: translateY(-50%);
		color: hsla(0, 0%, 100%, 0.8);
		transition: color 0.2s;
	}
	.clear-filter:hover {
		color: #fff;
	}
	.action-menu :global(.trigger) {
		justify-content: center;
		min-width: 56px;
		min-height: 40px;
		padding: 0 16px;
		border-radius: 4px;
		color: #fff;
		transition: all var(--duration-fast);
	}
	.action-menu :global(.trigger:hover),
	.action-menu :global(.trigger.open) {
		background-color: var(--color-background-control-focus);
	}

	.grid-row {
		display: flex;
		flex: 1;
		min-height: 0;
	}
	.grid-pad {
		padding: 16px 0 40px var(--page-gutter);
	}
	.grid-pad.rows-pad {
		padding-top: 0;
	}
	/* Plex: DirectoryListTableHeader. */
	.table-header {
		position: sticky;
		top: 0;
		z-index: 2;
		display: flex;
		height: 40px;
		background-color: rgb(30, 37, 47);
		color: #fff;
	}
	.columns-button {
		display: grid;
		place-items: center;
		width: 40px;
		color: rgba(255, 255, 255, 0.8);
	}
	.column {
		display: flex;
		align-items: center;
		height: 40px;
		font-size: 13px;
	}
	.column.active {
		flex: 1;
		justify-content: space-between;
		padding: 0 20px 0 62px;
		background-color: rgb(33, 41, 52);
		color: #fff;
		text-align: left;
	}
	.actions-column {
		width: 82px;
	}
	.sort-arrow {
		font-size: 14px;
	}
	/* Plex: DirectoryListJumpBar. */
	.jump-bar {
		display: flex;
		flex-flow: column;
		justify-content: space-around;
		flex-shrink: 0;
		margin: 10px 0;
		padding: 0 10px;
		overflow: hidden;
		font-size: 13px;
	}
	.character {
		height: 25px;
		color: hsla(0, 0%, 100%, 0.45);
		text-align: center;
		transition: color 0.2s;
	}
	.character:hover {
		color: #fff;
	}
</style>
