<script lang="ts">
	import { page } from '$app/state';
	import { settingsPages } from '#lib/settings.svelte.ts';

	// Plex Web's SettingsSidebar: a 240px panel with the "jellex" list
	// header and its pages; the current one is accent with a bullet.
</script>

<nav class="sidebar" aria-label="Settings">
	<div class="content">
		<div class="header">jellex</div>
		{#each settingsPages as p (p.key)}
			{@const selected = page.url.pathname === `/settings/${p.key}`}
			<a
				class="link"
				class:selected
				href="/settings/{p.key}"
				aria-current={selected ? 'page' : undefined}
			>
				<span class="icon"
					>{#if selected}•{/if}</span
				>
				<span class="title">{p.label}</span>
			</a>
		{/each}
	</div>
</nav>

<style>
	/* Plex: SidebarContainer. */
	.sidebar {
		flex-shrink: 0;
		width: 240px;
		margin: 0 8px 8px;
		overflow: hidden auto;
		border-radius: 4px;
		background-color: rgba(0, 0, 0, 0.15);
	}
	.content {
		padding: 10px 5px 60px 0;
	}
	/* Plex: SidebarList-sidebarListHeader. */
	.header {
		padding-left: 25px;
		color: hsla(0, 0%, 100%, 0.3);
		font-family: var(--font-heading);
		font-size: 15px;
		font-weight: 700;
		line-height: 45px;
	}
	/* Plex: SidebarLink. */
	.link {
		display: flex;
		height: 30px;
		padding: 4px 0 2px 25px;
		color: hsla(0, 0%, 100%, 0.7);
		font-size: 13px;
		line-height: 22.2857px;
		transition: color 0.2s;
	}
	.link:hover {
		color: #fff;
	}
	.link.selected {
		color: var(--color-brand-accent);
	}
	.icon {
		flex-shrink: 0;
		width: 18px;
		line-height: 14px;
		padding-top: 4px;
	}
	.title {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
</style>
