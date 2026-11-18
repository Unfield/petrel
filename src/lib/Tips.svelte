<script lang="ts">
	// One tooltip for the whole app. Any button or link with a title shows it
	// here on hover or keyboard focus, styled like Tern's Tooltip and without
	// the browser's long wait. The title moves aside while the tip is open, so
	// the browser's own tooltip doesn't show on top of it.
	import { onMount } from 'svelte';

	const SELECTOR = 'button[title], a[title], [role="button"][title], summary[title]';
	let tip = $state<HTMLDivElement>();
	let text = $state('');
	let open = $state(false);
	let anchor: HTMLElement | null = null;
	let timer: ReturnType<typeof setTimeout> | undefined;
	let warmUntil = 0; // moving between buttons shows the next tip at once

	function take(el: HTMLElement) {
		const title = el.getAttribute('title');
		if (!title) return '';
		el.dataset.tip = title;
		el.removeAttribute('title');
		// An icon-only button is named by its title; keep that name while it's moved.
		if (!el.hasAttribute('aria-label') && !el.textContent?.trim()) {
			el.setAttribute('aria-label', title);
			el.dataset.tipLabel = '';
		}
		return title;
	}

	function give(el: HTMLElement) {
		if (el.dataset.tip !== undefined) {
			if (!el.hasAttribute('title')) el.setAttribute('title', el.dataset.tip);
			delete el.dataset.tip;
		}
		if (el.dataset.tipLabel !== undefined) {
			el.removeAttribute('aria-label');
			delete el.dataset.tipLabel;
		}
	}

	function place() {
		if (!tip || !anchor) return;
		const a = anchor.getBoundingClientRect();
		const t = tip.getBoundingClientRect();
		const gap = 6;
		let top = a.top - t.height - gap;
		if (top < 4) top = a.bottom + gap;
		let left = a.left + a.width / 2 - t.width / 2;
		left = Math.max(4, Math.min(left, innerWidth - t.width - 4));
		tip.style.top = `${Math.round(top)}px`;
		tip.style.left = `${Math.round(left)}px`;
	}

	function show(el: HTMLElement, now: boolean) {
		clearTimeout(timer);
		if (anchor && anchor !== el) hide();
		const go = () => {
			if (!el.isConnected) return;
			const title = take(el);
			if (!title) return;
			anchor = el;
			text = title;
			open = true;
		};
		if (now || Date.now() < warmUntil) go();
		else timer = setTimeout(go, 500);
	}

	function hide() {
		clearTimeout(timer);
		if (open) warmUntil = Date.now() + 400;
		open = false;
		if (anchor) give(anchor);
		anchor = null;
	}

	$effect(() => {
		if (!tip) return;
		if (open) {
			tip.showPopover?.();
			place();
		} else tip.hidePopover?.();
	});

	onMount(() => {
		const over = (e: PointerEvent) => {
			if (e.pointerType === 'touch') return;
			const el = (e.target as Element | null)?.closest<HTMLElement>(SELECTOR + ', [data-tip]');
			if (!el) {
				if (anchor) hide();
				return;
			}
			if (el === anchor) return;
			show(el, false);
		};
		const focus = (e: FocusEvent) => {
			const el = e.target as HTMLElement;
			if (el?.matches?.(':focus-visible') && el.matches(SELECTOR)) show(el, true);
		};
		const blur = (e: FocusEvent) => e.target === anchor && hide();
		const leave = () => hide();
		const key = (e: KeyboardEvent) => e.key === 'Escape' && open && hide();
		document.addEventListener('pointerover', over);
		document.addEventListener('pointerdown', leave, true);
		document.addEventListener('focusin', focus);
		document.addEventListener('focusout', blur);
		document.addEventListener('keydown', key, true);
		document.addEventListener('scroll', leave, true);
		document.documentElement.addEventListener('pointerleave', leave);
		addEventListener('blur', leave);
		return () => {
			hide();
			document.removeEventListener('pointerover', over);
			document.removeEventListener('pointerdown', leave, true);
			document.removeEventListener('focusin', focus);
			document.removeEventListener('focusout', blur);
			document.removeEventListener('keydown', key, true);
			document.removeEventListener('scroll', leave, true);
			document.documentElement.removeEventListener('pointerleave', leave);
			removeEventListener('blur', leave);
		};
	});
</script>

<div bind:this={tip} class="tip" class:open role="tooltip" popover="manual">{text}</div>

<style>
	.tip {
		position: fixed;
		inset: auto;
		margin: 0;
		width: max-content;
		max-width: 16rem;
		padding: 0.35rem 0.55rem;
		border: 0;
		border-radius: var(--tern-radius-sm);
		background: var(--tern-fg);
		color: var(--tern-bg);
		font: 500 0.75rem/1.35 var(--tern-font);
		pointer-events: none;
		z-index: 1000;
	}
	.tip.open {
		animation: tip-in var(--tern-duration, 120ms) ease-out;
	}
	@keyframes tip-in {
		from {
			opacity: 0;
		}
	}
	@media (prefers-reduced-motion: reduce) {
		.tip.open {
			animation: none;
		}
	}
</style>
