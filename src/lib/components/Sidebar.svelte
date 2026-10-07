<script lang="ts">
	import type { BaseItemDto } from '@jellyfin/sdk/lib/generated-client';
	import { tick } from 'svelte';
	import { page } from '$app/state';
	import avatar from '#lib/assets/avatar.svg';
	import type { IconName } from '#lib/icons.ts';
	import { paneSwap } from '#lib/motion.ts';
	import Icon from './Icon.svelte';
	import Menu from './Menu.svelte';

	// Plex Web's SourceSidebar. The main pane lists Home and the pinned
	// libraries, then "More ›". More swaps in the all-sources pane ("‹
	// Pinned", the server, every library with a pin when pinned); the panes
	// slide x ±100% with opacity over 0.2s easeInOut. Hovering a library
	// swaps its pin for a ⋮ menu with Pin or Unpin. Collapsed, the sidebar
	// is a 64px icon rail that expands over the page on hover.
	let {
		views,
		collapsed,
		serverName
	}: { views: BaseItemDto[]; collapsed: boolean; serverName: string } = $props();

	const icons: Record<string, IconName> = {
		movies: 'movies',
		tvshows: 'shows',
		music: 'music',
		musicvideos: 'music',
		homevideos: 'movies'
	};
	const iconFor = (v: BaseItemDto) => icons[v.CollectionType ?? ''] ?? 'movies';

	// Pins are kept per browser. Media libraries start pinned; Jellyfin's
	// Collections and Playlists views (Plex has no such sources) start
	// unpinned, so they only show under More.
	const PIN_KEY = 'jellex.pins';
	const defaultPinned = (v: BaseItemDto) => (v.CollectionType ?? '') in icons;
	function readPins(): Record<string, boolean> {
		try {
			return JSON.parse(localStorage.getItem(PIN_KEY) ?? '{}');
		} catch {
			return {};
		}
	}
	let pins = $state(readPins());
	const isPinned = (v: BaseItemDto) => pins[v.Id!] ?? defaultPinned(v);
	function setPinned(v: BaseItemDto, pinned: boolean) {
		pins = { ...pins, [v.Id!]: pinned };
		try {
			localStorage.setItem(PIN_KEY, JSON.stringify(pins));
		} catch {
			// Not persisted.
		}
	}
	const pinned = $derived(views.filter(isPinned));

	type Pane = 'pinned' | 'all';
	let pane = $state<Pane>('pinned');
	let leaving = $state<Pane | null>(null);
	let panes: Record<Pane, HTMLDivElement | undefined> = $state({
		pinned: undefined,
		all: undefined
	});
	async function show(next: Pane) {
		if (next === pane || leaving) return;
		leaving = pane;
		pane = next;
		await tick();
		// More: the new pane enters from the left and the old one leaves to
		// the right. Back reverses it.
		await paneSwap(panes[next]!, panes[leaving]!, next === 'all' ? '-100%' : '100%');
		leaving = null;
	}

	let hovering = $state(false);
	const path = $derived(page.url.pathname);
	const expanded = $derived(!collapsed || hovering);
	const selected = (v: BaseItemDto) => path.startsWith(`/library/${v.Id}`);
</script>

{#snippet libraryRow(view: BaseItemDto, showPin: boolean)}
	<div class="item" class:selected={selected(view)}>
		<a class="source-link" href="/library/{view.Id}">
			<span class="icon"><Icon name={iconFor(view)} size={24} /></span>
			<span class="title">{view.Name}</span>
		</a>
		<span class="actions">
			<span class="overflow">
				<!-- Plex: SourceSidebarMenu, measured 8px above the trigger's bottom. -->
				<Menu
					variant="plain"
					offset={-8}
					items={[
						isPinned(view)
							? { label: 'Unpin', onselect: () => setPinned(view, false) }
							: { label: 'Pin', onselect: () => setPinned(view, true) }
					]}
				>
					{#snippet trigger()}<Icon name="more-vertical" size={14} label="Actions" />{/snippet}
				</Menu>
			</span>
			{#if showPin && isPinned(view)}
				<span class="pinned-icon"><Icon name="pin" size={16} /></span>
			{/if}
		</span>
	</div>
{/snippet}

<nav
	class="sidebar"
	class:collapsed
	aria-label="Libraries"
	onmouseenter={() => (hovering = true)}
	onmouseleave={() => (hovering = false)}
>
	<div class="frame" class:overlay={collapsed && hovering} class:narrow={!expanded}>
		{#if pane === 'pinned' || leaving === 'pinned'}
			<div class="pane no-scrollbar" bind:this={panes.pinned}>
				<a class="source-link home" class:selected={path === '/'} href="/">
					<span class="icon"><Icon name="home" size={24} /></span>
					<span class="title">Home</span>
				</a>
				{#each pinned as view (view.Id)}
					{@render libraryRow(view, false)}
				{/each}
				<button class="toggle" type="button" onclick={() => show('all')}>
					<span class="toggle-title">More</span>
					<span class="toggle-icon more"><Icon name="chevron-right" size={24} /></span>
				</button>
			</div>
		{/if}
		{#if pane === 'all' || leaving === 'all'}
			<div class="pane no-scrollbar" bind:this={panes.all}>
				<button class="toggle" type="button" onclick={() => show('pinned')}>
					<span class="toggle-icon back"><Icon name="chevron-left" size={24} /></span>
					<span class="toggle-title">Pinned</span>
				</button>
				<!-- Plex: AllSourcesSidebarContent. -->
				<div class="server">
					<a class="source-link medium" href="/">
						<span class="icon"><img class="avatar" src={avatar} alt="" /></span>
						<span class="title server-title">{serverName}</span>
					</a>
					{#each views as view (view.Id)}
						{@render libraryRow(view, true)}
					{/each}
				</div>
			</div>
		{/if}
	</div>
</nav>

<style>
	/* Plex: SourceSidebar-openSidebar / -collapsedSidebar. */
	.sidebar {
		position: relative;
		z-index: 1;
		flex-shrink: 0;
		width: var(--sidebar-width);
		min-width: var(--sidebar-width);
		margin: 0 0 var(--size-xs) var(--size-xs);
		transition: min-width 0.2s cubic-bezier(0.4, 0, 0.2, 1);
	}
	.sidebar.collapsed {
		width: 64px;
		min-width: 64px;
	}
	/* Plex: SourceSidebar-sidebar. */
	.frame {
		position: absolute;
		inset: 0 auto 0 0;
		width: var(--sidebar-width);
		max-width: var(--sidebar-width);
		overflow: hidden;
		border-radius: var(--border-radius-s);
		background-color: rgba(0, 0, 0, 0.15);
		transition:
			max-width 0.2s cubic-bezier(0.4, 0, 0.2, 1),
			background-color 0.2s ease-in;
	}
	.frame.narrow {
		max-width: 64px;
		background-color: transparent;
		transition:
			max-width 0.2s cubic-bezier(0.4, 0, 0.2, 1),
			background-color 1s ease-out;
	}
	/* Plex: SourceSidebar-expandedSidebar, the hover overlay when collapsed. */
	.frame.overlay {
		background-color: #1f2326;
		box-shadow: var(--shadow-high);
	}
	/* Plex: SourceSidebar-pane. */
	.pane {
		position: absolute;
		inset: 0;
		width: var(--sidebar-width);
		overflow: hidden auto;
	}

	/* Plex: SourceSidebarItem + SourceSidebarLink (small 40px, medium 50px). */
	.item {
		position: relative;
		display: flex;
		align-items: stretch;
	}
	.source-link {
		position: relative;
		display: flex;
		align-items: center;
		flex: 1 1 auto;
		min-width: 0;
		height: 40px;
		min-height: 40px;
		color: hsla(0, 0%, 100%, 0.7);
		transition: color 0.2s;
	}
	.source-link.medium {
		height: auto;
		min-height: 50px;
	}
	.icon {
		display: flex;
		align-items: center;
		justify-content: center;
		flex-shrink: 0;
		height: 100%;
		padding: 0 var(--size-m);
		transition: color 0.2s;
	}
	.title {
		margin-right: var(--size-m);
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		color: hsla(0, 0%, 100%, 0.75);
		font-size: 15px;
		line-height: 1.5;
		transition: color 0.2s;
	}
	.source-link:hover .icon,
	.source-link:hover .title,
	.item:hover .icon,
	.item:hover .title {
		color: #fff;
	}
	.source-link.selected .icon,
	.source-link.selected .title,
	.item.selected .icon,
	.item.selected .title {
		color: var(--color-text-accent);
	}
	.source-link.selected::before,
	.item.selected::before {
		content: '';
		position: absolute;
		top: 2px;
		bottom: 2px;
		left: 0;
		width: 2px;
		border-radius: 4px;
		background-color: var(--color-text-accent);
	}
	.source-link.selected:hover::before,
	.item.selected:hover::before {
		background-color: #fff;
	}

	/* Plex: SourceSidebarItem-actionContainer: the pin, or ⋮ on hover. */
	.actions {
		display: flex;
		align-items: stretch;
		flex-shrink: 0;
	}
	.pinned-icon {
		display: flex;
		align-items: center;
		margin: 0 10px;
		color: hsla(0, 0%, 100%, 0.45);
	}
	.overflow {
		display: none;
	}
	.item:hover .overflow,
	.overflow:has(:global(.trigger.open)) {
		display: flex;
	}
	.item:hover .pinned-icon,
	.overflow:has(:global(.trigger.open)) + .pinned-icon {
		display: none;
	}
	.overflow :global(.trigger) {
		justify-content: center;
		width: 34px;
		height: 40px;
		color: #fff;
	}

	/* Plex: AllSourcesSidebarContent + SourceSidebarServerHeader. */
	.server {
		padding-bottom: 30px;
	}
	.avatar {
		display: block;
		width: 24px;
		height: 24px;
		border-radius: 50%;
	}
	.server-title,
	.source-link:hover .server-title {
		color: var(--color-accent-dark);
	}

	/* Plex: SourcesToggleButton. */
	.toggle {
		display: inline-flex;
		align-items: center;
		min-width: var(--sidebar-width);
		min-height: 50px;
		padding: 0 0 0 var(--size-s);
		color: hsla(0, 0%, 100%, 0.75);
		font-size: 15px;
		text-transform: capitalize;
		transition: color 0.2s;
	}
	.toggle:hover {
		color: #fff;
	}
	.toggle-icon {
		display: flex;
	}
	.toggle-icon.more {
		margin-left: var(--size-xxs);
	}
	.toggle-icon.back {
		margin-right: var(--size-xxs);
	}
	.narrow .title {
		opacity: 0;
	}
	/* Plex drops the toggle and actions from the icon rail; they return on hover. */
	.narrow .toggle,
	.narrow .actions {
		visibility: hidden;
	}
</style>
