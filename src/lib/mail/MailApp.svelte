<script lang="ts">
	import { t } from '@ternkit/tern';
	import { onMount } from 'svelte';
	import Compose from './Compose.svelte';
	import Help from './Help.svelte';
	import Icon from './Icon.svelte';
	import MessageList from './MessageList.svelte';
	import Palette from './Palette.svelte';
	import Reader from './Reader.svelte';
	import Settings from './Settings.svelte';
	import Sidebar from './Sidebar.svelte';
	import { MailStore } from './store.svelte';
	import type { Account, Prefs } from './types';
	import { accentFg, hue, initials, isMac } from './util';
	import './mail.css';

	let { user, accounts, prefs }: { user: { name: string; email: string }; accounts: Omit<Account, 'mailboxes'>[]; prefs: Prefs } = $props();

	// svelte-ignore state_referenced_locally
	const store = new MailStore(prefs, accounts.length);
	// svelte-ignore state_referenced_locally
	store.accounts = accounts.map((a) => ({ ...a, mailboxes: [] }));

	let mobile = $state(false);
	let width = $state(1440);
	let systemDark = $state(false);
	let resizing = $state<'' | 'cols' | 'rows'>('');
	let main = $state<HTMLElement>();
	let searchInput = $state<HTMLInputElement>();
	let userMenu = $state(false);
	// Narrow windows collapse the sidebar, and below 900px show one pane at a time.
	const layout = $derived(mobile || width < 900 ? 'off' : store.prefs.layout);
	const rail = $derived(store.prefs.rail || (!mobile && width < 1100));
	const dark = $derived(store.prefs.mode === 'dark' || (store.prefs.mode === 'system' && systemDark));
	const mod = isMac() ? '⌘' : 'Ctrl';
	const hasAccounts = $derived(store.accounts.length > 0);

	onMount(() => {
		const mq = matchMedia('(max-width: 720px)');
		const dq = matchMedia('(prefers-color-scheme: dark)');
		mobile = mq.matches;
		systemDark = dq.matches;
		const onMq = () => (mobile = mq.matches);
		const onDq = () => (systemDark = dq.matches);
		mq.addEventListener('change', onMq);
		dq.addEventListener('change', onDq);

		let events: EventSource | undefined;
		if (accounts.length) {
			store.loadAccounts();
			store.loadList();
			// New mail and changes from other devices arrive here.
			events = new EventSource('/api/mail/events');
			events.addEventListener('change', async () => {
				const before = store.unread('inbox', 'all');
				clearTimeout(liveTimer);
				liveTimer = setTimeout(async () => {
					await store.loadAccounts();
					await store.loadList({ silent: true });
					if (store.unread('inbox', 'all') > before) store.notify(t('m.t.newMail'));
				}, 400);
			});
		}
		return () => {
			mq.removeEventListener('change', onMq);
			dq.removeEventListener('change', onDq);
			events?.close();
		};
	});
	let liveTimer: ReturnType<typeof setTimeout>;

	// Search as you type, a moment after the last key.
	let searchTimer: ReturnType<typeof setTimeout>;
	function searched() {
		clearTimeout(searchTimer);
		searchTimer = setTimeout(() => {
			store.checked.clear();
			store.loadList();
		}, 300);
	}

	function startResize(e: PointerEvent) {
		if (layout === 'off' || !main) return;
		const div = e.currentTarget as HTMLElement;
		div.setPointerCapture(e.pointerId);
		resizing = layout === 'below' ? 'rows' : 'cols';
		const mainRect = main.getBoundingClientRect();
		const listRect = (main.firstElementChild as HTMLElement).getBoundingClientRect();
		const move = (ev: PointerEvent) => {
			if (layout === 'right') {
				const w = store.prefs.rtl ? listRect.right - ev.clientX : ev.clientX - listRect.left;
				store.prefs = { ...store.prefs, listW: Math.round(Math.max(300, Math.min(w, mainRect.width - 380))) };
			} else {
				const h = ((ev.clientY - mainRect.top) / mainRect.height) * 100;
				store.prefs = { ...store.prefs, listH: Math.round(Math.max(22, Math.min(h, 75))) };
			}
		};
		const up = () => {
			div.removeEventListener('pointermove', move);
			div.removeEventListener('pointerup', up);
			resizing = '';
			store.savePrefs();
		};
		div.addEventListener('pointermove', move);
		div.addEventListener('pointerup', up);
	}

	function scrollToCursor(id?: string) {
		if (!id) return;
		requestAnimationFrame(() => document.querySelector(`.row[data-id="${CSS.escape(id)}"]`)?.scrollIntoView({ block: 'nearest' }));
	}

	function keydown(e: KeyboardEvent) {
		const mod = e.metaKey || e.ctrlKey;
		if (mod && e.key.toLowerCase() === 'k') {
			e.preventDefault();
			store.ui.palette = !store.ui.palette;
			return;
		}
		if (e.key === 'Escape') {
			if (store.ui.help) return void (store.ui.help = false);
			if (store.ui.palette) return void (store.ui.palette = false);
			if (store.ui.settings) return void (store.ui.settings = false);
			if (userMenu) return void (userMenu = false);
			if (store.ui.drawer) return void (store.ui.drawer = false);
			const max = store.composes.find((c) => c.max);
			if (max) return store.updateCompose(max.key, { max: false });
			if ((e.target as HTMLElement).closest?.('.compose')) return;
			if (store.checked.size) return store.checked.clear();
			if (store.openId && layout === 'off') return store.close();
			return;
		}
		if (store.ui.palette || store.ui.help) return;
		const el = e.target as HTMLElement;
		if (el.closest?.('input, textarea, select, [contenteditable="true"]') || mod || e.altKey) return;
		const cur = store.openId ?? store.cursorId;
		const target = store.replyTarget();
		const keys: Record<string, () => unknown> = {
			j: () => scrollToCursor(store.step(1)),
			k: () => scrollToCursor(store.step(-1)),
			ArrowDown: () => scrollToCursor(store.step(1)),
			ArrowUp: () => scrollToCursor(store.step(-1)),
			Enter: () => store.cursorId && !store.openId && store.open(store.cursorId),
			o: () => store.cursorId && store.open(store.cursorId),
			u: () => layout === 'off' && store.close(),
			e: () => cur && !store.inTrash && store.archive([cur]),
			'#': () => cur && store.trash([cur]),
			Delete: () => cur && store.trash([cur]),
			s: () => cur && store.toggleFlag(cur),
			x: () => cur && (store.checked.has(cur) ? store.checked.delete(cur) : store.checked.add(cur)),
			r: () => target && store.reply(target, 'reply'),
			a: () => target && store.reply(target, 'replyAll'),
			f: () => target && store.reply(target, 'forward'),
			c: () => hasAccounts && store.compose(),
			'/': () => searchInput?.focus(),
			'?': () => (store.ui.help = true),
			I: () => cur && store.setSeen(store.idsOf([cur]).map((m) => m.id), true),
			U: () => cur && store.setSeen(store.idsOf([cur]).map((m) => m.id), false)
		};
		const fn = keys[e.key];
		if (fn) {
			e.preventDefault();
			fn();
		}
	}
</script>

<svelte:window onkeydown={keydown} bind:innerWidth={width} />

<div
	class="pm app"
	class:rail
	class:drawer-open={store.ui.drawer}
	class:resizing={!!resizing}
	class:rows={resizing === 'rows'}
	data-mode={store.prefs.mode}
	data-preset={store.prefs.preset}
	data-density={store.prefs.density}
	dir={store.prefs.rtl ? 'rtl' : 'ltr'}
	style:--accent={store.prefs.accent}
	style:--accent-fg={accentFg(store.prefs.accent)}
	style:--list-w="{store.prefs.listW}px"
	style:--list-h="{store.prefs.listH}%"
>
	<header class="topbar">
		<button class="icon-btn" onclick={() => (mobile ? (store.ui.drawer = !store.ui.drawer) : store.setPref('rail', !store.prefs.rail))} title={t('m.a.menu')} aria-label={t('m.a.menu')}><Icon name="menu" /></button>
		<a class="brand" href="/mail" onclick={(e) => (e.preventDefault(), store.go('inbox', 'all'))}>
			<svg class="logo" width="32" height="32" viewBox="0 0 64 64" aria-hidden="true"
				><rect width="64" height="64" rx="17" fill="var(--accent)" /><g transform="translate(9 4.6) scale(.72)"
					><path d="M6 31c8.5-7.5 17.5-7 26 3.5C40.5 24 49.5 23.5 58 31c-8.6-2.4-17 .8-26 10.8C23 31.8 14.6 28.6 6 31z" fill="var(--accent-fg)" /><path
						d="M14 50c3-2.6 6-2.6 9 0s6 2.6 9 0 6-2.6 9 0 6 2.6 9 0"
						fill="none"
						stroke="var(--accent-fg)"
						stroke-width="4"
						stroke-linecap="round"
						opacity=".6"
					/></g
				></svg
			>
			<span class="wordmark">Petrel</span>
		</a>
		<label class="search">
			<Icon name="search" />
			<input bind:this={searchInput} type="search" bind:value={store.search} oninput={searched} onkeydown={(e) => e.key === 'Escape' && ((store.search = ''), store.loadList(), searchInput?.blur())} placeholder={t('m.search')} autocomplete="off" spellcheck="false" disabled={!hasAccounts} />
			{#if store.search}<button class="icon-btn sm" onclick={() => ((store.search = ''), store.loadList())} aria-label={t('m.a.clear')}><Icon name="x" /></button>{/if}
			<kbd class="kbd">/</kbd>
		</label>
		<div class="top-actions">
			<button class="cmdk hide-m" onclick={() => (store.ui.palette = true)} title={t('m.a.palette')}><Icon name="command" /><span>{mod === '⌘' ? '⌘K' : 'Ctrl K'}</span></button>
			<button class="icon-btn hide-m" onclick={() => (store.ui.help = true)} title={t('m.a.help')} aria-label={t('m.a.help')}><Icon name="keyboard" /></button>
			<button class="icon-btn" onclick={() => store.setPref('mode', dark ? 'light' : 'dark')} title={t('m.a.theme')} aria-label={t('m.a.theme')}><Icon name={dark ? 'sun' : 'moon'} /></button>
			<button class="icon-btn" onclick={() => (store.ui.settings = true)} title={t('m.a.settings')} aria-label={t('m.a.settings')}><Icon name="sliders" /></button>
			<div class="pop-wrap">
				<button class="me-btn" onclick={() => (userMenu = !userMenu)} title={user.name || user.email} aria-expanded={userMenu}>
					<span class="av" style:--h={hue(user.email)} style:--avatar="34px">{initials(user.name || user.email)}</span>
				</button>
				{#if userMenu}
					<div class="pop end" role="menu">
						<div class="pop-title">{user.name || user.email}<span>{user.email}</span></div>
						<a role="menuitem" href="/settings"><Icon name="user" size={16} />{t('m.a.account')}</a>
						<a role="menuitem" href="/settings/mailboxes"><Icon name="inbox" size={16} />{t('m.a.mailboxes')}</a>
						<form method="POST" action="/logout"><button role="menuitem" type="submit"><Icon name="logout" size={16} />{t('m.a.signOut')}</button></form>
					</div>
				{/if}
			</div>
		</div>
	</header>

	<div class="shell">
		<Sidebar {store} />
		{#if !hasAccounts}
			<main class="main" data-layout="off">
				<section class="pane welcome">
					<div class="mark">
						<svg width="64" height="64" viewBox="0 0 64 64" aria-hidden="true"
							><path d="M6 31c8.5-7.5 17.5-7 26 3.5C40.5 24 49.5 23.5 58 31c-8.6-2.4-17 .8-26 10.8C23 31.8 14.6 28.6 6 31z" fill="var(--accent)" /><path
								d="M14 50c3-2.6 6-2.6 9 0s6 2.6 9 0 6-2.6 9 0 6 2.6 9 0"
								fill="none"
								stroke="var(--accent)"
								stroke-width="3.6"
								stroke-linecap="round"
								opacity=".55"
							/></svg
						>
					</div>
					<h1>{t('m.empty.noAccounts')}</h1>
					<p>{t('m.empty.noAccountsSub')}</p>
					<div class="protocols"><span>JMAP</span><span>IMAP</span><span>POP3</span><span>SMTP</span></div>
					<a class="btn primary" href="/settings/mailboxes"><Icon name="plus" size={16} />{t('m.empty.connect')}</a>
				</section>
			</main>
		{:else}
			<main class="main" class:reading={!!store.openId && !!store.item(store.openId)} data-layout={layout} bind:this={main}>
				<MessageList {store} />
				<!-- svelte-ignore a11y_no_static_element_interactions -->
				<div class="divider" onpointerdown={startResize} ondblclick={() => (store.setPref('listW', 420), store.setPref('listH', 44))}></div>
				<Reader {store} mobile={layout === 'off'} />
			</main>
		{/if}
	</div>

	{#if hasAccounts}
		<button class="fab" onclick={() => store.compose()}><Icon name="pencil" /><span>{t('m.compose')}</span></button>
	{/if}

	<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
	<div class="scrim" class:show={store.ui.settings || store.ui.drawer} onclick={() => ((store.ui.settings = false), (store.ui.drawer = false))}></div>
	<Settings {store} {dark} />
	{#if store.ui.palette}<Palette {store} />{/if}
	{#if store.ui.help}<Help {store} />{/if}

	<div class="composes">
		{#each store.composes as c (c.key)}<Compose {store} {c} />{/each}
	</div>

	<div class="toast" class:show={!!store.toast} class:error={store.toast?.error} role="status" aria-live="polite">
		{#if store.toast}
			<span class="tmsg">{store.toast.msg}</span>
			{#if store.toast.undo}
				<button
					onclick={() => {
						const u = store.toast?.undo;
						store.toast = null;
						u?.();
					}}>{t('m.undo')}</button
				>
			{/if}
			<button class="x" onclick={() => (store.toast = null)} aria-label="Close"><Icon name="x" size={16} /></button>
		{/if}
	</div>
</div>
