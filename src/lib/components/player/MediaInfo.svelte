<script lang="ts">
	import type { BaseItemDto } from '@jellyfin/sdk/lib/generated-client';
	import { getLibraryApi } from '@jellyfin/sdk/lib/utils/api/library-api';
	import { mediaInfo, type MediaProps } from '#lib/mediainfo.ts';
	import { player } from '#lib/player.svelte.ts';
	import { session } from '#lib/session.svelte.ts';
	import Icon from '../Icon.svelte';
	import Spinner from '../Spinner.svelte';

	// Plex Web's Media Info modal (mediaInfoModal: Bootstrap 3 markup and
	// CSS, "media-info-modal modal modal-lg fade"), for the playing item or
	// any item a menu passes in.
	let { onclose, item }: { onclose: () => void; item?: BaseItemDto } = $props();

	let full = $state<BaseItemDto | null>(null);
	const subject = $derived(item ? full : player.item);
	$effect(() => {
		const id = item?.Id;
		if (!id) return;
		getLibraryApi(session.requireApi)
			.getItem({ itemId: id, userId: session.userId })
			.then((r) => (full = r.data))
			.catch(() => {});
	});
	const media = $derived.by((): MediaProps[] => {
		const it = subject;
		if (!it) return [];
		const trick = it.Trickplay ?? {};
		return (it.MediaSources ?? []).map((s) => mediaInfo(it, s, !!trick[s.Id ?? '']));
	});
	const loading = $derived(!subject);
	const hasStreams = $derived(loading || media.some((m) => m.Part.some((p) => p.Stream.length)));
	const heading = { 1: 'Video', 2: 'Audio', 3: 'Subtitles' } as Record<number, string>;

	// Bootstrap 3 modal: .fade → .in on the next frame; hiding removes .in
	// and waits for the 0.3s dialog transition before removing.
	let shown = $state(false);
	$effect(() => {
		const raf = requestAnimationFrame(() => (shown = true));
		return () => cancelAnimationFrame(raf);
	});
	let closing = false;
	function hide() {
		if (closing) return;
		closing = true;
		shown = false;
		setTimeout(onclose, 300);
	}
</script>

<svelte:window onkeydown={(e) => e.key === 'Escape' && hide()} />

<div class="modal-backdrop fade" class:in={shown}></div>
<div
	class="media-info-modal modal modal-lg fade"
	class:in={shown}
	role="dialog"
	aria-modal="true"
	aria-label="Media Info"
	tabindex="-1"
	onclick={(e) => e.target === e.currentTarget && hide()}
	onkeydown={() => {}}
>
	<div class="modal-dialog">
		<div class="modal-content">
			<div class="modal-header">
				<button type="button" class="close" aria-label="Close" onclick={hide}>
					<Icon name="glyph-remove-2" size={16} />
				</button>
				<h4 class="modal-title">
					<span class="modal-icon"><Icon name="glyph-circle-info" size={18} /></span>
					Media Info
				</h4>
			</div>
			<div class="modal-body modal-body-scroll dark-scrollbar">
				<div class="position-ref" class:hide-streams-column={!hasStreams}>
					{#each media as m, i (i)}
						<div class="files">
							<h4>Files</h4>
							<ul class="media-info-file-list well">
								{#each m.Part as part, j (j)}
									<li>{part.file}</li>
								{/each}
							</ul>
						</div>
						<div class="media">
							<div class="props">
								<h4>Media</h4>
								<ul>
									{#each m.properties as p (p.name)}
										<li>
											<span class="detail-label">{p.name}</span>
											<span class="detail">{p.value}</span>
										</li>
									{/each}
								</ul>
							</div>
							{#each m.Part as part, j (j)}
								<div class="part">
									<div class="props">
										<h4>Part</h4>
										<ul>
											{#each part.properties as p (p.name)}
												<li>
													<span class="detail-label">{p.name}</span>
													<span class="detail">{p.value}</span>
												</li>
											{/each}
										</ul>
									</div>
									<div class="streams">
										{#each part.Stream as st, k (k)}
											<div class="props">
												{#if heading[st.streamType]}<h4>{heading[st.streamType]}</h4>{/if}
												<ul>
													{#each st.properties as p (p.name)}
														<li>
															<span class="detail-label">{p.name}</span>
															<span class="detail">{p.value}</span>
														</li>
													{/each}
												</ul>
											</div>
										{/each}
									</div>
								</div>
							{/each}
						</div>
					{/each}
					{#if loading}
						<div class="loading"><Spinner /></div>
					{/if}
				</div>
			</div>
		</div>
	</div>
</div>

<style>
	.loading {
		display: flex;
		justify-content: center;
		padding: 40px 0;
	}
	/* Bootstrap 3 (Plex Web's legacy modal styles). */
	.modal-backdrop {
		position: fixed;
		inset: 0;
		z-index: 1080;
		background-color: #000;
	}
	.fade {
		opacity: 0;
		transition: opacity 0.15s linear;
	}
	.modal-backdrop.fade.in {
		opacity: 0.5;
	}
	.modal.fade.in {
		opacity: 1;
	}
	.modal {
		position: fixed;
		inset: 0;
		z-index: 1080;
		overflow-y: auto;
		color: #eee;
		font-size: 14px;
		line-height: 24px;
	}
	.modal-dialog {
		position: relative;
		width: 750px;
		max-width: 100%;
		margin: 0 auto;
		padding: 70px 10px;
		transform: translateY(-25%);
		transition: transform 0.3s ease-out;
	}
	.modal.in .modal-dialog {
		transform: none;
	}
	.modal-content {
		position: relative;
		border-radius: 3px;
		outline: none;
		background-clip: padding-box;
		background-color: #282828;
		box-shadow: 0 5px 15px rgba(0, 0, 0, 0.5);
	}
	.modal-header {
		min-height: 16.71px;
		padding: 15px 20px;
		border-bottom: 1px solid #222;
		border-radius: 3px 3px 0 0;
		background-color: #323232;
	}
	.close {
		display: flex;
		align-items: flex-start;
		float: right;
		height: 19px;
		margin-top: 6px;
		color: #eee;
		font-size: 16px;
		line-height: 1;
		opacity: 0.2;
	}
	.close:hover,
	.close:focus {
		opacity: 0.5;
	}
	/* Glyphicons: inline-block, line-height 1, top 1px, middle-aligned. */
	.close :global(svg),
	.modal-icon :global(svg) {
		display: block;
		overflow: visible;
	}
	.close :global(svg) {
		position: relative;
		top: 1px;
	}
	.modal-title {
		max-width: 650px;
		margin: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		font-size: 20px;
		font-weight: 400;
		line-height: 1.71428571;
		vertical-align: bottom;
	}
	.modal-icon {
		display: inline-block;
		position: relative;
		top: -2.5px;
		height: 18px;
		margin-right: 5px;
		color: #999;
		font-size: 18px;
		vertical-align: middle;
		line-height: 1;
	}
	.modal-body {
		position: relative;
		padding: 5px 20px;
	}
	.modal-body-scroll {
		max-height: 400px;
		overflow: hidden auto;
		-webkit-overflow-scrolling: touch;
	}
	h4 {
		margin: 12px 0;
		font-size: 16px;
		font-weight: 400;
		line-height: 24px;
	}
	.well {
		min-height: 20px;
		margin: 0 0 10px;
		padding: 10px 10px 10px 0;
		border: 1px solid #202020;
		border-radius: 3px;
		background-color: #323232;
		box-shadow: inset 0 1px 1px rgba(0, 0, 0, 0.05);
		list-style: none;
	}
	.media-info-file-list li {
		padding-left: 10px;
		color: #999;
	}

	/* .media-info-modal */
	.position-ref {
		position: relative;
	}
	.media {
		overflow: auto;
		margin-bottom: 40px;
	}
	.media:last-child,
	.part:last-child {
		margin-bottom: 0;
	}
	.media .props {
		position: sticky;
		top: 0;
		float: left;
		width: 200px;
		padding-right: 20px;
	}
	.media .props ul {
		margin: 0 0 12px;
		padding-left: 0;
		color: #555;
		font-size: 14px;
	}
	.media .props ul li {
		list-style: none;
		overflow: hidden;
		text-overflow: ellipsis;
	}
	.media .props .detail-label {
		color: #999;
	}
	.media .props .detail {
		margin-left: 5px;
		color: #eee;
	}
	.media .part {
		margin-left: 200px;
		overflow: hidden auto;
	}
	.media .streams {
		margin-left: 200px;
		overflow: auto;
	}
	.media .streams .props {
		position: static;
		float: none;
		width: auto;
	}
	.hide-streams-column .part {
		margin-left: 345px;
	}
	.hide-streams-column .props {
		width: 345px;
	}
	.hide-streams-column .streams {
		display: none;
	}
</style>
