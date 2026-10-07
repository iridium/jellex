<script lang="ts">
	import { fadeIn } from '#lib/motion.ts';
	import type { BaseItemDto } from '@jellyfin/sdk/lib/generated-client';
	import { getLibraryApi } from '@jellyfin/sdk/lib/utils/api/library-api';
	import { getUserDataApi } from '@jellyfin/sdk/lib/utils/api/user-data-api';
	import { goto, invalidate } from '$app/navigation';
	import CastCard from '#lib/components/CastCard.svelte';
	import Hub from '#lib/components/Hub.svelte';
	import Icon from '#lib/components/Icon.svelte';
	import { personHref } from '#lib/library.ts';
	import type { BaseItemPerson } from '@jellyfin/sdk/lib/generated-client';
	import MultiselectBar from '#lib/components/MultiselectBar.svelte';
	import { selection } from '#lib/selection.svelte.ts';
	import Menu from '#lib/components/Menu.svelte';
	import PageHeader from '#lib/components/PageHeader.svelte';
	import PlaylistPage from '#lib/components/PlaylistPage.svelte';
	import StarRating from '#lib/components/StarRating.svelte';
	import PosterCard from '#lib/components/PosterCard.svelte';
	import TrackList from '#lib/components/TrackList.svelte';
	import {
		duration,
		episodeCode,
		longDate,
		plural,
		ticksToSeconds,
		timeLeft
	} from '#lib/format.ts';
	import { imageUrl, type ImageKind } from '#lib/images.ts';
	import { streams } from '#lib/item.ts';
	import { session } from '#lib/session.svelte.ts';
	import { itemMenu, loadPlaylists } from '#lib/actions.svelte.ts';
	import { player } from '#lib/player.svelte.ts';
	import { tintFrom } from '#lib/ultrablur.ts';
	import { background } from '#lib/ui.svelte.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();

	const d = $derived(data.details);
	const item = $derived(d.item);
	const type = $derived(item.Type ?? '');

	// Plex tints the backdrop from the artwork on every details page.
	$effect(() => tintFrom(item));

	// Poster column: 250px posters and squares, 300px episode thumbnails.
	const posterKind: ImageKind = $derived(
		type === 'Episode' || type === 'Video'
			? 'landscape'
			: ['MusicAlbum', 'MusicArtist', 'Playlist', 'Audio'].includes(type)
				? 'square'
				: 'poster'
	);
	const posterWidth = $derived(posterKind === 'landscape' ? 300 : 250);
	const posterSrc = $derived(imageUrl(item, posterKind, posterWidth));

	// Titles: Plex shows the series or artist as the title and the season,
	// episode or album as the subtitle.
	const title = $derived(
		type === 'Episode' || type === 'Season'
			? (item.SeriesName ?? item.Name)
			: type === 'MusicAlbum'
				? (item.AlbumArtist ?? item.Name)
				: item.Name
	);
	const subtitle = $derived(
		type === 'Episode' || type === 'Season' || type === 'MusicAlbum' ? item.Name : undefined
	);
	const directors = $derived((item.People ?? []).filter((p) => p.Type === 'Director'));
	const writers = $derived((item.People ?? []).filter((p) => p.Type === 'Writer'));
	const cast = $derived(
		(item.People ?? []).filter((p) => p.Type !== 'Director' || type !== 'Episode').slice(0, 30)
	);
	const runtime = $derived(item.RunTimeTicks ? duration(ticksToSeconds(item.RunTimeTicks)) : '');
	const left = $derived(timeLeft(item));
	const progress = $derived(item.IsFolder ? 0 : (item.UserData?.PlayedPercentage ?? 0));
	const played = $derived(!!item.UserData?.Played);
	const favorite = $derived(!!item.UserData?.IsFavorite);

	// What Play starts, and whether Plex would call it Resume.
	const playTarget = $derived(type === 'Series' ? d.nextUp : item);
	const resumable = $derived((playTarget?.UserData?.PlaybackPositionTicks ?? 0) > 0);
	const status = $derived(
		type === 'Series' && d.nextUp ? `On Deck — ${episodeCode(d.nextUp)}` : left
	);

	// Stream choices carry over to the player.
	const s = $derived(streams(item));
	let audioIndex = $state<number | undefined>(undefined);
	let subtitleIndex = $state<number | undefined>(undefined);
	const audio = $derived(
		s.audio.find((a) => a.Index === (audioIndex ?? s.defaultAudio)) ?? s.audio[0]
	);
	const subtitleStream = $derived(
		s.subtitles.find((t) => t.Index === (subtitleIndex ?? s.defaultSubtitle))
	);
	// Fades the composited background in once its art has loaded (Plex's
	// 600ms linear image fade).
	function fadeInOnLoad(el: HTMLElement) {
		const img = el.querySelector('img')!;
		const show = () => {
			el.style.opacity = '1';
			el.animate([{ opacity: 0 }, { opacity: 1 }], { duration: 600, easing: 'linear' });
		};
		if (img.complete && img.naturalWidth) show();
		else img.addEventListener('load', show, { once: true });
	}

	// Plex's header extras: Show Background (items with art) and the
	// Previous / list / Next navigation through siblings.
	const art = $derived(imageUrl(item, 'backdrop', 1920));
	const siblingLabel = (s: BaseItemDto) =>
		s.Type === 'Episode' && s.IndexNumber != null
			? `E${s.IndexNumber} — ${s.Name ?? ''}`
			: (s.Name ?? '');
	const siblingNav = $derived.by(() => {
		const list = d.siblings;
		const at = list.findIndex((s) => s.Id === item.Id);
		if (list.length < 2 || at < 0) return undefined;
		return {
			previous: at > 0 ? `/items/${list[at - 1].Id}` : undefined,
			next: at < list.length - 1 ? `/items/${list[at + 1].Id}` : undefined,
			items: list.map((s) => ({
				label: siblingLabel(s),
				active: s.Id === item.Id,
				onselect: () => goto(`/items/${s.Id}`)
			}))
		};
	});
	const playable = $derived(
		!!(
			playTarget ||
			d.episodes.length ||
			d.tracks.length ||
			d.children.length ||
			d.albums.length ||
			d.seasons.length
		)
	);
	function play(fromStart = false) {
		const own = playTarget === item;
		// Plex's Resume button asks "Resume Playback" too.
		player.play(playTarget ?? item, {
			fromStart,
			askResume: !fromStart,
			audio: own ? audioIndex : undefined,
			subtitle: own ? subtitleIndex : undefined
		});
	}

	let expanded = $state(false);
	let summary: HTMLElement | undefined = $state();
	let clamped = $state(false);
	let fullHeight = $state(0);
	const collapsedHeight = 73;
	$effect(() => {
		void item.Overview;
		expanded = false;
		if (!summary) return;
		clamped = summary.scrollHeight > summary.clientHeight + 1;
		const measure = () => {
			const clone = summary!.cloneNode(true) as HTMLElement;
			clone.classList.add('expanded');
			clone.style.cssText =
				'position:absolute;visibility:hidden;width:' + summary!.clientWidth + 'px';
			summary!.parentElement!.appendChild(clone);
			fullHeight = Math.ceil(clone.scrollHeight);
			clone.remove();
		};
		measure();
	});

	const userData = $derived(getUserDataApi(session.requireApi));
	async function toggleWatched() {
		const p = { itemId: item.Id!, userId: session.userId };
		await (played ? userData.markUnplayedItem(p) : userData.markPlayedItem(p));
		await invalidate('jellex:item');
	}
	async function toggleFavorite() {
		const p = { itemId: item.Id!, userId: session.userId };
		await (favorite ? userData.unmarkFavoriteItem(p) : userData.markFavoriteItem(p));
		await invalidate('jellex:item');
	}
	const hasTrailer = $derived(
		(item.LocalTrailerCount ?? 0) > 0 || (item.RemoteTrailers?.length ?? 0) > 0
	);
	async function playTrailer() {
		if ((item.LocalTrailerCount ?? 0) > 0) {
			const { data: trailers } = await getLibraryApi(session.requireApi).getLocalTrailers({
				itemId: item.Id!,
				userId: session.userId
			});
			if (trailers[0]) return player.play(trailers[0]);
		}
		const url = item.RemoteTrailers?.[0]?.Url;
		if (url) window.open(url, '_blank', 'noopener');
	}

	const genreHref = (g: string) =>
		d.library ? `/library/${d.library.Id}?pivot=library&genre=${encodeURIComponent(g)}` : undefined;

	const streamLabel = (t: { DisplayTitle?: string | null; Title?: string | null }) =>
		t.DisplayTitle ?? t.Title ?? 'Unknown';
	const seasonLines = (s: BaseItemDto) => [s.Name ?? '', plural(s.ChildCount ?? 0, 'episode')];
</script>

<svelte:head>
	<title>{subtitle ? `${title} – ${subtitle}` : title} · jellex</title>
</svelte:head>

{#if type === 'Playlist'}
	<PlaylistPage playlist={item} entries={d.children} serverName={data.serverName} />
{:else}
	<PageHeader
		background={art ? { shown: background.shown, ontoggle: () => background.toggle() } : undefined}
		nav={siblingNav}
		title={d.library?.Name ?? 'Home'}
		detail={data.serverName}
		href={d.library ? `/library/${d.library.Id}` : '/'}
		slider
		menu={d.library
			? [
					{ label: 'Play', onselect: () => player.play(d.library!) },
					{ label: 'Shuffle', onselect: () => player.play(d.library!, { shuffle: true }) }
				]
			: undefined}
	/>

	<!-- Plex's Show Background: the art, composited the way PMS does it for
     Plex Web's opacity=10&background=343a3f request, over the whole window;
     the finished image fades in like Plex's other images. -->
	{#if background.shown && art}
		{#key art}
			<div class="art-background" use:fadeInOnLoad>
				<img src={art} alt="" />
			</div>
		{/key}
	{/if}

	{#snippet people(list: BaseItemPerson[])}
		{#each list as person, i (person.Id ?? i)}{#if i},
			{/if}{@const href = personHref(d.library?.Id, person)}{#if href}<a class="person-link" {href}
					>{person.Name}</a
				>{:else}{person.Name}{/if}{/each}
	{/snippet}

	{#if selection.count}<MultiselectBar />{/if}
	<div class="page-content scroller">
		<div class="content">
			<div class="metadata">
				<!-- Poster column. -->
				<div class="poster-column" style:width="{posterWidth}px">
					<div
						class="poster"
						class:landscape={posterKind === 'landscape'}
						class:square={posterKind === 'square'}
					>
						{#if posterSrc}
							<img src={posterSrc} alt="" use:fadeIn />
						{:else}
							<span class="poster-initials">{(item.Name ?? '').slice(0, 2).toUpperCase()}</span>
						{/if}
						{#if progress > 0 && progress < 100}
							<div class="progress">
								<div class="bar" style:transform="translate3d({progress - 100}%, 0, 0)"></div>
							</div>
						{/if}
						<!-- Plex: MetadataPosterCardActions-playCoverButton. On hover the
					     whole poster becomes the Play/Resume button. -->
						{#if playable}
							<button
								class="play-cover"
								type="button"
								aria-label={resumable ? 'Resume' : 'Play'}
								onclick={() => play()}
							>
								<span class="play-circle"><Icon name="play" size={42} /></span>
							</button>
						{/if}
					</div>
					{#if status}
						<div class="played-status">{status}</div>
					{/if}
				</div>

				<!-- Metadata column. -->
				<div class="info">
					<div class="head">
						<div class="titles">
							<h1>{title}</h1>
							{#if subtitle}<h2>{subtitle}</h2>{/if}
							{#if directors.length && type === 'Movie'}
								<div class="byline">Directed by {@render people(directors)}</div>
							{/if}
						</div>
						<div class="meta">
							{#if type === 'Episode'}
								<div class="meta-row">
									{#if item.ParentIndexNumber != null}<span>Season {item.ParentIndexNumber}</span
										>{/if}
									{#if item.IndexNumber != null}<span>Episode {item.IndexNumber}</span>{/if}
									{#if left}<span>{left}</span>{/if}
								</div>
								<div class="meta-row">
									{#if item.PremiereDate}<span>{longDate(item.PremiereDate)}</span>{/if}
									{#if runtime}<span>{runtime}</span>{/if}
								</div>
							{:else if type !== 'Season'}
								<div class="meta-row">
									{#if item.OfficialRating}
										<span class="rating" aria-label="Rated {item.OfficialRating}"
											>{item.OfficialRating}</span
										>
									{/if}
									{#if item.ProductionYear}<span>{item.ProductionYear}</span>{/if}
									{#if runtime && type !== 'Series'}<span>{runtime}</span>{/if}
									{#if item.Genres?.length}
										<span class="genres">
											{#each item.Genres.slice(0, 3) as g, i (g)}
												<a class="genre" href={genreHref(g)}>{g}</a>{i <
												Math.min(item.Genres.length, 3) - 1
													? ', '
													: ''}
											{/each}
										</span>
									{/if}
								</div>
							{/if}
							<div class="meta-row stars"><StarRating {item} /></div>
						</div>
					</div>

					<div class="actions">
						{#if playable}
							<button class="play-button" type="button" onclick={() => play()}>
								<Icon name="play-solid" size={24} />
								<span>{resumable ? 'Resume' : 'Play'}</span>
							</button>
						{/if}
						{#if type === 'MusicAlbum'}
							<button
								class="icon-button"
								type="button"
								aria-label="Shuffle"
								title="Shuffle"
								onclick={() => player.play(item, { shuffle: true })}
							>
								<Icon name="shuffle" size={24} />
							</button>
						{/if}
						{#if hasTrailer}
							<button
								class="icon-button"
								type="button"
								aria-label="Play Trailer"
								title="Play Trailer"
								onclick={playTrailer}
							>
								<Icon name="trailer" size={24} />
							</button>
						{/if}
						{#if !['MusicAlbum', 'MusicArtist', 'Playlist', 'Audio'].includes(type)}
							<button
								class="icon-button"
								class:on={played}
								type="button"
								aria-label={played ? 'Mark as Unwatched' : 'Mark as Watched'}
								title={played ? 'Mark as Unwatched' : 'Mark as Watched'}
								onclick={toggleWatched}
							>
								<Icon name="watched" size={24} />
							</button>
						{/if}
						<div class="icon-button more">
							<Menu
								variant="plain"
								onopen={loadPlaylists}
								items={[
									...(resumable && playTarget
										? [{ label: 'Play from Beginning', onselect: () => play(true) }]
										: []),
									...itemMenu(item)
								]}
							>
								{#snippet trigger()}<Icon name="more-horizontal" size={24} label="More" />{/snippet}
							</Menu>
						</div>
					</div>

					{#if item.Overview}
						<div class="summary-block">
							<!-- Plex: the clamp wrapper animates max-height (0.3s) between the
						     three-line height and the full text. -->
							<div
								class="summary-clamp"
								style:max-height="{expanded ? fullHeight : collapsedHeight}px"
							>
								<div class="summary" class:expanded bind:this={summary}>{item.Overview}</div>
							</div>
							{#if clamped || expanded}
								<button class="more-toggle" type="button" onclick={() => (expanded = !expanded)}>
									{expanded ? 'Less' : 'More'}
									<span class="chevron" class:up={expanded}
										><Icon name="chevron-down" size={16} /></span
									>
								</button>
							{/if}
						</div>
					{/if}

					{#if s.video || s.audio.length || (type === 'Episode' && (writers.length || directors.length))}
						<div class="tables">
							{#if type === 'Episode' && (writers.length || directors.length)}
								<div class="info-table">
									{#if directors.length}
										<div class="row">
											<span class="label">Directed by</span><span class="value"
												>{@render people(directors)}</span
											>
										</div>
									{/if}
									{#if writers.length}
										<div class="row">
											<span class="label">Written by</span><span class="value"
												>{@render people(writers)}</span
											>
										</div>
									{/if}
								</div>
							{/if}
							{#if s.video || s.audio.length}
								<div class="info-table">
									{#if s.video}
										<div class="row">
											<span class="label">Video</span><span class="value"
												>{streamLabel(s.video)}</span
											>
										</div>
									{/if}
									{#if s.audio.length}
										<div class="row">
											<span class="label">Audio</span>
											<span class="value">
												{#if s.audio.length > 1}
													<span class="stream-menu">
														<Menu
															label={audio ? streamLabel(audio) : 'None'}
															size="large"
															offset={0}
															items={s.audio.map((a) => ({
																label: streamLabel(a),
																active: a === audio,
																onselect: () => (audioIndex = a.Index ?? undefined)
															}))}
														/>
													</span>
												{:else}{streamLabel(s.audio[0])}{/if}
											</span>
										</div>
									{/if}
									{#if s.video}
										<div class="row">
											<span class="label">Subtitles</span>
											<span class="value stream-menu">
												<Menu
													label={subtitleStream ? streamLabel(subtitleStream) : 'None'}
													size="large"
													offset={0}
													items={[
														{
															label: 'None',
															active: !subtitleStream,
															onselect: () => (subtitleIndex = -1)
														},
														...s.subtitles.map((t) => ({
															label: streamLabel(t),
															active: t === subtitleStream,
															onselect: () => (subtitleIndex = t.Index ?? -1)
														}))
													]}
												/>
											</span>
										</div>
									{/if}
								</div>
							{/if}
						</div>
					{/if}
				</div>
			</div>

			<!-- Children and related rows, in Plex's order. -->
			{#if d.seasons.length}
				<Hub title="Seasons">
					{#each d.seasons as season (season.Id)}
						<PosterCard item={season} lines={seasonLines(season)} />
					{/each}
				</Hub>
			{/if}

			{#if d.tracks.length}
				<TrackList tracks={d.tracks} />
			{/if}

			{#if cast.length && type !== 'MusicAlbum' && type !== 'MusicArtist'}
				<Hub title="Cast & Crew">
					{#each cast as person, i (`${person.Id}-${person.Type}-${i}`)}
						<CastCard {person} libraryId={d.library?.Id} />
					{/each}
				</Hub>
			{/if}

			{#if d.episodes.length}
				<section class="list">
					<h2 class="list-title">{plural(d.episodes.length, 'Episode')}</h2>
					<div class="wrap-grid landscape">
						{#each d.episodes as ep (ep.Id)}
							<PosterCard
								item={ep}
								kind="landscape"
								lines={[ep.Name ?? '', ep.IndexNumber != null ? `Episode ${ep.IndexNumber}` : '']}
							/>
						{/each}
					</div>
				</section>
			{/if}

			{#if d.albums.length}
				<section class="list">
					<h2 class="list-title">{plural(d.albums.length, 'Album')}</h2>
					<div class="wrap-grid">
						{#each d.albums as album (album.Id)}
							<PosterCard
								item={album}
								kind="square"
								lines={[album.Name ?? '', album.ProductionYear ? String(album.ProductionYear) : '']}
							/>
						{/each}
					</div>
				</section>
			{/if}

			{#if d.children.length}
				<section class="list">
					<h2 class="list-title">{plural(d.children.length, 'Item')}</h2>
					<div class="wrap-grid">
						{#each d.children as child (child.Id)}
							<PosterCard item={child} />
						{/each}
					</div>
				</section>
			{/if}

			{#if d.similar.length}
				<Hub title="More Like This">
					{#each d.similar as other (other.Id)}
						<PosterCard item={other} kind={posterKind === 'landscape' ? 'poster' : posterKind} />
					{/each}
				</Hub>
			{/if}
		</div>
	</div>
{/if}

<style>
	.art-background {
		position: fixed;
		inset: 0;
		z-index: -2;
		pointer-events: none;
	}
	.art-background img {
		width: 100%;
		height: 100%;
		object-fit: cover;
	}
	/* PMS's photo transcoder (PhotoTranscodeRequestHandler): for opacity
	   below 100 it fills a new image with the background colour and pastes
	   the art on it with FreeImage_Paste at alpha = opacity * 256 / 100,
	   so opacity=10 gives 25/256 of the art over #343a3f. */
	.art-background {
		background-color: #343a3f;
		opacity: 0;
	}
	.art-background img {
		opacity: calc(25 / 256);
	}
	/* Plex: the crew's tag links. */
	.person-link {
		color: inherit;
	}
	.person-link:hover {
		text-decoration: underline;
	}
	.page-content {
		flex: 1;
		min-height: 0;
		overflow: hidden auto;
	}
	/* Plex: PrePlayPageContent. */
	.content {
		padding: 40px 0;
	}
	/* Plex: PrePlayMetadata. */
	.metadata {
		display: flex;
		gap: 48px;
		margin-bottom: 40px;
		padding: 0 var(--page-gutter);
	}
	.poster-column {
		display: flex;
		flex-direction: column;
		flex-shrink: 0;
	}
	.poster {
		position: relative;
		display: grid;
		place-items: center;
		aspect-ratio: 2 / 3;
		border-radius: 4px;
		background-color: rgba(0, 0, 0, 0.45);
		box-shadow: 0 0 4px rgba(0, 0, 0, 0.3);
		overflow: hidden;
	}
	.poster.square {
		aspect-ratio: 1;
	}
	.poster.landscape {
		aspect-ratio: 16 / 9;
	}
	.poster img {
		width: 100%;
		height: 100%;
		object-fit: cover;
	}
	/* Plex: PosterCardLink-hoveredLink + MetadataPosterCardActions. */
	.play-cover {
		position: absolute;
		inset: 0;
		display: none;
		place-items: center;
		border-radius: 4px;
		background-color: rgba(30, 30, 30, 0.6);
		box-shadow:
			0 0 0 1px var(--color-brand-accent),
			0 0 4px rgba(0, 0, 0, 0.3);
		color: #fff;
		transition: all 0.2s;
	}
	.poster:hover .play-cover {
		display: grid;
	}
	.poster:hover .progress {
		opacity: 0;
	}
	.play-circle {
		display: block;
		box-sizing: border-box;
		width: 42px;
		height: 42px;
		overflow: hidden;
		border: 2px solid var(--color-brand-accent);
		border-radius: 50%;
		background-color: var(--color-brand-accent);
		color: #1f2326;
	}
	.play-circle :global(svg) {
		margin: -2px;
	}
	.poster-initials {
		color: hsla(0, 0%, 100%, 0.2);
		font-size: 48px;
	}
	.progress {
		position: absolute;
		left: 0;
		right: 0;
		bottom: 0;
		height: 4px;
		overflow: hidden;
		border-radius: 2px;
		background-color: #1f2326;
	}
	.bar {
		height: 100%;
		background-color: var(--color-brand-accent);
		transition: transform 0.6s ease-in-out;
	}
	/* Plex: PrePlayPosterCardPlayedStatus. */
	.played-status {
		margin-top: -4px;
		padding: 14px 10px 10px;
		border-radius: 0 0 4px 4px;
		background-color: rgba(0, 0, 0, 0.15);
		color: rgba(250, 250, 250, 0.75);
		text-align: center;
	}

	.info {
		display: flex;
		flex-direction: column;
		gap: 32px;
		max-width: 700px;
		min-width: 0;
	}
	.head {
		display: flex;
		flex-direction: column;
		gap: 16px;
	}
	h1 {
		margin: 0;
		color: #fff;
		font-family: var(--font-heading);
		font-size: 32px;
		font-weight: 700;
		line-height: 40px;
	}
	.titles h2 {
		margin: 0;
		color: rgba(255, 255, 255, 0.8);
		font-family: var(--font-heading);
		font-size: var(--font-heading-2-font-size);
		font-weight: 700;
		line-height: 32px;
	}
	.byline {
		color: rgba(255, 255, 255, 0.6);
		font-size: 12px;
		line-height: 16px;
	}
	.meta {
		display: flex;
		flex-direction: column;
		gap: 4px;
	}
	.stars {
		height: 24px;
	}
	.meta-row {
		display: flex;
		align-items: center;
		flex-wrap: wrap;
		gap: 12px;
		color: rgba(255, 255, 255, 0.8);
		font-size: 14px;
		line-height: 20px;
	}
	/* Plex: PrePlayContentRating-badge. */
	.rating {
		padding: 3px 7px;
		border-radius: 5px;
		background-color: #191919;
		color: #fff;
		font-weight: 700;
		line-height: 14px;
		text-transform: uppercase;
	}
	.genre:hover {
		color: #fff;
	}

	.actions {
		display: flex;
		align-items: center;
		gap: 8px;
	}
	/* Plex's primary button (accent, 40px). */
	.play-button {
		display: inline-flex;
		align-items: center;
		gap: 8px;
		min-height: 40px;
		padding: 0 16px;
		border-radius: 4px;
		background-color: var(--color-background-accent);
		color: var(--color-text-on-accent);
		font-size: 16px;
		font-weight: 600;
		line-height: 24px;
		transition: all var(--duration-fast);
	}
	.play-button:hover {
		background-color: var(--color-background-accent-focus);
	}
	/* Plex's clear icon buttons (56×40). */
	.icon-button {
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
	.icon-button:hover {
		background-color: var(--color-background-control-focus);
	}
	.icon-button.on {
		color: var(--color-brand-accent);
	}
	.icon-button.more {
		padding: 0;
	}
	.icon-button.more :global(.trigger) {
		min-width: 56px;
		min-height: 40px;
		justify-content: center;
		color: #fff;
	}

	.summary-block {
		display: flex;
		flex-direction: column;
		gap: 0;
	}
	.summary-clamp {
		overflow: hidden;
		transition: max-height 0.3s;
	}
	.summary {
		display: -webkit-box;
		-webkit-box-orient: vertical;
		-webkit-line-clamp: 3;
		line-clamp: 3;
		overflow: hidden;
		color: #fff;
		font-size: 16px;
		line-height: 24px;
		white-space: pre-line;
	}
	.summary.expanded {
		display: block;
		-webkit-line-clamp: unset;
		line-clamp: unset;
	}
	.more-toggle {
		display: inline-flex;
		align-items: center;
		gap: 4px;
		align-self: flex-start;
		color: var(--color-brand-accent);
		font-size: 14px;
		font-weight: 600;
		line-height: 20px;
		transition: color 0.1s;
	}
	.more-toggle:hover {
		color: #fff;
	}
	.chevron {
		display: flex;
		transition: transform 0.3s;
	}
	/* Plex flips the chevron with rotateX. */
	.chevron.up {
		transform: rotateX(180deg);
	}

	/* Plex: StreamDetailPropertiesTable. */
	.tables {
		display: flex;
		gap: 32px;
	}
	.info-table {
		display: flex;
		flex-direction: column;
		gap: 8px;
	}
	.row {
		display: flex;
		gap: 24px;
		font-size: 14px;
		line-height: 20px;
	}
	.label {
		flex-shrink: 0;
		width: 95px;
		color: rgba(255, 255, 255, 0.6);
	}
	.value {
		color: #fff;
	}
	.stream-menu :global(.trigger) {
		color: var(--color-brand-accent);
		font-size: 14px;
		line-height: 20px;
	}
	.stream-menu :global(.trigger .arrow) {
		border-top-color: var(--color-brand-accent);
	}

	/* Plex: PrePlayListTitle + wrapped poster lists. */
	.list {
		margin-bottom: 40px;
		padding: 0 var(--page-gutter);
	}
	.list-title {
		margin: 0;
		padding-bottom: 20px;
		color: #fff;
		font-family: var(--font-heading-2-font-family);
		font-size: var(--font-heading-2-font-size);
		font-weight: 700;
		line-height: 32px;
	}
	.wrap-grid {
		display: flex;
		flex-wrap: wrap;
		gap: 30px;
	}
</style>
