<script lang="ts">
	import { player, QUALITIES } from '#lib/player.svelte.ts';
	import { streams } from '#lib/item.ts';
	import Menu, { type MenuItem } from '../Menu.svelte';

	// Plex Web's AudioVideoPlaybackSettings: "PLAYBACK SETTINGS" over rows
	// of label (right-aligned, uppercase) and value (accent dropdowns).
	let { subtitleSize = $bindable() }: { subtitleSize: number } = $props();

	const item = $derived(player.item);
	const s = $derived(item ? streams(item) : null);
	const source = $derived(player.stream?.source);
	const audio = $derived(
		s?.audio.find((a) => a.Index === (player.audioIndex ?? s?.defaultAudio)) ?? s?.audio[0]
	);
	const subtitle = $derived(
		s?.subtitles.find((t) => t.Index === (player.subtitleIndex ?? s?.defaultSubtitle))
	);
	let showAll = $state(false);

	function mbps(bits: number | null | undefined) {
		if (!bits) return '';
		return bits >= 1e6
			? `${(bits / 1e6).toFixed(bits >= 1e7 ? 0 : 1)} Mbps`
			: `${Math.round(bits / 1e3)} kbps`;
	}
	function resolution() {
		const h = s?.video?.Height ?? 0;
		if (h >= 2000) return '4K';
		if (h >= 1000) return '1080p HD';
		if (h >= 700) return '720p HD';
		return h ? `${h}p SD` : '';
	}
	const originalDetail = $derived([mbps(source?.Bitrate), resolution()].filter(Boolean).join(', '));
	const current = $derived(QUALITIES.find((q) => q.bitrate === player.maxBitrate));
	const qualityLabel = $derived(
		current
			? `Convert to ${current.label} (${current.detail})`
			: `Original${originalDetail ? ` (${originalDetail})` : ''}`
	);
	const qualityItems = $derived<MenuItem[]>([
		{
			label: 'Original',
			detail: originalDetail,
			active: player.maxBitrate === null,
			onselect: () => player.setQuality(null)
		},
		{ label: 'Convert Automatically', onselect: () => player.setQuality(null) },
		...(showAll
			? QUALITIES.map((q) => ({
					label: `Convert to ${q.label}`,
					detail: q.detail,
					active: player.maxBitrate === q.bitrate,
					onselect: () => player.setQuality(q.bitrate)
				}))
			: [{ label: 'Show all', keepOpen: true, onselect: () => (showAll = true) }])
	]);
	const sizes = [
		{ label: 'Tiny', value: 0.6 },
		{ label: 'Small', value: 0.8 },
		{ label: 'Normal', value: 1 },
		{ label: 'Large', value: 1.3 },
		{ label: 'Huge', value: 1.6 }
	];
	const label = (t: { DisplayTitle?: string | null; Title?: string | null }) =>
		t.DisplayTitle ?? t.Title ?? 'Unknown';
</script>

<div class="settings">
	<div class="title">Playback Settings</div>
	<div class="rows">
		{#if !player.isAudio}
			<div class="row">
				<div class="label">Quality</div>
				<div class="cell value">
					<Menu label={qualityLabel} size="auto" placement="above" items={qualityItems} />
				</div>
			</div>
		{/if}
		<div class="row">
			<div class="label">Audio Stream</div>
			<div class="cell">
				{#if s && s.audio.length > 1}
					<span class="value">
						<Menu
							label={audio ? label(audio) : 'None'}
							size="auto"
							placement="above"
							items={s.audio.map((a) => ({
								label: label(a),
								active: a === audio,
								onselect: () => player.setAudio(a.Index!)
							}))}
						/>
					</span>
				{:else}
					{audio ? label(audio) : 'None'}
				{/if}
			</div>
		</div>
		{#if !player.isAudio}
			<div class="row">
				<div class="label">Subtitles</div>
				<div class="cell value">
					<Menu
						label={subtitle ? label(subtitle) : 'None'}
						size="auto"
						placement="above"
						items={[
							{ label: 'None', active: !subtitle, onselect: () => player.setSubtitle(-1) },
							...(s?.subtitles ?? []).map((t) => ({
								label: label(t),
								active: t === subtitle,
								onselect: () => player.setSubtitle(t.Index!)
							}))
						]}
					/>
				</div>
			</div>
			<div class="row" class:disabled={!subtitle}>
				<div class="label">Subtitle Size</div>
				<div class="cell value">
					<Menu
						label={sizes.find((z) => z.value === subtitleSize)?.label ?? 'Normal'}
						size="auto"
						placement="above"
						items={sizes.map((z) => ({
							label: z.label,
							active: z.value === subtitleSize,
							onselect: () => (subtitleSize = z.value)
						}))}
					/>
				</div>
			</div>
		{/if}
		<div class="row">
			<div class="label">Auto Play</div>
			<div class="cell">
				<label class="check">
					<input
						type="checkbox"
						checked={player.autoplay}
						onchange={(e) => player.setAutoplay(e.currentTarget.checked)}
					/>
				</label>
			</div>
		</div>
	</div>
</div>

<style>
	.settings {
		position: absolute;
		z-index: 3;
		left: 0;
		right: 0;
		bottom: 100px;
		display: flex;
		flex-direction: column;
		align-items: center;
		padding: 20px 0 24px;
		background-color: rgba(35, 36, 38, 0.8);
		box-shadow: 0 0 4px 0 rgba(0, 0, 0, 0.5);
	}
	/* Plex: AudioVideoPlaybackSettings-title. */
	.title {
		margin-bottom: 20px;
		color: #fff;
		font-family: var(--font-heading);
		font-size: 15px;
		font-weight: 700;
		line-height: 25.71px;
		text-align: center;
		text-transform: uppercase;
	}
	.rows {
		width: 100%;
		padding: 0 20px;
	}
	/* Plex: AudioVideoSettingsRow. */
	.row {
		display: flex;
		justify-content: center;
		font-size: 13px;
		line-height: 22.29px;
		color: #fff;
	}
	.row.disabled {
		opacity: 0.2;
		pointer-events: none;
	}
	.label {
		flex: 1 0 130px;
		min-width: 0;
		margin-right: 40px;
		color: hsla(0, 0%, 100%, 0.45);
		font-family: var(--font-heading);
		font-weight: 700;
		text-align: right;
		text-transform: uppercase;
		white-space: nowrap;
	}
	.cell {
		flex: 1 0 130px;
		min-width: 0;
		overflow: visible;
		white-space: nowrap;
	}
	.value :global(.trigger) {
		height: 22px;
		color: var(--color-brand-accent);
	}
	.value :global(.trigger .arrow) {
		border-top-color: var(--color-brand-accent);
	}
	.value :global(.trigger:hover) {
		color: #fff;
	}
	.check input {
		margin: 0;
		vertical-align: middle;
		accent-color: var(--color-brand-accent);
	}
</style>
