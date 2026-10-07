<script lang="ts">
	import type { Snippet } from 'svelte';
	import { modalIn, modalOut } from '#lib/motion.ts';
	import Icon from './Icon.svelte';

	// Plex Web's Modal + ModalContent (Modal-small: 480px): a grey backdrop,
	// an #2d2d2d panel with an 18px header and a 60px close button, and a
	// darker body. Opens with Plex's modal motion (fade + scale 0.95 → 1)
	// and plays it back out on close.
	let {
		title,
		onclose,
		children,
		width = 480,
		bare = false
	}: {
		title: string;
		onclose: () => void;
		children: Snippet;
		width?: number;
		/** Content that brings its own sections (no tinted ModalBody). */
		bare?: boolean;
	} = $props();

	let backdrop: HTMLDivElement | undefined = $state();
	let modal: HTMLDivElement | undefined = $state();
	let closing = false;

	$effect(() => {
		if (modal && backdrop) modalIn(modal, backdrop);
	});

	/** Plays the exit motion, then calls onclose. */
	export async function close() {
		if (closing) return;
		closing = true;
		if (modal && backdrop) await modalOut(modal, backdrop);
		onclose();
	}
</script>

<svelte:window onkeydown={(e) => e.key === 'Escape' && close()} />

<div class="modal-root">
	<div
		class="backdrop"
		bind:this={backdrop}
		role="presentation"
		onclick={(e) => e.target === e.currentTarget && close()}
	>
		<div
			class="modal"
			bind:this={modal}
			role="dialog"
			aria-modal="true"
			aria-label={title}
			style:width="{width}px"
		>
			<div class="content">
				<div class="header">{title}</div>
				<button class="close" type="button" aria-label="Close" onclick={close}>
					<Icon name="close" size={18} />
				</button>
				{#if bare}{@render children()}{:else}<div class="body">{@render children()}</div>{/if}
			</div>
		</div>
	</div>
</div>

<style>
	/* Plex: Modal-modalContainer (modalZIndex 1014). */
	.modal-root {
		position: fixed;
		inset: 0;
		z-index: 1014;
		font-family: var(--font-system);
		font-size: 13px;
	}
	/* Plex: Modal-modalBackdrop. */
	.backdrop {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 100%;
		height: 100%;
		overflow-y: scroll;
		background-color: hsla(0, 0%, 40%, 0.6);
	}
	/* Plex: Modal-modal Modal-small. */
	.modal {
		display: flex;
		max-width: 90%;
		max-height: 90%;
		margin: auto;
		transform-origin: center center;
	}
	/* Plex: ModalContent. */
	.content {
		position: relative;
		display: flex;
		flex-direction: column;
		flex-grow: 1;
		width: 100%;
		background-color: #2d2d2d;
	}
	/* Plex: ModalHeader. */
	.header {
		flex-shrink: 0;
		max-width: 100%;
		min-width: 0;
		padding: 15px 60px 15px 30px;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		color: #fff;
		font-size: 18px;
		line-height: 30.86px;
	}
	/* Plex: ModalContent-closeButton. */
	.close {
		position: absolute;
		top: 0;
		right: 0;
		z-index: 1;
		display: grid;
		place-items: center;
		width: 60px;
		height: 60px;
		color: hsla(0, 0%, 100%, 0.7);
		font-size: 18px;
		transition: color 0.2s;
	}
	.close:hover {
		color: #fff;
	}
	/* Plex: ModalBody (modalScroller: no padding; content adds its own). */
	.body {
		flex-grow: 1;
		overflow: auto;
		background-color: rgba(0, 0, 0, 0.15);
	}
</style>
