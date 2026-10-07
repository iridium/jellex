<script lang="ts">
	import type { Snippet } from 'svelte';
	import Icon from './Icon.svelte';
	import Menu, { type MenuItem } from './Menu.svelte';
	import PosterSizeSlider from './PosterSizeSlider.svelte';
	import type { ListStyle } from '#lib/ui.svelte.ts';
	import { navigating } from '$app/state';

	// Plex Web's PageHeader: 50px, the breadcrumb on the left (a plain title,
	// or Plex's two-line PageHeaderTitle: library over server name), optional
	// centred tabs, and page actions on the right.
	let {
		title,
		detail,
		href,
		center,
		right,
		slider = false,
		menu,
		listStyle,
		onListStyle,
		crumb,
		background,
		nav
	}: {
		/** Plex's Show / Blur Background button (details pages with art). */
		background?: { shown: boolean; ontoggle: () => void };
		/** Plex's details navigation: Previous, a list of the siblings, Next. */
		nav?: { previous?: string; next?: string; items: MenuItem[] };
		title: string;
		/** Plex's CurrentSourceBreadcrumb: "title · crumb" on one line. */
		crumb?: string;
		detail?: string;
		href?: string;
		center?: Snippet;
		right?: Snippet;
		/** Show Plex's poster-size slider (pages that list posters). */
		slider?: boolean;
		/** The breadcrumb's ⋮ menu (Plex's CurrentSourceBreadcrumb menu). */
		menu?: MenuItem[];
		/** Plex's MetadataListStylesMenu (grid / detail / table). */
		listStyle?: ListStyle;
		onListStyle?: (s: ListStyle) => void;
	} = $props();
	const styleIcon = { grid: 'grid', detail: 'view-detail', table: 'view-table' } as const;
</script>

<div class="page-header">
	<div class="left">
		{#if crumb}
			<div class="crumbs">
				<svelte:element this={href ? 'a' : 'span'} class="crumb-title link" {href}
					>{title}</svelte:element
				>
				<span class="dash">·</span>
				<span class="crumb">{crumb}</span>
			</div>
		{:else if detail}
			<svelte:element this={href ? 'a' : 'div'} class="title-container link" {href}>
				<div class="title">{title}</div>
				<div class="detail">{detail}</div>
			</svelte:element>
		{:else}
			<span class="breadcrumb">{title}</span>
		{/if}
		{#if menu}
			<div class="breadcrumb-menu">
				<Menu variant="plain" items={menu}>
					{#snippet trigger()}<Icon name="more-vertical" size={14} label="Actions" />{/snippet}
				</Menu>
			</div>
		{/if}
	</div>
	<div class="center">
		{#if center}{@render center()}{/if}
	</div>
	<div class="right">
		{#if right}{@render right()}{/if}
		{#if background}
			<button
				class="header-button"
				class:on={background.shown}
				type="button"
				aria-label={background.shown ? 'Blur Background' : 'Show Background'}
				title={background.shown ? 'Blur Background' : 'Show Background'}
				onclick={background.ontoggle}
			>
				<Icon name="background" size={24} />
			</button>
			<span class="divider"></span>
		{/if}
		{#if slider || listStyle}
			<div class="styles" class:with-slider={slider && (!listStyle || listStyle === 'grid')}>
				{#if slider && (!listStyle || listStyle === 'grid')}<PosterSizeSlider />{/if}
				{#if !listStyle || !onListStyle}
					<!-- Plex shows the grid style as a static icon where it can't change. -->
					<span class="waffle" aria-hidden="true"><Icon name="grid" size={24} /></span>
				{:else}
					<span class="style-menu">
						<Menu
							variant="plain"
							align="right"
							size="auto"
							items={[
								{
									label: 'Grid View',
									active: listStyle === 'grid',
									onselect: () => onListStyle('grid')
								},
								{
									label: 'Detail View',
									active: listStyle === 'detail',
									onselect: () => onListStyle('detail')
								},
								{
									label: 'Table View',
									active: listStyle === 'table',
									onselect: () => onListStyle('table')
								}
							]}
						>
							{#snippet trigger()}
								<span class="style-trigger" aria-label="Show List Styles">
									<Icon name={styleIcon[listStyle]} size={24} />
									<Icon name="chevron-down" size={16} />
								</span>
							{/snippet}
						</Menu>
					</span>
				{/if}
			</div>
		{/if}
		{#if nav}
			<!-- While the next item loads, Plex disables the switcher: the list
			     at 0.3 and Previous / Next at 0.5. -->
			<span class="divider"></span>
			<a
				class="header-button"
				class:disabled={!nav.previous || !!navigating.to}
				href={nav.previous ?? null}
				aria-label="Previous"
				aria-disabled={!nav.previous}
			>
				<Icon name="back" size={24} />
			</a>
			<span class="nav-menu" class:busy={!!navigating.to}>
				<Menu variant="plain" align="right" size="large" items={nav.items}>
					{#snippet trigger()}
						<span class="style-trigger" aria-label="Show List">
							<Icon name="episode-list" size={24} />
							<Icon name="chevron-down" size={16} />
						</span>
					{/snippet}
				</Menu>
			</span>
			<a
				class="header-button"
				class:disabled={!nav.next || !!navigating.to}
				href={nav.next ?? null}
				aria-label="Next"
				aria-disabled={!nav.next}
			>
				<Icon name="nav-next" size={24} />
			</a>
		{/if}
	</div>
</div>

<style>
	.page-header {
		display: flex;
		align-items: center;
		flex-shrink: 0;
		height: 50px;
		padding: 0 25px 0 var(--page-gutter);
		color: hsla(0, 0%, 100%, 0.75);
		font-size: 16px;
		line-height: 1.71428571;
	}
	.left,
	.right {
		display: flex;
		align-items: center;
		height: 100%;
		min-width: 0;
		flex-shrink: 0;
	}
	.left {
		margin-right: 25px;
	}
	.center {
		display: flex;
		align-items: center;
		justify-content: center;
		flex: 1;
		min-width: 0;
		height: 100%;
	}
	/* Plex: CurrentSourceBreadcrumb-menuContainer, a 56×50 button. */
	.breadcrumb-menu {
		display: flex;
		height: 100%;
		margin-left: 0;
	}
	.breadcrumb-menu :global(.trigger) {
		justify-content: center;
		width: 56px;
		color: var(--color-text-default);
	}
	.breadcrumb-menu :global(.trigger:hover) {
		color: var(--color-text-primary);
	}
	/* Plex: MetadataListStylesMenu (150px with its view-style button). */
	.styles {
		display: flex;
		align-items: center;
		gap: 16px;
		height: 24px;
		padding: 0 12px 0 16px;
	}
	.styles.with-slider {
		min-width: 150px;
	}
	.waffle {
		display: flex;
		width: 46px;
		color: rgba(255, 255, 255, 0.75);
	}
	/* Plex: a 46px slot holding the 44px trigger. */
	.style-menu {
		display: flex;
		width: 46px;
		height: 24px;
	}
	.style-menu :global(.trigger) {
		color: var(--color-text-default);
	}
	.style-menu :global(.trigger:hover),
	.style-menu :global(.trigger.open) {
		color: var(--color-text-primary);
	}
	.style-trigger {
		display: inline-flex;
		align-items: center;
		gap: 4px;
	}
	/* Plex's header icon buttons (56×40) and their 1×35 dividers. */
	.header-button {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		min-width: 56px;
		height: 40px;
		padding: 0 16px;
		color: rgba(255, 255, 255, 0.8);
		transition: color var(--duration-fast);
	}
	.header-button:hover,
	.header-button.on {
		color: #fff;
	}
	.header-button.disabled {
		opacity: 0.5;
		pointer-events: none;
	}
	.divider {
		flex-shrink: 0;
		width: 1px;
		height: 35px;
		background-color: rgba(255, 255, 255, 0.1);
	}
	.nav-menu {
		display: flex;
		width: 44px;
		height: 24px;
	}
	.nav-menu.busy {
		opacity: 0.3;
		pointer-events: none;
	}
	.nav-menu :global(.trigger) {
		color: rgba(255, 255, 255, 0.8);
	}
	.nav-menu :global(.trigger:hover),
	.nav-menu :global(.trigger.open) {
		color: #fff;
	}
	/* Plex: CurrentSourceBreadcrumb + DashSeparator. */
	.crumbs {
		display: flex;
		align-items: baseline;
		min-width: 0;
		line-height: 24px;
		white-space: nowrap;
	}
	.crumb-title {
		flex-shrink: 0;
		color: hsla(0, 0%, 100%, 0.75);
	}
	.dash,
	.crumb {
		color: hsla(0, 0%, 100%, 0.6);
	}
	/* Plex: 4px between title and separator group, then the separator's
	   own 0 4px margin. */
	.dash {
		margin: 0 4px 0 8px;
	}
	.crumb {
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.breadcrumb {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	/* Plex: PageHeaderTitle. */
	.title-container {
		display: flex;
		flex-direction: column;
		min-width: 0;
	}
	.title {
		color: hsla(0, 0%, 100%, 0.75);
		line-height: 24px;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.detail {
		color: hsla(0, 0%, 100%, 0.45);
		font-size: 12px;
		line-height: 16.8px;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	a.title-container:hover .title {
		color: #fff;
	}
</style>
