<script lang="ts">
	import { goto } from '$app/navigation';
	import Wordmark from '#lib/components/Wordmark.svelte';
	import {
		DEMO_SERVER,
		defaultServerUrl,
		findServer,
		session,
		type FoundServer
	} from '#lib/session.svelte.ts';

	// Two steps, like Jellyfin's own clients: find the server first, then
	// sign in to it with a password or Quick Connect.
	let serverInput = $state(defaultServerUrl());
	let server = $state<FoundServer | null>(null);
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
		return "Couldn't reach the server.";
	}

	async function connect(event: SubmitEvent) {
		event.preventDefault();
		error = '';
		busy = true;
		try {
			server = await findServer(serverInput);
			if (!server) error = "Couldn't find a Jellyfin server at that address.";
		} finally {
			busy = false;
		}
	}

	function changeServer() {
		cancelQuick?.();
		cancelQuick = null;
		server = null;
		quickCode = quickStatus = error = '';
	}

	async function submit(event: SubmitEvent) {
		event.preventDefault();
		if (server) await signIn(server.url, username, password);
	}

	async function signIn(url: string, user: string, pass: string) {
		error = '';
		busy = true;
		try {
			await session.signIn(url, user, pass);
			await goto('/', { replace: true });
		} catch (e) {
			error = describe(e);
		} finally {
			busy = false;
		}
	}

	async function quickConnect() {
		if (!server) return;
		error = '';
		quickStatus = 'Starting…';
		try {
			const { code, poll } = await session.startQuickConnect(server.url);
			quickCode = code;
			quickStatus = 'Waiting for approval…';
			let stopped = false;
			cancelQuick = () => (stopped = true);
			while (!stopped) {
				await new Promise((r) => setTimeout(r, 2000));
				if (stopped) return;
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

		{#if !server}
			<p class="sub">Enter the address of your Jellyfin server.</p>
			<form onsubmit={connect}>
				<label for="server">Server</label>
				<!-- svelte-ignore a11y_autofocus -->
				<input
					id="server"
					bind:value={serverInput}
					autocomplete="url"
					spellcheck="false"
					placeholder="jellyfin.example.com"
					autofocus
					required
				/>
				{#if error}<p class="err" role="alert">{error}</p>{/if}
				<button class="primary" type="submit" disabled={busy}>
					{busy ? 'Connecting…' : 'Connect'}
				</button>
			</form>
			<div class="divider">or</div>
			<button class="secondary demo" type="button" onclick={signInToDemo} disabled={busy}
				>Try the Jellyfin demo</button
			>
		{:else}
			<div class="server">
				<div class="server-text">
					<span class="server-name">{server.name}</span>
					<span class="server-url">{server.url}</span>
				</div>
				<button class="change" type="button" onclick={changeServer}>Change</button>
			</div>

			<form onsubmit={submit}>
				<label for="u">Username</label>
				<!-- svelte-ignore a11y_autofocus -->
				<input id="u" bind:value={username} autocomplete="username" autofocus required />
				<label for="p">Password</label>
				<input id="p" type="password" bind:value={password} autocomplete="current-password" />
				{#if error}<p class="err" role="alert">{error}</p>{/if}
				<button class="primary" type="submit" disabled={busy}>Sign in</button>
			</form>

			{#if server.quickConnect}
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
			{/if}
		{/if}
	</div>
	<a
		class="source"
		href="https://github.com/iridium/jellex"
		target="_blank"
		rel="noopener noreferrer">jellex on GitHub</a
	>
</main>

<style>
	.login {
		min-height: 100%;
		display: grid;
		place-items: center;
		align-content: center;
		gap: 16px;
		padding: 16px;
		overflow: auto;
	}
	.source {
		color: var(--color-text-muted);
		font-size: 13px;
	}
	.source:hover {
		color: var(--color-text-primary);
		text-decoration: underline;
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
	.server {
		display: flex;
		align-items: center;
		gap: 12px;
		margin: 16px 0 8px;
		padding: 10px 12px;
		border-radius: var(--border-radius-s);
		background: var(--color-background-control);
	}
	.server-text {
		display: flex;
		flex: 1;
		flex-direction: column;
		min-width: 0;
	}
	.server-name {
		color: var(--color-text-primary);
		font-weight: 600;
	}
	.server-url {
		overflow: hidden;
		color: var(--color-text-muted);
		font-size: 13px;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.change {
		width: auto;
		margin: 0;
		padding: 4px 0;
		color: var(--color-accent-dark);
		font-weight: 400;
	}
	.change:hover {
		text-decoration: underline;
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
