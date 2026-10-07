<script lang="ts">
	import { goto } from '$app/navigation';
	import Wordmark from '#lib/components/Wordmark.svelte';
	import { DEMO_SERVER, defaultServerUrl, session } from '#lib/session.svelte.ts';

	let serverUrl = $state(defaultServerUrl());
	let username = $state('');
	let password = $state('');
	let error = $state('');
	let busy = $state(false);
	let quickCode = $state('');
	let quickStatus = $state('');
	let cancelQuick: (() => void) | null = null;

	if (session.signedIn) goto('/', { replace: true });

	function describe(e: unknown): string {
		const status = (e as { response?: { status?: number } })?.response?.status;
		if (status === 401) return 'Wrong username or password.';
		if (status) return `The server answered with HTTP ${status}.`;
		return "Couldn't reach the server. Check the address.";
	}

	async function submit(event: SubmitEvent) {
		event.preventDefault();
		await signIn(serverUrl, username, password);
	}

	async function signIn(server: string, user: string, pass: string) {
		error = '';
		busy = true;
		try {
			await session.signIn(server, user, pass);
			await goto('/', { replace: true });
		} catch (e) {
			error = describe(e);
		} finally {
			busy = false;
		}
	}

	async function quickConnect() {
		error = '';
		quickStatus = 'Starting…';
		try {
			const { code, poll } = await session.startQuickConnect(serverUrl);
			quickCode = code;
			quickStatus = 'Waiting for approval…';
			let stopped = false;
			cancelQuick = () => (stopped = true);
			while (!stopped) {
				await new Promise((r) => setTimeout(r, 2000));
				if (await poll()) {
					await goto('/', { replace: true });
					return;
				}
			}
		} catch (e) {
			quickCode = '';
			quickStatus = '';
			error = e instanceof Error && !('response' in e) ? e.message : describe(e);
		}
	}

	$effect(() => () => cancelQuick?.());

	// Jellyfin's public demo server: user "demo", no password.
	function signInToDemo() {
		return signIn(DEMO_SERVER, 'demo', '');
	}
</script>

<main class="login">
	<div class="panel">
		<div class="brand">
			<Wordmark height={28} />
		</div>
		<p class="sub">Sign in with your Jellyfin account.</p>

		<form onsubmit={submit}>
			<label for="server">Server</label>
			<input id="server" bind:value={serverUrl} autocomplete="url" spellcheck="false" required />
			<label for="u">Username</label>
			<input id="u" bind:value={username} autocomplete="username" required />
			<label for="p">Password</label>
			<input id="p" type="password" bind:value={password} autocomplete="current-password" />
			{#if error}<p class="err" role="alert">{error}</p>{/if}
			<button class="primary" type="submit" disabled={busy}>Sign in</button>
		</form>

		<div class="divider">or</div>
		{#if quickCode}
			<div class="quick" aria-live="polite">
				<p>Enter this code in a Jellyfin app you're signed in to (Settings → Quick Connect):</p>
				<div class="code">{quickCode}</div>
				<p>{quickStatus}</p>
			</div>
		{:else}
			<button class="secondary" type="button" onclick={quickConnect} disabled={!!quickStatus}>
				Use Quick Connect
			</button>
		{/if}
		<button class="secondary demo" type="button" onclick={signInToDemo} disabled={busy}
			>Try the Jellyfin demo</button
		>
	</div>
</main>

<style>
	.login {
		min-height: 100%;
		display: grid;
		place-items: center;
		padding: 16px;
		overflow: auto;
	}
	.panel {
		width: 100%;
		max-width: 360px;
		background: var(--color-background-modal);
		border-radius: var(--border-radius-m);
		box-shadow: var(--shadow-high);
		padding: 32px 28px;
	}
	.brand {
		display: flex;
		align-items: center;
		gap: 10px;
		margin-bottom: 6px;
	}
	.sub {
		color: var(--color-text-muted);
		margin: 0 0 22px;
	}
	label {
		display: block;
		font-size: 13px;
		color: var(--color-text-muted);
		margin: 14px 0 6px;
	}
	input {
		width: 100%;
		padding: 8px 12px;
		border-radius: var(--border-radius-s);
		border: 0;
		background: var(--color-background-control);
		color: var(--color-text-primary);
		font: inherit;
	}
	input:focus {
		outline: 2px solid var(--color-background-accent);
		outline-offset: -1px;
		background: var(--color-background-control-focus);
	}
	button {
		width: 100%;
		margin-top: 20px;
		padding: 10px;
		border: 0;
		border-radius: var(--border-radius-s);
		font-weight: 600;
	}
	button:disabled {
		opacity: 0.5;
		cursor: default;
	}
	.primary {
		background: var(--color-background-accent);
		color: var(--color-text-on-accent);
	}
	.primary:hover:not(:disabled) {
		background: var(--color-background-accent-focus);
	}
	.secondary {
		background: var(--color-background-control);
		color: var(--color-text-primary);
		margin-top: 0;
	}
	.secondary.demo {
		margin-top: 8px;
	}
	.secondary:hover:not(:disabled) {
		background: var(--color-background-control-focus);
	}
	.err {
		color: var(--color-text-alert);
		margin: 14px 0 0;
	}
	.divider {
		display: flex;
		align-items: center;
		gap: 10px;
		color: var(--color-text-muted);
		font-size: 12px;
		margin: 22px 0 16px;
	}
	.divider::before,
	.divider::after {
		content: '';
		flex: 1;
		border-top: 1px solid var(--color-background-control);
	}
	.quick {
		text-align: center;
	}
	.quick p {
		color: var(--color-text-muted);
		font-size: 13px;
		margin: 0;
	}
	.code {
		font:
			600 30px/1 ui-monospace,
			monospace;
		letter-spacing: 6px;
		margin: 12px 0;
		color: var(--color-text-primary);
	}
</style>
