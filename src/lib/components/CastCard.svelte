<script lang="ts">
	import { personHref } from '#lib/library.ts';
	import { fadeIn } from '#lib/motion.ts';
	import type { BaseItemPerson } from '@jellyfin/sdk/lib/generated-client';
	import { session } from '#lib/session.svelte.ts';

	// Plex Web's cast & crew card: a 165px circle (photo or initials), the
	// name, and the role underneath.
	let { person, libraryId }: { person: BaseItemPerson; libraryId?: string } = $props();
	const href = $derived(personHref(libraryId, person));

	const src = $derived.by(() => {
		if (!person.Id || !person.PrimaryImageTag) return undefined;
		const dpr = Math.min(window.devicePixelRatio || 1, 2);
		const size = Math.round(165 * dpr);
		return `${session.requireApi.basePath}/Items/${person.Id}/Images/Primary?fillWidth=${size}&fillHeight=${size}&quality=90&tag=${person.PrimaryImageTag}`;
	});
	const initials = $derived(
		(person.Name ?? '')
			.split(/\s+/)
			.filter(Boolean)
			.slice(0, 2)
			.map((w) => w[0])
			.join('')
			.toUpperCase()
	);
	// Plex labels crew by department, as TMDB does.
	const departments: Record<string, string> = {
		Director: 'Directing',
		Writer: 'Writing',
		Producer: 'Production',
		Composer: 'Sound',
		Conductor: 'Sound',
		Lyricist: 'Writing',
		Arranger: 'Sound',
		Editor: 'Editing',
		Engineer: 'Crew',
		Mixer: 'Sound'
	};
	const role = $derived(
		person.Type === 'Actor' || person.Type === 'GuestStar'
			? (person.Role ?? '')
			: (departments[person.Type ?? ''] ?? person.Role ?? person.Type ?? '')
	);
</script>

<!-- Plex: the photo has its own overlay link (hover draws an inset ring)
     and the name is a separate link that underlines on hover. -->
<div class="cast-card">
	<div class="circle">
		{#if src}
			<img {src} alt="" loading="lazy" decoding="async" use:fadeIn />
		{:else}
			<span class="initials">{initials}</span>
		{/if}
		{#if href}<a class="overlay" {href} aria-label={person.Name}></a>{/if}
	</div>
	{#if href}
		<a class="name" {href}>{person.Name}</a>
	{:else}
		<div class="name">{person.Name}</div>
	{/if}
	{#if role}<div class="role">{role}</div>{/if}
</div>

<style>
	.cast-card {
		display: flex;
		flex-direction: column;
		align-items: center;
		width: 165px;
		flex-shrink: 0;
		text-align: center;
	}
	.circle {
		position: relative;
		display: grid;
		place-items: center;
		width: 165px;
		height: 165px;
		margin-bottom: 8px;
		overflow: hidden;
		border-radius: 9999px;
		background-color: rgba(255, 255, 255, 0.1);
		filter: drop-shadow(0 2px 2px rgba(0, 0, 0, 0.3));
	}
	/* Plex: the card image link; no transition. */
	.overlay {
		position: absolute;
		inset: 0;
		border-radius: var(--border-radius-max);
	}
	.overlay:hover {
		box-shadow:
			0 0 0 1px inset var(--color-background-focus),
			0 0 0 3px inset var(--color-text-on-focus);
	}
	img {
		width: 100%;
		height: 100%;
		object-fit: cover;
	}
	.initials {
		color: hsla(0, 0%, 100%, 0.45);
		font-family: var(--font-heading);
		font-size: 48px;
		font-weight: 500;
		line-height: 1;
	}
	.name,
	.role {
		max-width: 100%;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		font-size: 14px;
		line-height: 20px;
	}
	.name {
		padding: 3.6px 0;
		color: rgba(255, 255, 255, 0.8);
		transition: color 0.1s;
	}
	a.name:hover {
		color: var(--color-text-primary);
		text-decoration: underline;
	}
	.role {
		color: rgba(255, 255, 255, 0.6);
	}
</style>
