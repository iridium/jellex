// Plex Web's motion constants and the framer-motion transitions it uses,
// replayed with the Web Animations API. Values from Plex Web 4.160's
// bundle (module 82810 and the Menu, Modal, SourceSidebar and Scroller
// components); see AGENTS.md "Copying Plex Web".

export const speed = {
	default: 200,
	slow: 600,
	fast: 100,
	modal: 200,
	menu: 150,
	tooltip: 150
} as const;

/** Plex's motionEasing. */
export const motionEasing = 'cubic-bezier(0.6, 0.4, 0.2, 1.4)';
/** framer-motion's named easings. */
export const easeOut = 'cubic-bezier(0, 0, 0.58, 1)';
export const easeInOut = 'cubic-bezier(0.42, 0, 0.58, 1)';

interface Pose {
	opacity: number;
	y?: number;
	x?: string;
	scale?: number;
}

function transform(p: Pose) {
	return `translateX(${p.x ?? '0px'}) translateY(${p.y ?? 0}px) scale(${p.scale ?? 1})`;
}

/**
 * framer-motion's per-property transitions: opacity and transform can have
 * different easings, so they run as two animations.
 */
function tween(
	el: Element,
	from: Pose,
	to: Pose,
	duration: number,
	opacityEasing: string,
	transformEasing: string
): Promise<void> {
	const opts = { duration, fill: 'both' as const };
	const a = el.animate([{ opacity: from.opacity }, { opacity: to.opacity }], {
		...opts,
		easing: opacityEasing
	});
	const b = el.animate([{ transform: transform(from) }, { transform: transform(to) }], {
		...opts,
		easing: transformEasing
	});
	const identity = to.opacity === 1 && !to.y && !to.x && (to.scale ?? 1) === 1;
	return Promise.all([a.finished, b.finished]).then(
		() => {
			// An entrance ends on the element's own styles, so it leaves
			// nothing inline: a leftover transform would make the element the
			// containing block for fixed descendants such as submenus.
			if (!identity) {
				a.commitStyles?.();
				b.commitStyles?.();
			}
			a.cancel();
			b.cancel();
		},
		() => {}
	);
}

/** Plex's Menu: preEnter {0, y -10, 0.95} → enter; exit {0, y 10, 0.95}. */
export function menuIn(el: Element) {
	return tween(
		el,
		{ opacity: 0, y: -10, scale: 0.95 },
		{ opacity: 1 },
		speed.menu,
		easeOut,
		motionEasing
	);
}
export function menuOut(el: Element) {
	return tween(
		el,
		{ opacity: 1 },
		{ opacity: 0, y: 10, scale: 0.95 },
		speed.menu,
		easeOut,
		motionEasing
	);
}

/** Plex's Modal: {opacity 0, scale 0.95} ↔ {1, 1}; the backdrop fades. */
export function modalIn(modal: Element, backdrop: Element) {
	backdrop.animate([{ opacity: 0 }, { opacity: 1 }], { duration: speed.modal, easing: easeOut });
	return tween(
		modal,
		{ opacity: 0, scale: 0.95 },
		{ opacity: 1 },
		speed.modal,
		easeOut,
		motionEasing
	);
}
export function modalOut(modal: Element, backdrop: Element) {
	backdrop.animate([{ opacity: 1 }, { opacity: 0 }], {
		duration: speed.modal,
		easing: easeOut,
		fill: 'both'
	});
	return tween(
		modal,
		{ opacity: 1 },
		{ opacity: 0, scale: 0.95 },
		speed.modal,
		easeOut,
		motionEasing
	);
}

/** Plex's SourceSidebar panes: x ±100% with opacity, 0.2s easeInOut. */
export function paneSwap(entering: Element, leaving: Element | null, enterFrom: '100%' | '-100%') {
	const exitTo = enterFrom === '100%' ? '-100%' : '100%';
	const opts = { duration: speed.default, easing: easeInOut, fill: 'both' as const };
	const enter = entering.animate(
		[
			{ opacity: 0, transform: `translateX(${enterFrom})` },
			{ opacity: 1, transform: 'translateX(0%)' }
		],
		opts
	);
	const exit = leaving?.animate(
		[
			{ opacity: 1, transform: 'translateX(0%)' },
			{ opacity: 0, transform: `translateX(${exitTo})` }
		],
		opts
	);
	// The entering pane ends untransformed, so fixed menus inside it are
	// positioned against the viewport. The leaving pane is removed by the
	// caller once this resolves.
	return Promise.all([enter.finished, exit?.finished]).then(
		() => enter.cancel(),
		() => {}
	);
}

/**
 * Svelte action: Plex fades artwork in once it has loaded (a 600ms linear
 * opacity animation, slowSpeed). Re-runs when the src changes.
 */
export function fadeIn(img: HTMLImageElement) {
	const onLoad = () => {
		img.style.opacity = '1';
		img.animate([{ opacity: 0 }, { opacity: 1 }], { duration: speed.slow, easing: 'linear' });
	};
	const arm = () => {
		if (img.complete && img.naturalWidth) return;
		img.style.opacity = '0';
	};
	arm();
	img.addEventListener('load', onLoad);
	const mo = new MutationObserver(arm);
	mo.observe(img, { attributes: true, attributeFilter: ['src'] });
	return {
		destroy() {
			img.removeEventListener('load', onLoad);
			mo.disconnect();
		}
	};
}

/** framer-motion easeInOut, for the scroll tween. */
function cubicBezier(p1x: number, p1y: number, p2x: number, p2y: number) {
	const cx = 3 * p1x,
		bx = 3 * (p2x - p1x) - cx,
		ax = 1 - cx - bx;
	const cy = 3 * p1y,
		by = 3 * (p2y - p1y) - cy,
		ay = 1 - cy - by;
	const x = (t: number) => ((ax * t + bx) * t + cx) * t;
	const y = (t: number) => ((ay * t + by) * t + cy) * t;
	const dx = (t: number) => (3 * ax * t + 2 * bx) * t + cx;
	return (p: number) => {
		let t = p;
		for (let i = 0; i < 8; i++) {
			const e = x(t) - p;
			const d = dx(t);
			if (Math.abs(e) < 1e-6 || Math.abs(d) < 1e-6) break;
			t -= e / d;
		}
		return y(Math.min(1, Math.max(0, t)));
	};
}
const easeInOutFn = cubicBezier(0.42, 0, 0.58, 1);

/** Plex's Scroller smooth scroll: a 400ms easeInOut tween of scrollLeft/Top. */
export function smoothScroll(el: HTMLElement, to: { left?: number; top?: number }, duration = 400) {
	const from = { left: el.scrollLeft, top: el.scrollTop };
	const max = { left: el.scrollWidth - el.clientWidth, top: el.scrollHeight - el.clientHeight };
	const target = {
		left: to.left == null ? from.left : Math.max(0, Math.min(max.left, to.left)),
		top: to.top == null ? from.top : Math.max(0, Math.min(max.top, to.top))
	};
	const t0 = performance.now();
	const step = (now: number) => {
		const p = Math.min(1, (now - t0) / duration);
		const e = easeInOutFn(p);
		el.scrollLeft = from.left + (target.left - from.left) * e;
		el.scrollTop = from.top + (target.top - from.top) * e;
		if (p < 1) requestAnimationFrame(step);
	};
	requestAnimationFrame(step);
}
