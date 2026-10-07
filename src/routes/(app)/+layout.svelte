<script lang="ts">
	import NavBar from '#lib/components/NavBar.svelte';
	import Sidebar from '#lib/components/Sidebar.svelte';
	import SettingsSidebar from '#lib/components/SettingsSidebar.svelte';
	import { navigating, page } from '$app/state';
	import Spinner from '#lib/components/Spinner.svelte';
	import type { LayoutProps } from './$types';

	let { data, children }: LayoutProps = $props();

	const KEY = 'jellex.sidebarCollapsed';
	function load(): boolean {
		try {
			return localStorage.getItem(KEY) === '1';
		} catch {
			return false;
		}
	}
	let collapsed = $state(load());
	// Plex swaps in the settings sidebar (and a Home button) on settings.
	const inSettings = $derived(page.url.pathname.startsWith('/settings'));
	// Plex hides the page's content behind a spinner while the next page
	// loads (the header stays). Query-only changes (filters, sorts) don't.
	const loading = $derived(!!navigating.to && navigating.to.url.pathname !== page.url.pathname);
	function toggle() {
		collapsed = !collapsed;
		try {
			localStorage.setItem(KEY, collapsed ? '1' : '0');
		} catch {
			// Not persisted; fine.
		}
	}
</script>

<div class="app">
	<NavBar
		sidebarCollapsed={collapsed}
		serverName={data.serverName}
		onToggleSidebar={toggle}
		home={inSettings}
	/>
	<div class="body">
		{#if inSettings}
			<SettingsSidebar />
		{:else}
			<Sidebar views={data.views} {collapsed} serverName={data.serverName} />
		{/if}
		<main class="page" class:loading>
			{@render children()}
			{#if loading}<div class="page-spinner"><Spinner /></div>{/if}
		</main>
	</div>
</div>

<style>
	.app {
		display: flex;
		flex-direction: column;
		/* The mini player takes the bottom 100px when open. */
		height: calc(100% - var(--mini-player-height, 0px));
	}
	.body {
		display: flex;
		flex: 1;
		min-height: 0;
	}
	/* Plex: Page-pageScroller, which runs to the window's right edge. */
	.page {
		position: relative;
		display: flex;
		flex-direction: column;
		flex: 1;
		min-width: 0;
		font-size: 13px;
		line-height: 1.71428571;
	}
	.page.loading :global(.page-content) {
		visibility: hidden;
	}
	.page-spinner {
		position: absolute;
		inset: 0;
		display: grid;
		place-items: center;
		pointer-events: none;
	}
</style>
