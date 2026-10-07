// App-wide modals opened from menus anywhere (Plex renders these in its
// modal root). The ModalHost component in the root layout shows them.
import type { BaseItemDto } from '@jellyfin/sdk/lib/generated-client';

export type ModalRequest =
	{ kind: 'info'; item: BaseItemDto } | { kind: 'add-to-playlist'; items: BaseItemDto[] };

let current = $state<ModalRequest | null>(null);

export const modals = {
	get current() {
		return current;
	},
	open(m: ModalRequest) {
		current = m;
	},
	close() {
		current = null;
	}
};
