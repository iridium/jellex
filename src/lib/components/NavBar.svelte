<script lang="ts">
	import { goto } from '$app/navigation';
	import Wordmark from '#lib/components/Wordmark.svelte';
	import Icon from './Icon.svelte';
	import Menu from './Menu.svelte';
	import SearchBox from './search/SearchBox.svelte';
	import { player } from '#lib/player.svelte.ts';
	import { session } from '#lib/session.svelte.ts';

	// Plex Web's NavBar. Sizes are measured from the real client: an 8px
	// inset, 48px tall bar; 56×40 icon buttons; the wordmark at 24px tall in
	// a 16px-padded link; a 480px search pill.
	let {
		sidebarCollapsed,
		serverName,
		onToggleSidebar,
		home = false
	}: {
		sidebarCollapsed: boolean;
		serverName: string;
		onToggleSidebar: () => void;
		/** Settings pages show Home in place of the sidebar toggle. */
		home?: boolean;
	} = $props();

	const initial = $derived((session.user?.Name ?? '?').slice(0, 1).toUpperCase());

	async function signOut() {
		// Close the player while it can still report the stop; the root
		// layout unmounts it once signed out.
		player.close();
		await session.signOut();
		await goto('/login', { replace: true });
	}
</script>

<header class="navbar">
	<div class="left">
		{#if home}
			<a class="nav-button" href="/" aria-label="Home"><Icon name="home" size={24} /></a>
		{:else}
			<button
				class="nav-button"
				type="button"
				aria-label={sidebarCollapsed ? 'Expand' : 'Collapse'}
				onclick={onToggleSidebar}
			>
				<Icon name="menu" size={24} />
			</button>
		{/if}
		<a class="nav-button logo" href="/" aria-label="Home">
			<Wordmark height={24} />
		</a>
		<SearchBox {serverName} />
	</div>

	<div class="right">
		<a class="nav-button" href="/settings" aria-label="Settings">
			<Icon name="settings" size={24} />
		</a>
		<div class="nav-button account">
			<Menu variant="plain" align="right" items={[{ label: 'Sign Out', onselect: signOut }]}>
				{#snippet trigger()}
					<span class="avatar" aria-label="Account: {session.user?.Name ?? ''}">{initial}</span>
				{/snippet}
			</Menu>
		</div>
	</div>
</header>

<style>
	.navbar {
		position: relative;
		z-index: 2;
		display: flex;
		align-items: center;
		justify-content: space-between;
		flex-shrink: 0;
		height: var(--size-xxl);
		margin: var(--size-xs);
		border-radius: var(--border-radius-s);
		background: var(--color-surface-background-60);
		font-size: 13px;
	}
	.left,
	.right {
		display: flex;
		align-items: center;
		height: 100%;
		min-width: 0;
	}
	.left {
		flex: 1;
	}
	.right {
		padding: 0 var(--size-m);
	}

	/* Plex: _76v8d61 _76v8d65 _76v8d6h _76v8d6g (a "clear" medium button). */
	.nav-button {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		flex-shrink: 0;
		min-height: 40px;
		padding: 0 var(--size-m);
		border-radius: var(--border-radius-s);
		color: var(--color-text-default);
		line-height: 0;
		transition: all var(--duration-fast);
	}
	.nav-button:hover {
		color: var(--color-text-primary);
	}
	.logo {
		align-self: stretch;
		min-height: 0;
	}

	.account {
		padding: 0 0 0 var(--size-xs);
	}
	.avatar {
		display: grid;
		place-items: center;
		width: 28px;
		height: 28px;
		border-radius: 50%;
		background: var(--color-background-accent);
		color: var(--color-text-on-accent);
		font-size: 13px;
		font-weight: 700;
		line-height: 1;
	}
</style>
