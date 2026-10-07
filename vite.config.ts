import tailwindcss from '@tailwindcss/vite';
import adapter from '@sveltejs/adapter-cloudflare';
import { sveltekit } from '@sveltejs/kit/vite';
import { execSync } from 'node:child_process';
import { defineConfig } from 'vite';

// The commit being built, shown in Settings > General. Cloudflare's Git
// builds set WORKERS_CI_COMMIT_SHA; elsewhere ask git.
function commit(): string {
	if (process.env.WORKERS_CI_COMMIT_SHA) return process.env.WORKERS_CI_COMMIT_SHA;
	try {
		return execSync('git rev-parse HEAD', { encoding: 'utf8' }).trim();
	} catch {
		return '';
	}
}

export default defineConfig({
	define: { __APP_COMMIT__: JSON.stringify(commit()) },
	// Plex's port, so bookmarks and muscle memory carry over; HMR needs the same
	// port inside and outside the container.
	server: { port: 32400, strictPort: true },
	plugins: [
		tailwindcss(),
		sveltekit({
			compilerOptions: {
				// Force runes mode for the project, except for libraries. Can be removed in svelte 6.
				runes: ({ filename }) =>
					filename.split(/[/\\]/).includes('node_modules') ? undefined : true
			},
			adapter: adapter()
		})
	]
});
