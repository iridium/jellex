<script lang="ts">
	import { goto } from '$app/navigation';
	import {
		accentPresets,
		applyTheme,
		resetCustomization,
		settings,
		type Settings
	} from '#lib/settings.svelte.ts';
	import type { PageProps } from './$types';

	// Plex Web's SettingsPage for the web app's own settings: "jellex —
	// Page", the Show Advanced bar, the fields, and Save Changes (enabled
	// once something changed). Only settings jellex honours are listed.
	let { data }: PageProps = $props();

	let draft = $state<Settings>({ ...settings.value });
	let advanced = $state(false);
	const dirty = $derived(JSON.stringify(draft) !== JSON.stringify(settings.value));
	// Pages that have advanced settings show the bar.
	const hasAdvanced = $derived(data.page === 'general');

	$effect(() => {
		data.page;
		draft = { ...settings.value };
	});

	// Theme changes preview straight away and revert unless saved.
	const custom = $derived(!accentPresets.some((p) => p.main === draft.accent.toLowerCase()));
	$effect(() => {
		applyTheme(draft.accent);
		return () => applyTheme(settings.value.accent);
	});

	function save(e: SubmitEvent) {
		e.preventDefault();
		settings.save(draft);
	}

	function reset() {
		resetCustomization();
		// Reload so the sidebar reads its defaults again.
		goto('/').then(() => location.reload());
	}

	const colors = [
		['White', '#ffffff'],
		['Yellow', '#ffee00'],
		['Black', '#000000'],
		['Cyan', '#00ffff'],
		['Blue', '#0000ff'],
		['Green', '#00ff00'],
		['Magenta', '#ee00ee'],
		['Red', '#ff0000'],
		['Grey', '#808080']
	] as const;
	const positions = [
		['Top', 'top'],
		['Middle', 'middle'],
		['Bottom', 'bottom']
	] as const;
	const sizes = [
		['Tiny', 50],
		['Small', 75],
		['Normal', 100],
		['Large', 125],
		['Huge', 200]
	] as const;
</script>

<svelte:head>
	<title>{data.label} · Settings · jellex</title>
</svelte:head>

<div class="page-content scroller">
	<div class="content">
		<h2 class="header">jellex<span class="dash">—</span>{data.label}</h2>

		<!-- Plex: the advanced-settings bar (its button where a page has
		     advanced settings jellex supports). -->
		<div class="advanced-bar">
			{#if hasAdvanced}
				<button class="small-button" type="button" onclick={() => (advanced = !advanced)}>
					{advanced ? 'Hide Advanced' : 'Show Advanced'}
				</button>
			{/if}
		</div>

		<form onsubmit={save}>
			{#if data.page === 'general'}
				<h4>Version {__APP_VERSION__}</h4>
				<div class="form-group">
					<p class="help">
						jellex is open source:
						<a href="https://github.com/iridium/jellex" target="_blank" rel="noopener noreferrer"
							>github.com/iridium/jellex</a
						>
					</p>
				</div>
				<div class="form-group">
					<label
						><input type="checkbox" bind:checked={draft.rememberTab} /> Remember selected tab</label
					>
					<p class="help">Default to the last selected tab for a media source.</p>
				</div>
				{#if advanced}
					<hr />
					<div class="form-group">
						<button class="small-button" type="button" onclick={reset}>Reset Customization</button>
						<p class="help">Reset sidebar navigation to the default state.</p>
					</div>
					<hr />
				{/if}
			{:else if data.page === 'theme'}
				<h4 class="section">Colors</h4>
				<div class="form-group">
					<label for="accent">Accent Color</label>
					<select
						id="accent"
						value={custom ? 'custom' : draft.accent.toLowerCase()}
						onchange={(e) => {
							const v = e.currentTarget.value;
							if (v !== 'custom') draft.accent = v;
						}}
					>
						{#each accentPresets as p (p.main)}<option value={p.main}>{p.label}</option>{/each}
						<option value="custom">Custom</option>
					</select>
					<input
						class="color"
						type="color"
						aria-label="Custom accent color"
						bind:value={draft.accent}
					/>
					<p class="help">Used for highlights, progress bars, selected tabs and buttons.</p>
				</div>
			{:else}
				<h4 class="section">Audio &amp; Subtitles</h4>
				<div class="form-group">
					<label for="subtitle-color">Subtitle Color</label>
					<select id="subtitle-color" bind:value={draft.subtitleColor}>
						{#each colors as [label, value] (value)}<option {value}>{label}</option>{/each}
					</select>
				</div>
				<div class="form-group">
					<label for="subtitle-position">Subtitle Position</label>
					<select id="subtitle-position" bind:value={draft.subtitlePosition}>
						{#each positions as [label, value] (value)}<option {value}>{label}</option>{/each}
					</select>
				</div>
				<div class="form-group">
					<label for="subtitle-size">Subtitle Size</label>
					<select id="subtitle-size" bind:value={draft.subtitleSize}>
						{#each sizes as [label, value] (value)}<option {value}>{label}</option>{/each}
					</select>
				</div>
			{/if}

			<div class="actions">
				<button class="save" type="submit" disabled={!dirty}>Save Changes</button>
			</div>
		</form>
	</div>
</div>

<style>
	/* Plex: Page-page + SettingsPage-content (centred, 1030px max). */
	.page-content {
		flex: 1;
		min-height: 0;
		overflow: hidden auto;
		padding-right: 5px;
	}
	.content {
		max-width: 1030px;
		margin: 0 auto;
		padding: 0 20px 48px 30px;
		color: #eee;
		font-size: 13px;
		line-height: 22.2857px;
	}
	/* Plex: SettingsPageHeader. */
	.header {
		margin: 40px 0 20px;
		color: #fff;
		font-family: var(--font-heading);
		font-size: 24px;
		font-weight: 700;
		line-height: 36px;
	}
	.dash {
		margin: 0 4px;
	}
	.advanced-bar {
		display: flex;
		justify-content: flex-end;
		/* Plex's bar sits in its own wrapper, so its margin adds to the
		   header's instead of collapsing. */
		margin: 40px 0 20px;
		min-height: 50px;
		padding: 13px 20px;
		background-color: rgba(0, 0, 0, 0.15);
		line-height: 22px;
	}
	/* Bootstrap btn-sm as Plex styles it. */
	.small-button {
		height: 22px;
		padding: 2px 14px;
		border-radius: 3px;
		background-color: rgba(255, 255, 255, 0.25);
		color: #fff;
		font-family: var(--font-heading);
		font-size: 12px;
		font-weight: 600;
		line-height: 18px;
	}
	.small-button:hover {
		background-color: rgba(255, 255, 255, 0.3);
	}
	/* Plex's form doesn't collapse margins with what it holds. */
	form {
		display: flow-root;
	}
	h4 {
		margin: 12px 0 20px;
		padding-bottom: 10px;
		font-size: 16px;
		font-weight: 400;
		line-height: 24px;
	}
	h4.section {
		margin: 12px 0;
		color: #999;
	}
	.form-group {
		margin-bottom: 20px;
	}
	label {
		display: inline-block;
		margin-bottom: 5px;
	}
	label input[type='checkbox'] {
		margin: 4px 0 0;
		vertical-align: top;
	}
	/* Plex leaves selects with the browser's own look. */
	select {
		all: revert;
		margin-left: 10px;
		font-size: 16px;
	}
	.color {
		width: 32px;
		height: 23px;
		margin-left: 10px;
		padding: 0;
		border: 0;
		background: none;
		vertical-align: middle;
		cursor: pointer;
	}
	.help {
		margin: 5px 0 10px;
		color: hsla(0, 0%, 100%, 0.45);
	}
	/* Plex's help-block links (the settings' "here"). */
	.help a {
		color: var(--color-accent-dark);
	}
	.help a:hover {
		text-decoration: underline;
	}
	hr {
		margin: 24px 0;
		border: 0;
		border-top: 1px solid #323232;
	}
	.actions {
		margin-top: 40px;
	}
	/* Plex: btn-lg btn-primary. */
	.save {
		padding: 10px 18px;
		border-radius: 3px;
		background-color: var(--color-accent-dark);
		color: #fff;
		font-family: var(--font-heading);
		font-size: 16px;
		font-weight: 600;
		line-height: 21.28px;
	}
	.save:disabled {
		opacity: 0.3;
		cursor: default;
	}
</style>
