<script lang="ts">
	// Plex Web's Spinner: a 2px ring with two coloured quarters, one turn
	// every 0.6s. Sizes are Plex's tiny/small/medium/large.
	let { size = 'medium' }: { size?: 'tiny' | 'small' | 'medium' | 'large' } = $props();
</script>

<span class="spinner {size}" role="status" aria-label="Loading"></span>

<style>
	.spinner {
		display: inline-block;
		box-sizing: border-box;
		border: 2px solid transparent;
		/* Plex's #cc7b19, recoloured like the rest of its accents. */
		border-color: var(--color-accent-dark) transparent transparent var(--color-accent-dark);
		border-radius: 50%;
		animation: spin 0.6s linear infinite;
	}
	.tiny {
		width: 12px;
		height: 12px;
	}
	.small {
		width: 16px;
		height: 16px;
	}
	.medium {
		width: 24px;
		height: 24px;
	}
	.large {
		width: 30px;
		height: 30px;
	}
</style>
