<script lang="ts">
	import { fadeIn } from '#lib/motion.ts';
	import type { BaseItemDto } from '@jellyfin/sdk/lib/generated-client';
	import { imageUrl, type ImageKind } from '#lib/images.ts';
	import { timeAgo } from '#lib/format.ts';
	import { itemMenu, loadPlaylists } from '#lib/actions.svelte.ts';
	import { player } from '#lib/player.svelte.ts';
	import Menu from './Menu.svelte';
	import { selection } from '#lib/selection.svelte.ts';
	import { posterSize } from '#lib/ui.svelte.ts';
	import Icon from './Icon.svelte';

	// Plex Web's MetadataPosterListItem: the card (image, progress bar, hover
	// actions) and up to three lines of text under it. Plex's sizes: 165px
	// posters and squares, 360px wide episode thumbnails.
	let {
		item,
		kind = 'poster',
		width: fixedWidth,
		lines: fixedLines,
		addedLine = false,
		continueWatching = false
	}: {
		item: BaseItemDto;
		kind?: ImageKind;
		width?: number;
		/** Overrides the text lines under the card (title first). */
		lines?: string[];
		/** Adds when the item was added ("13 hours ago"), as in Plex's hub lists. */
		addedLine?: boolean;
		/** In a Continue Watching hub (adds Plex's "Remove from Continue Watching"). */
		continueWatching?: boolean;
	} = $props();

	// Plex's slider sets list card sizes unless a width is given.
	const width = $derived(
		fixedWidth ?? (kind === 'landscape' ? posterSize.wideWidth : posterSize.width)
	);
	const src = $derived(imageUrl(item, kind, Math.max(width, 200)));
	const href = $derived(`/items/${item.Id}`);
	// Plex draws progress from a playback position, so shows and seasons
	// (Jellyfin's percentage of episodes watched) get none.
	const progress = $derived(item.IsFolder ? 0 : (item.UserData?.PlayedPercentage ?? 0));
	const played = $derived(
		!!item.UserData?.Played &&
			!['MusicAlbum', 'MusicArtist', 'Audio', 'Playlist', 'Person'].includes(item.Type ?? '')
	);
	const unplayedCount = $derived(
		item.Type === 'Series' || item.Type === 'Season' ? (item.UserData?.UnplayedItemCount ?? 0) : 0
	);
	const selected = $derived(selection.has(item.Id));

	// Plex's lines: title, then context (year, episode name, artist), then
	// the episode code for episodes. Each part links to what it names: the
	// show, the episode, "S1" to the season and "E4" to the episode.
	interface Part {
		text: string;
		href?: string;
		separator?: boolean;
	}
	const link = (id: string | null | undefined) => (id ? `/items/${id}` : undefined);
	const lines = $derived.by((): Part[][] => {
		const year = item.ProductionYear ? String(item.ProductionYear) : '';
		switch (item.Type) {
			case 'Episode': {
				const code: Part[] = [];
				if (item.ParentIndexNumber != null && item.IndexNumber != null)
					code.push(
						{ text: `S${item.ParentIndexNumber}`, href: link(item.SeasonId) },
						{ text: '·', separator: true },
						{ text: `E${item.IndexNumber}`, href }
					);
				return [
					[{ text: item.SeriesName ?? '', href: link(item.SeriesId) ?? href }],
					[{ text: item.Name ?? '', href }],
					code
				];
			}
			case 'Season':
				return [
					[{ text: item.SeriesName ?? '', href: link(item.SeriesId) ?? href }],
					[{ text: item.Name ?? '', href }]
				];
			case 'MusicAlbum': {
				const artist = item.AlbumArtists?.[0];
				return [
					[{ text: item.Name ?? '', href }],
					[{ text: artist?.Name ?? item.AlbumArtist ?? '', href: link(artist?.Id) }]
				];
			}
			case 'Series': {
				const end = item.EndDate ? new Date(item.EndDate).getFullYear() : null;
				const span =
					year && end && item.Status === 'Ended' && end !== +year ? `${year} – ${end}` : year;
				return [[{ text: item.Name ?? '', href }], [{ text: span }]];
			}
			default:
				return [[{ text: item.Name ?? '', href }], [{ text: year }]];
		}
	});
	const shown = $derived(
		fixedLines
			? fixedLines.map((text, i): Part[] => [{ text, href: i === 0 ? href : undefined }])
			: lines
	);
	const title = $derived(shown[0][0]);
	const details = $derived([
		...shown.slice(1).filter((line) => line.some((part) => part.text)),
		...(addedLine && item.DateCreated ? [[{ text: timeAgo(item.DateCreated) }] as Part[]] : [])
	]);
</script>

<div
	class="poster-item"
	class:square={kind === 'square'}
	class:landscape={kind === 'landscape'}
	style:--poster-width="{width}px"
>
	<div class="card">
		{#if src}
			<img {src} alt="" loading="lazy" decoding="async" use:fadeIn />
		{:else}
			<div class="fallback"></div>
		{/if}

		{#if progress > 0 && progress < 100}
			<div class="progress">
				<div class="track"></div>
				<div class="bar" style:transform="translate3d({progress - 100}%, 0, 0)"></div>
			</div>
		{/if}

		<!-- Plex: MetadataPosterCardBadge. Played items get a check; shows
		     and seasons with unwatched episodes get the count. -->
		{#if played}
			<span class="badge"
				><span class="badge-tile"><Icon name="played" size={16} label="Played" /></span></span
			>
		{:else if unplayedCount > 0}
			<span class="badge" aria-label="{unplayedCount} unplayed"
				><span class="badge-tile count">{unplayedCount}</span></span
			>
		{/if}

		<!-- While anything is selected, Plex's cards toggle selection on
		     click instead of opening, and drop their play and ⋮ buttons. -->
		<a
			class="poster-link"
			class:selected
			{href}
			aria-label={title.text}
			onclick={(e) => {
				if (!selection.count) return;
				e.preventDefault();
				selection.toggle(item);
			}}
		></a>

		<div class="actions">
			<button
				class="action select"
				class:is-selected={selected}
				type="button"
				aria-label="Select"
				aria-pressed={selected}
				onclick={() => selection.toggle(item)}
			>
				<span class="select-circle"
					>{#if selected}<Icon name="check" size={9} />{/if}</span
				>
			</button>
			{#if !selection.count}
				<button
					class="action play"
					type="button"
					aria-label="Play"
					onclick={() => player.play(item, { askResume: true })}
				>
					<span class="play-circle"><Icon name="play" size={42} /></span>
				</button>
				<div class="action more">
					<Menu variant="plain" items={itemMenu(item, { continueWatching })} onopen={loadPlaylists}>
						{#snippet trigger()}<Icon
								name="more-vertical"
								size={14}
								label="More Actions"
							/>{/snippet}
					</Menu>
				</div>
			{/if}
		</div>
	</div>

	<a class="title" href={title.href}>{title.text}</a>
	{#each details as line, i (i)}
		{#if line.length === 1 && line[0].href}
			<a class="detail" href={line[0].href}>{line[0].text}</a>
		{:else}
			<div class="detail">
				{#each line as part, j (j)}
					{#if part.separator}<span class="separator">{part.text}</span>{:else if part.href}<a
							href={part.href}>{part.text}</a
						>{:else}{part.text}{/if}
				{/each}
			</div>
		{/if}
	{/each}
</div>

<style>
	.poster-item {
		width: var(--poster-width, 165px);
		flex-shrink: 0;
		--card-ratio: 1.5;
	}
	.poster-item.square {
		--card-ratio: 1;
	}
	.poster-item.landscape {
		--card-ratio: 0.5625;
	}

	/* Plex: PosterCard-card + MetadataSimplePosterCard-card. */
	.card {
		position: relative;
		/* Plex rounds the card height down to whole pixels (165 × 1.5 → 247). */
		height: round(down, calc(var(--poster-width, 165px) * var(--card-ratio)), 1px);
		margin-bottom: 8px;
		border-radius: 4px;
		background-color: rgba(0, 0, 0, 0.45);
		box-shadow: 0 0 4px rgba(0, 0, 0, 0.3);
	}
	img {
		display: block;
		width: 100%;
		height: 100%;
		object-fit: cover;
		border-radius: 4px;
	}
	.fallback {
		width: 100%;
		height: 100%;
		border-radius: 4px;
		background-color: var(--color-surface-foreground-5);
	}

	/* Plex: MetadataPosterCard-progress + MetadataPosterCardProgressBar. */
	.progress {
		position: absolute;
		left: 0;
		right: 0;
		bottom: 0;
		height: 4px;
		overflow: hidden;
		border-radius: 2px;
	}
	.track,
	.bar {
		position: absolute;
		inset: 0;
	}
	.track {
		background-color: #1f2326;
	}
	/* Plex hides the progress bar while the card is hovered. */
	.card:hover .progress,
	.card:has(.poster-link.selected) .progress {
		opacity: 0;
	}
	.bar {
		background-color: var(--color-brand-accent);
		transition: transform 0.6s ease-in-out;
	}

	/* Plex: MetadataPosterCardBadge (top right) + -badgeBackground. */
	.badge {
		position: absolute;
		top: 0;
		right: 0;
		pointer-events: none;
	}
	.badge-tile {
		display: inline-flex;
		justify-content: center;
		min-width: var(--size-xl);
		padding: var(--size-xs);
		border-radius: 0 4px;
		background-color: var(--color-surface-background-80);
		color: var(--color-text-primary);
	}
	.badge-tile :global(svg) {
		display: block;
	}
	/* Chroma's caption text. */
	.count {
		font-family: var(--font-caption-font-family);
		font-size: var(--font-caption-font-size);
		font-weight: var(--font-caption-font-weight);
		line-height: var(--font-caption-line-height);
	}

	/* Plex: PosterCardLink-link / -hoveredLink. */
	.poster-link {
		position: absolute;
		inset: 0;
		display: block;
		border-radius: 4px;
	}
	.card:hover .poster-link {
		background-color: rgba(30, 30, 30, 0.6);
		box-shadow:
			0 0 0 1px var(--color-brand-accent),
			0 0 4px rgba(0, 0, 0, 0.3);
	}
	.poster-link.selected,
	.card:hover .poster-link.selected {
		background-color: rgba(30, 30, 30, 0.6);
		box-shadow:
			0 0 0 2px var(--color-brand-accent),
			0 0 4px rgba(0, 0, 0, 0.3);
	}

	/* Plex: MetadataPosterCardActions. Hidden until the card is hovered. */
	.actions {
		position: absolute;
		inset: 0;
		pointer-events: none;
	}
	.action {
		position: absolute;
		display: flex;
		align-items: center;
		justify-content: center;
		width: 32px;
		height: 32px;
		color: hsla(0, 0%, 100%, 0.7);
		font-size: 14px;
		line-height: 14px;
		opacity: 0;
		pointer-events: auto;
		transition: all 0.2s;
	}
	.card:hover .action,
	.action:focus-visible,
	.select.is-selected {
		opacity: 1;
	}
	.action:hover {
		color: #fff;
	}

	.select {
		top: 0;
		left: 0;
		cursor: default;
	}
	.select-circle {
		display: grid;
		place-items: center;
		box-sizing: border-box;
		width: 14px;
		height: 14px;
		border: 2px solid hsla(0, 0%, 100%, 0.7);
		border-radius: 50%;
		transition: all 0.2s;
	}
	.select:hover .select-circle {
		border-color: #fff;
	}
	.select.is-selected .select-circle {
		color: rgba(0, 0, 0, 0.75);
		background-color: var(--color-brand-accent);
		border-color: var(--color-brand-accent);
		box-shadow: 0 0 4px rgba(0, 0, 0, 0.6);
		transform: scale(1.4);
	}

	.play {
		top: 50%;
		left: 50%;
		transform: translate(-50%, -50%);
	}
	.play-circle {
		display: block;
		box-sizing: border-box;
		width: 42px;
		height: 42px;
		flex-shrink: 0;
		border: 2px solid hsla(0, 0%, 100%, 0.7);
		border-radius: 50%;
		background-color: rgba(0, 0, 0, 0.45);
		color: hsla(0, 0%, 100%, 0.7);
		overflow: hidden;
		transition: all 0.2s;
	}
	.play-circle :global(svg) {
		margin: -2px;
	}
	.play:hover .play-circle {
		background-color: var(--color-brand-accent);
		border-color: var(--color-brand-accent);
		color: #1f2326;
	}

	.more {
		right: 0;
		bottom: 0;
	}
	.more :global(.trigger) {
		justify-content: center;
		width: 32px;
		height: 32px;
		color: inherit;
	}

	/* Plex: MetadataPosterCardTitle. */
	.title,
	.detail {
		display: block;
		height: 24px;
		padding: 0 4px;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		font-size: 13px;
		line-height: 24px;
	}
	.title {
		color: #fff;
	}
	.title:hover {
		text-decoration: underline;
	}
	.detail,
	.detail a {
		color: hsla(0, 0%, 100%, 0.45);
	}
	a.detail:hover,
	.detail a:hover {
		color: #fff;
		text-decoration: underline;
	}
	/* Plex: DashSeparator. */
	.separator {
		margin: 0 4px;
	}
</style>
