<script lang="ts">
	import { page } from '$app/state';
	import Icon from '#lib/components/Icon.svelte';

	// Plex Web's EmptyPage error state ("Not found", "Go Home").
	const notFound = $derived(page.status === 404);
</script>

<svelte:head>
	<title>{notFound ? 'Not found' : 'Something went wrong'} · jellex</title>
</svelte:head>

<div class="empty-page">
	<div class="content">
		<span class="icon"><Icon name="warning" size={40} /></span>
		<h1 class="title">{notFound ? 'Not found' : 'Something went wrong'}</h1>
		<p class="description">
			{notFound
				? 'We’re having trouble finding this page. It might have been deleted'
				: (page.error?.message ?? 'An unexpected error occurred.')}
		</p>
		<a class="button" href="/">Go Home</a>
	</div>
</div>

<style>
	/* Plex: EmptyPage + EmptyPageContent. */
	.empty-page {
		display: flex;
		flex-direction: column;
		flex: 1;
		min-height: 0;
		font-size: 13px;
	}
	.content {
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		flex: 1;
		padding: 48px 48px 98px;
		font-size: 15px;
	}
	.icon {
		display: grid;
		place-items: center;
		margin-bottom: 42px;
		padding: 30px;
		border-radius: 50%;
		background-color: rgba(0, 0, 0, 0.3);
		color: hsla(0, 0%, 100%, 0.75);
		line-height: 0;
	}
	.title {
		margin: 0;
		color: #eee;
		font-family: var(--font-heading);
		font-size: 24px;
		font-weight: 700;
		line-height: 1.5;
	}
	.description {
		max-width: 640px;
		margin: 30px 0 40px;
		color: hsla(0, 0%, 100%, 0.75);
		font-size: 15px;
		line-height: 1.5;
		text-align: center;
	}
	.button {
		min-height: 32px;
		padding: 4px 20px;
		border-radius: 4px;
		background-color: #666;
		color: #fff;
		font-size: 16px;
		font-weight: 600;
		line-height: 24px;
		transition: background-color 0.2s;
	}
	.button:hover {
		background-color: #707070;
	}
</style>
