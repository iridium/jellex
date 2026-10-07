<script lang="ts" module>
	import type { IconName } from '#lib/icons.ts';
	export interface MenuItem {
		label: string;
		/** Shown as the selected entry (Plex's "isActive"). */
		active?: boolean;
		/** Trailing marker, e.g. a sort direction arrow. */
		suffix?: string;
		danger?: boolean;
		/** Smaller grey text after the label (Plex's itemExtraDetail). */
		detail?: string;
		/** Keep the menu open after choosing (e.g. "Show all"). */
		keepOpen?: boolean;
		/** A submenu (Plex's NestedMenuItem, with a ▸). */
		items?: MenuEntry[];
		/** Shown at the right of an active entry (Plex's SelectedMenuItem):
		 *  a check by default, or a sort direction. */
		selectedIcon?: IconName;
		/** Plex's DisclosureFilterMenuItem: choosing it swaps the menu for a
		 *  filterable list of values. */
		drill?: MenuDrill;
		onselect?: () => void;
	}
	/** Plex's MenuHeader: an uppercase grey label, e.g. "Recent". */
	export interface MenuHeading {
		heading: string;
	}
	export interface MenuDrill {
		title: string;
		placeholder: string;
		load: () => Promise<{ label: string; active?: boolean; onselect: () => void }[]>;
	}
	export type MenuEntry = MenuItem | MenuHeading | { separator: true };
	export const separator = { separator: true } as const;
</script>

<script lang="ts">
	import type { Snippet } from 'svelte';
	import { menuIn, menuOut } from '#lib/motion.ts';
	import Icon from './Icon.svelte';
	import Spinner from './Spinner.svelte';

	// Plex Web's DisclosureArrowLink + Menu: a text button with a small
	// triangle that opens a #191a1c menu of 13px items. The menu is placed
	// with fixed positioning, so scrolling rows can't clip it (Plex renders
	// it in a portal).
	let {
		items,
		label,
		trigger,
		size = 'medium',
		align = 'left',
		variant = 'disclosure',
		arrow = 'medium',
		placement = 'below',
		offset,
		onopen
	}: {
		items: MenuEntry[];
		label?: string;
		trigger?: Snippet;
		size?: 'small' | 'medium' | 'large' | 'auto';
		align?: 'left' | 'right';
		variant?: 'disclosure' | 'plain';
		/** Plex's DisclosureArrow size: medium (8×5) or large (10×6). */
		arrow?: 'medium' | 'large';
		/** Open below the trigger, or above it (menus in the player bar). */
		placement?: 'below' | 'above';
		/** Distance from the trigger's bottom edge to the menu's top, where a
		 *  Plex menu sets its own popper offset (the sidebar's is -8). */
		offset?: number;
		/** Called when the menu opens (e.g. to load playlists). */
		onopen?: () => void;
	} = $props();

	let open = $state(false);
	let root: HTMLElement | undefined = $state();
	let pos = $state<Record<string, string>>({});
	let sub = $state<{ index: number; items: MenuEntry[]; top: number; left: number } | null>(null);
	type DrillValue = Awaited<ReturnType<MenuDrill['load']>>[number];
	let drillToken = 0;
	let drill = $state<{ spec: MenuDrill; values: DrillValue[] | null; query: string } | null>(null);
	const drillShown = $derived(
		drill?.values?.filter((v) =>
			v.label.toLowerCase().includes(drill!.query.trim().toLowerCase())
		) ?? []
	);

	function place() {
		if (!root) return;
		const r = root.getBoundingClientRect();
		const p: Record<string, string> = {};
		if (placement === 'above') p.bottom = `${window.innerHeight - r.top + 4}px`;
		// Plex opens menus flush under their trigger (the library toolbar's
		// and the stream pickers' measured at 0) unless a menu sets its own
		// popper offset.
		else p.top = `${r.bottom + (offset ?? 0)}px`;
		if (align === 'right') p.right = `${window.innerWidth - r.right}px`;
		else p.left = `${r.left}px`;
		pos = p;
	}

	let menuEls = new Set<HTMLElement>();
	function enter(el: HTMLElement) {
		menuEls.add(el);
		menuIn(el);
		return { destroy: () => menuEls.delete(el) };
	}
	let closing = false;
	async function close() {
		if (!open || closing) return;
		closing = true;
		await Promise.all([...menuEls].map((el) => menuOut(el)));
		open = false;
		sub = null;
		drill = null;
		closing = false;
	}
	function toggle() {
		if (open) return void close();
		open = true;
		sub = null;
		drill = null;
		place();
		onopen?.();
	}

	async function choose(item: MenuItem, e: MouseEvent, index: number) {
		if (item.items) return openSub(item, e, index);
		if (item.drill) {
			// $state proxies what it stores, so match the request by number.
			const token = ++drillToken;
			drill = { spec: item.drill, values: null, query: '' };
			const values = await item.drill.load().catch(() => []);
			if (drill && token === drillToken) drill.values = values;
			return;
		}
		if (!item.keepOpen) close();
		item.onselect?.();
	}

	function openSub(item: MenuItem, e: MouseEvent, index: number) {
		const r = (e.currentTarget as HTMLElement).getBoundingClientRect();
		const left = r.right + 220 > window.innerWidth ? r.left - 220 : r.right;
		sub = { index, items: item.items!, top: r.top - 8, left };
	}

	$effect(() => {
		if (!open) return;
		const outside = (e: Event) => {
			const t = e.target as Element;
			if (root?.contains(t) || t.closest?.('.menu')) return;
			close();
		};
		const esc = (e: KeyboardEvent) => e.key === 'Escape' && close();
		const onScroll = (e: Event) => {
			if (!(e.target as Element).closest?.('.menu')) close();
		};
		document.addEventListener('pointerdown', outside);
		document.addEventListener('keydown', esc);
		window.addEventListener('scroll', onScroll, true);
		window.addEventListener('resize', place);
		return () => {
			document.removeEventListener('pointerdown', outside);
			document.removeEventListener('keydown', esc);
			window.removeEventListener('scroll', onScroll, true);
			window.removeEventListener('resize', place);
		};
	});
</script>

{#snippet list(entries: MenuEntry[], nested: boolean)}
	{#each entries as entry, i (i)}
		{#if 'separator' in entry}
			<div class="separator" role="separator"></div>
		{:else if 'heading' in entry}
			<div class="heading" role="presentation">{entry.heading}</div>
		{:else}
			<button
				class="menu-item"
				class:active={entry.active}
				class:danger={entry.danger}
				class:nested-open={!nested && sub?.index === i}
				type="button"
				role="menuitem"
				aria-haspopup={entry.items ? 'menu' : undefined}
				onclick={(e) => choose(entry, e, i)}
				onpointerenter={(e) => {
					if (nested) return;
					if (entry.items) openSub(entry, e as unknown as MouseEvent, i);
					else sub = null;
				}}
			>
				<span class="label"
					>{entry.label}{#if entry.detail}<span class="detail">{entry.detail}</span>{/if}</span
				>
				{#if entry.items}
					<span class="nested-arrow"></span>
				{:else if entry.suffix}
					<span class="suffix">{entry.suffix}</span>
				{:else if entry.active}
					<span class="selected-icon"
						><Icon name={entry.selectedIcon ?? 'selected'} size={14} label="Selected" /></span
					>
				{/if}
			</button>
		{/if}
	{/each}
{/snippet}

<div class="menu-root" bind:this={root}>
	<button
		class="trigger"
		class:disclosure={variant === 'disclosure'}
		class:large={arrow === 'large'}
		class:open
		type="button"
		aria-haspopup="menu"
		aria-expanded={open}
		onclick={toggle}
	>
		{#if trigger}{@render trigger()}{:else}{label}{/if}
		{#if variant === 'disclosure'}<span class="arrow"></span>{/if}
	</button>
	{#if open}
		<div
			use:enter
			class="menu size-{size}"
			class:right={align === 'right'}
			class:above={placement === 'above'}
			role="menu"
			style:top={pos.top}
			style:bottom={pos.bottom}
			style:left={pos.left}
			style:right={pos.right}
		>
			{#if drill}
				<!-- Plex: DirectoryListFilterMenu's drill-in. -->
				<div class="drill-header">
					<button class="drill-back" type="button" onclick={() => (drill = null)}>
						<Icon name="back" size={16} label="Back" />
						<span class="drill-title">{drill.spec.title}</span>
					</button>
					<input
						class="drill-search"
						placeholder={drill.spec.placeholder}
						bind:value={drill.query}
					/>
				</div>
				<div class="separator" role="separator"></div>
				<div class="drill-list">
					{#if !drill.values}
						<div class="drill-state"><Spinner size="small" /></div>
					{:else}
						{#each drillShown as v (v.label)}
							<button
								class="menu-item"
								class:active={v.active}
								type="button"
								role="menuitem"
								onclick={() => {
									close();
									v.onselect();
								}}
							>
								<span class="label">{v.label}</span>
								{#if v.active}<span class="selected-icon"
										><Icon name="selected" size={14} label="Selected" /></span
									>{/if}
							</button>
						{/each}
					{/if}
				</div>
			{:else}
				{@render list(items, false)}
			{/if}
		</div>
		{#if sub}
			{#key sub.index}
				<div
					use:enter
					class="menu size-{size === 'auto' ? 'medium' : size} submenu"
					role="menu"
					style:top="{sub.top}px"
					style:left="{sub.left}px"
				>
					{@render list(sub.items, true)}
				</div>
			{/key}
		{/if}
	{/if}
</div>

<style>
	.menu-root {
		position: relative;
		display: inline-flex;
		height: 100%;
		align-items: center;
	}
	/* Plex: DisclosureArrowLink (medium). */
	.trigger {
		position: relative;
		display: inline-flex;
		align-items: center;
		height: 100%;
		max-width: 100%;
		color: hsla(0, 0%, 100%, 0.7);
		white-space: nowrap;
		transition: color 0.2s;
	}
	.trigger:hover,
	.trigger.open {
		color: #fff;
	}
	.trigger.disclosure {
		padding-right: 13px;
	}
	/* Plex: DisclosureArrow (medium, down). */
	.arrow {
		position: absolute;
		top: 50%;
		right: 0;
		width: 0;
		height: 0;
		border-style: solid;
		border-color: hsla(0, 0%, 100%, 0.7) transparent;
		border-width: 5px 4px 0;
		transform: translateY(-50%);
		transition:
			border 0.2s,
			transform 0.4s;
	}
	.trigger.large {
		padding-right: 15px;
	}
	.trigger.large .arrow {
		border-width: 6px 5px 0;
	}
	.trigger:hover .arrow,
	.trigger.open .arrow {
		border-top-color: #fff;
	}
	/* Plex: DisclosureArrowLink-up — the arrow flips while the menu is open. */
	.trigger.open .arrow {
		transform: translateY(-50%) rotateX(180deg);
	}

	/* Plex: Menu + MenuItem (rendered with fixed positioning). */
	.menu {
		position: fixed;
		z-index: 1114;
		max-height: calc(100vh - 16px);
		overflow: auto;
		padding: 8px 0;
		border-radius: 4px;
		background-color: #191a1c;
		box-shadow: 0 4px 10px rgba(0, 0, 0, 0.35);
		font-family: var(--font-system);
		font-size: 13px;
		line-height: 1.71428571;
		text-align: left;
		white-space: normal;
		transform-origin: center top;
		transition: background 0.15s ease-out;
	}
	.size-small {
		width: 120px;
	}
	.size-medium {
		width: 180px;
	}
	.size-large {
		width: 220px;
	}
	.size-auto {
		width: max-content;
		min-width: 180px;
	}
	.menu-item {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 14px;
		width: 100%;
		padding: 4px 20px;
		color: hsla(0, 0%, 100%, 0.7);
		text-align: left;
		word-wrap: break-word;
		transition: color 0.2s;
	}
	.menu-item:hover,
	.menu-item.nested-open {
		background-color: hsla(0, 0%, 100%, 0.08);
		color: #fff;
	}
	.menu-item.active {
		color: var(--color-brand-accent);
	}
	.menu-item.danger:hover {
		background-color: #b32;
	}
	.suffix {
		color: hsla(0, 0%, 100%, 0.7);
	}
	/* Plex: AudioVideoQualityMenuItem-itemExtraDetail. */
	.detail {
		margin: 0 8px;
		color: hsla(0, 0%, 100%, 0.45);
		font-size: 12px;
	}
	/* Plex: NestedMenuItem's ▸. */
	.nested-arrow {
		flex-shrink: 0;
		width: 0;
		height: 0;
		border-style: solid;
		border-color: transparent currentColor;
		border-width: 4px 0 4px 6px;
	}
	/* Plex: SelectedMenuItem-selectedIcon. */
	.selected-icon {
		display: flex;
		flex-shrink: 0;
	}
	/* Plex: DirectoryListFilterMenu header, search and list. */
	.drill-header {
		padding: 2px 12px 6px;
	}
	.drill-back {
		display: flex;
		align-items: center;
		gap: 8px;
		width: 100%;
		height: 23.3px;
		color: hsla(0, 0%, 100%, 0.7);
		transition: color 0.2s;
	}
	.drill-back:hover {
		color: #fff;
	}
	.drill-title {
		color: hsla(0, 0%, 100%, 0.3);
		text-transform: uppercase;
	}
	.drill-search {
		width: 100%;
		height: 30px;
		margin-top: 8px;
		padding: 2px 22px;
		border: 0;
		border-radius: 4px;
		background-color: hsla(0, 0%, 100%, 0.07);
		color: #eee;
		font: inherit;
		outline: none;
	}
	.drill-search::placeholder {
		color: hsla(0, 0%, 100%, 0.3);
	}
	.drill-list {
		max-height: 276px;
		overflow-y: auto;
	}
	.drill-state {
		display: flex;
		justify-content: center;
		padding: 8px;
	}
	/* Plex: MenuHeader. */
	.heading {
		padding: 4px 20px;
		color: hsla(0, 0%, 100%, 0.45);
		text-transform: uppercase;
	}
	/* Plex: MenuSeparator. */
	.separator {
		height: 1px;
		margin: 5px 0;
		background-color: hsla(0, 0%, 78%, 0.15);
	}
</style>
