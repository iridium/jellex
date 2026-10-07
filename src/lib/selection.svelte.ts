// Plex Web's multi-select: the select circles on cards and rows add items
// here, and the page shows PageHeaderMultiselectActions while any are.
import type { BaseItemDto } from '@jellyfin/sdk/lib/generated-client';

let selected = $state(new Map<string, BaseItemDto>());

export const selection = {
	get count() {
		return selected.size;
	},
	get items(): BaseItemDto[] {
		return [...selected.values()];
	},
	has(id: string | null | undefined) {
		return !!id && selected.has(id);
	},
	toggle(item: BaseItemDto) {
		const next = new Map(selected);
		if (next.has(item.Id!)) next.delete(item.Id!);
		else next.set(item.Id!, item);
		selected = next;
	},
	clear() {
		if (selected.size) selected = new Map();
	}
};
