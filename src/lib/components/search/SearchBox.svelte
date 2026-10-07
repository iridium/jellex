<script lang="ts">
	import type { BaseItemDto } from '@jellyfin/sdk/lib/generated-client';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { search } from '#lib/search.ts';
	import Icon from '../Icon.svelte';
	import Spinner from '../Spinner.svelte';
	import SearchRow from './SearchRow.svelte';

	// Plex Web's UniversalSearch: a 480px pill that turns white when focused,
	// with a clear button once there's text, and the SearchPopover of top
	// results under it.
	let { serverName }: { serverName: string } = $props();

	let query = $state(
		page.url.pathname === '/search' ? (page.url.searchParams.get('query') ?? '') : ''
	);
	let focused = $state(false);
	let open = $state(false);
	let closing = $state(false);
	function hide() {
		if (!open || closing) return;
		closing = true;
		setTimeout(() => {
			open = false;
			closing = false;
		}, 100);
	}
	let results = $state<BaseItemDto[]>([]);
	let loading = $state(false);
	let root: HTMLElement | undefined = $state();
	let input: HTMLInputElement | undefined = $state();
	let timer: ReturnType<typeof setTimeout> | undefined;
	let token = 0;

	function changed() {
		clearTimeout(timer);
		const term = query.trim();
		if (!term) {
			results = [];
			hide();
			return;
		}
		open = true;
		loading = true;
		const t = ++token;
		timer = setTimeout(async () => {
			const r = await search(term, 'top', 10).catch(() => []);
			if (t !== token) return;
			results = r;
			loading = false;
		}, 250);
	}

	function submit(e: SubmitEvent) {
		e.preventDefault();
		const term = query.trim();
		if (!term) return;
		hide();
		input?.blur();
		goto(`/search?pivot=top&query=${encodeURIComponent(term)}`);
	}

	function clear() {
		query = '';
		results = [];
		hide();
		input?.focus();
	}

	$effect(() => {
		if (!open) return;
		const close = (e: Event) => {
			if (!root?.contains(e.target as Node) && !(e.target as Element).closest?.('.menu')) hide();
		};
		const esc = (e: KeyboardEvent) => e.key === 'Escape' && (hide(), input?.blur());
		document.addEventListener('pointerdown', close);
		document.addEventListener('keydown', esc);
		return () => {
			document.removeEventListener('pointerdown', close);
			document.removeEventListener('keydown', esc);
		};
	});

	let scrollEl: HTMLDivElement | undefined = $state();
	let shadowTop = $state(false);
	let shadowBottom = $state(false);
	function updateShadows() {
		if (!scrollEl) return;
		shadowTop = scrollEl.scrollTop > 0;
		shadowBottom = scrollEl.scrollTop + scrollEl.clientHeight < scrollEl.scrollHeight - 1;
	}
	$effect(() => {
		results;
		if (!scrollEl) return;
		const ro = new ResizeObserver(updateShadows);
		ro.observe(scrollEl);
		if (scrollEl.firstElementChild) ro.observe(scrollEl.firstElementChild);
		updateShadows();
		return () => ro.disconnect();
	});
</script>

<div class="search-root" bind:this={root}>
	<form class="search" class:focused role="search" onsubmit={submit}>
		<span class="search-icon"><Icon name="search" size={16} /></span>
		<input
			bind:this={input}
			bind:value={query}
			type="search"
			placeholder="Search"
			aria-label="Search"
			autocomplete="off"
			spellcheck="false"
			oninput={changed}
			onfocus={() => ((focused = true), query.trim() && (open = true))}
			onblur={() => (focused = false)}
		/>
		{#if query}
			<button class="clear" type="button" aria-label="Clear Search" onclick={clear}>
				<Icon name="clear" size={16} />
			</button>
		{/if}
	</form>

	{#if open}
		<!-- Plex: SearchPopover. -->
		<div class="popover" class:closing>
			<!-- Plex: the heading and rows scroll; inset shadows show there is
			     more above or below, and the footer stays put. -->
			<div class="scroll-area">
				<div class="scroller" bind:this={scrollEl} onscroll={updateShadows}>
					<div class="popover-header">
						<h2>Top Results for "{query.trim()}"</h2>
					</div>
					{#if loading && !results.length}
						<div class="state"><Spinner /></div>
					{:else if !results.length}
						<div class="state empty">No Results Found</div>
					{:else}
						<div class="results">
							{#each results as item (item.Id)}
								<SearchRow {item} {serverName} onnavigate={hide} />
							{/each}
						</div>
					{/if}
				</div>
				<div class="edge-shadow top" class:on={shadowTop}></div>
				<div class="edge-shadow bottom" class:on={shadowBottom}></div>
			</div>
			{#if results.length}
				<div class="footer">
					<button
						class="more-results"
						type="button"
						onclick={() => root?.querySelector('form')?.requestSubmit()}
					>
						View More Results
					</button>
				</div>
			{/if}
		</div>
	{/if}
</div>

<style>
	.search-root {
		position: relative;
		display: flex;
		align-items: center;
		height: 100%;
		margin-left: var(--size-s);
	}
	/* Plex: UniversalSearch (_1326e1n9). Focus turns the pill white. */
	.search {
		display: flex;
		align-items: center;
		width: 480px;
		min-width: 0;
		height: var(--size-xl);
		border-radius: var(--border-radius-max);
		background-color: var(--color-primary-foreground-10);
		color: var(--color-primary-foreground-100);
		transition:
			background-color var(--duration-fast),
			color var(--duration-fast);
	}
	.search.focused {
		background-color: var(--color-background-focus);
		color: var(--color-text-on-focus);
		transition: none;
	}
	.search-icon {
		display: flex;
		align-items: center;
		padding: var(--size-xxs) var(--size-xxs) var(--size-xxs) var(--size-xs);
		margin-right: var(--size-xxs);
	}
	input {
		flex: 1;
		min-width: 0;
		height: 100%;
		padding: 1px 2px 1px 0;
		border: 0;
		outline: 0;
		background: none;
		color: inherit;
		font-family: var(--font-body-2-font-family);
		font-size: var(--font-body-2-font-size);
		line-height: var(--font-body-2-line-height);
	}
	input:focus-visible {
		box-shadow: none;
	}
	input::placeholder {
		color: transparent;
	}
	input::-webkit-search-cancel-button {
		display: none;
	}
	.clear {
		display: flex;
		align-items: center;
		margin-right: var(--size-xxs);
		padding: var(--size-xxs) var(--size-xs);
		border-radius: var(--border-radius-max);
		color: inherit;
	}

	/* Plex: SearchPopover. */
	.popover {
		position: absolute;
		top: 48px;
		left: 0;
		z-index: 1020;
		width: 560px;
		/* Plex: 818px in a 900px window. */
		display: flex;
		flex-direction: column;
		max-height: calc(100vh - 82px);
		padding: 4px 0;
		border-radius: 4px;
		background-color: rgb(32, 38, 41);
		box-shadow: 0 4px 16px 0 rgba(0, 0, 0, 0.2);
		color: rgba(255, 255, 255, 0.8);
		font-size: 14px;
		line-height: 20px;
		/* Plex's popover enter: three 100ms keyframes, in Plex's order (the
		   two transform ones compose like Plex's: the last wins). */
		animation:
			popover-fade-in 100ms linear,
			popover-scale-in 100ms linear,
			popover-slide-in 100ms linear;
	}
	.popover.closing {
		animation:
			popover-fade-out 100ms linear forwards,
			popover-scale-out 100ms linear forwards,
			popover-slide-out 100ms linear forwards;
	}
	@keyframes popover-fade-in {
		0% {
			opacity: 0;
			animation-timing-function: ease-out;
		}
		to {
			opacity: 1;
		}
	}
	@keyframes popover-scale-in {
		0% {
			transform: scale(0.95);
			animation-timing-function: cubic-bezier(0.6, 0.4, 0.2, 1.4);
		}
		to {
			transform: scale(1);
		}
	}
	@keyframes popover-slide-in {
		0% {
			transform: translateY(-5px);
			animation-timing-function: cubic-bezier(0.6, 0.4, 0.2, 1.4);
		}
		to {
			transform: translateY(0);
		}
	}
	@keyframes popover-fade-out {
		0% {
			opacity: 1;
			animation-timing-function: ease-out;
		}
		to {
			opacity: 0;
		}
	}
	@keyframes popover-scale-out {
		0% {
			transform: scale(1);
			animation-timing-function: cubic-bezier(0.6, 0.4, 0.2, 1.4);
		}
		to {
			transform: scale(0.95);
		}
	}
	@keyframes popover-slide-out {
		0% {
			transform: translateY(0);
			animation-timing-function: cubic-bezier(0.6, 0.4, 0.2, 1.4);
		}
		to {
			transform: translateY(-5px);
		}
	}
	.scroll-area {
		position: relative;
		display: flex;
		flex: 1;
		min-height: 0;
		overflow: hidden;
	}
	.scroller {
		flex-grow: 1;
		overflow-y: auto;
		scrollbar-color: var(--color-surface-foreground-10) transparent;
	}
	.edge-shadow {
		position: absolute;
		left: 0;
		width: 100%;
		height: 16px;
		pointer-events: none;
		transition: box-shadow var(--duration-slow) ease-in-out;
	}
	.edge-shadow.top {
		top: 0;
	}
	.edge-shadow.bottom {
		bottom: 0;
	}
	.edge-shadow.top.on {
		box-shadow: inset 0 20px 16px -16px rgba(0, 0, 0, 0.4);
	}
	.edge-shadow.bottom.on {
		box-shadow: inset 0 -20px 16px -16px rgba(0, 0, 0, 0.4);
	}
	.popover-header {
		padding: var(--size-s) var(--size-m);
		text-align: center;
	}
	h2 {
		margin: 0;
		color: #fff;
		font-family: var(--font-heading-2-font-family);
		font-size: var(--font-heading-2-font-size);
		font-weight: 700;
		line-height: 32px;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.state {
		display: grid;
		place-items: center;
		padding: 24px;
	}
	.empty {
		color: hsla(0, 0%, 100%, 0.45);
	}
	.footer {
		display: flex;
		justify-content: flex-end;
		padding: 16px;
	}
	.more-results {
		min-height: 28px;
		padding: 0 12px;
		border-radius: 4px;
		background-color: var(--color-background-accent-focus);
		color: var(--color-text-on-accent);
		font-size: 14px;
		font-weight: 600;
		line-height: 20px;
		transition: background-color var(--duration-fast);
	}
	.more-results:hover {
		background-color: var(--color-background-accent);
	}
</style>
