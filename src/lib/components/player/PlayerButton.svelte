<script lang="ts">
	import type { IconName } from '#lib/icons.ts';
	import Icon from '../Icon.svelte';

	// Plex Web's PlayerIconButton: a 30px round button with an 18px icon.
	let {
		icon,
		label,
		onclick,
		disabled = false,
		active = false,
		primary = false
	}: {
		icon: IconName;
		label: string;
		onclick: () => void;
		disabled?: boolean;
		active?: boolean;
		primary?: boolean;
	} = $props();
</script>

<button
	class="player-button"
	class:active
	class:primary
	type="button"
	aria-label={label}
	title={label}
	{disabled}
	{onclick}
>
	<Icon name={icon} size={18} />
</button>

<style>
	.player-button {
		display: flex;
		align-items: center;
		justify-content: center;
		flex-shrink: 0;
		width: 30px;
		height: 30px;
		margin-left: 5px;
		border-radius: 15px;
		color: hsla(0, 0%, 100%, 0.7);
		font-size: 18px;
		transition: color 0.2s;
	}
	.player-button:first-child {
		margin-left: 0;
	}
	.player-button:hover:not(:disabled) {
		color: #fff;
	}
	.player-button:disabled {
		color: hsla(0, 0%, 100%, 0.15);
		cursor: default;
	}
	/* Plex: PlayerIconButton-isPrimary. */
	.primary {
		background-color: rgba(0, 0, 0, 0.45);
	}
	/* Plex: IconButton-isActive. */
	.active {
		color: var(--color-brand-accent);
	}
	.active:hover:not(:disabled) {
		color: var(--color-accent-light);
	}
</style>
