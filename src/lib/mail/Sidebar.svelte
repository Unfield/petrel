<script lang="ts">
	import { t } from '@ternkit/tern';
	import Icon from './Icon.svelte';
	import { fmtSize } from './util';
	import { keywordOf, ROLE_FOLDERS, type MailStore } from './store.svelte';

	let { store }: { store: MailStore } = $props();

	const acc = $derived(store.account === 'all' ? undefined : store.accountById(store.account));
	// With one account chosen, only the roles it has, and its own folders too.
	const roles = $derived(
		ROLE_FOLDERS.filter(
			(f) =>
				f.id === 'starred' ||
				(f.id === 'snoozed' ? store.accounts.some((a) => (!acc || a.id === acc.id) && a.mailboxes.some((b) => b.role === 'snoozed')) : !acc || acc.mailboxes.some((b) => b.role === f.id))
		)
	);
	// Storage of the chosen account, or of each one in All inboxes.
	const quotas = $derived(store.accounts.filter((a) => a.quota && (!acc || a.id === acc.id)));
	const pct = (q: { used: number; limit: number }) => (q.limit ? Math.min(100, Math.round((q.used / q.limit) * 100)) : 0);
	const custom = $derived(acc ? acc.mailboxes.filter((b) => !b.role) : []);
	// Folders can be made in IMAP and JMAP mailboxes, not POP3.
	const canFolders = $derived(!!acc && acc.protocol !== 'pop3');
	let naming = $state<string | null>(null); // '' for a new folder, else the folder being renamed
	let name = $state('');
	let menu = $state('');
	$effect(() => {
		const close = () => (menu = '');
		window.addEventListener('petrel:closemenus', close);
		return () => window.removeEventListener('petrel:closemenus', close);
	});

	function startNaming(id: string, current = '') {
		naming = id;
		name = current;
		menu = '';
	}
	async function finishNaming() {
		const n = name.trim();
		const id = naming;
		naming = null;
		if (!n || !acc || id === null) return;
		if (id === '') {
			const made = await store.folderOp('create', { account: acc.id, name: n });
			if (made) store.go(made);
		} else store.folderOp('rename', { folder: id, name: n });
	}
	const focus = (el: HTMLInputElement) => el.focus();
	const count = (role: string) => (role === 'inbox' || role === 'junk' ? store.unread(role) : role === 'drafts' ? draftCount() : 0);
	function draftCount() {
		let n = 0;
		for (const a of store.accounts) if (store.account === 'all' || a.id === store.account) for (const b of a.mailboxes) if (b.role === 'drafts') n += b.total;
		return n;
	}
</script>

<nav class="sidebar" aria-label={t('m.folders')}>
	<button class="compose-btn" onclick={() => store.compose()} title={t('m.compose')} disabled={!store.accounts.length}>
		<Icon name="pencil" size={17} /><span>{t('m.compose')}</span>
	</button>

	{#each roles as f (f.id)}
		{@const n = count(f.id)}
		<button class="nav-item" class:active={store.view === 'mail' && store.folder === f.id} class:has-unread={n > 0} title={t(`m.${f.id}`)} onclick={() => store.go(f.id)}>
			<Icon name={f.icon} /><span class="label">{t(`m.${f.id}`)}</span>{#if n}<span class="count">{n}</span>{/if}
		</button>
	{/each}

	<button class="nav-item" class:active={store.view === 'calendar'} title={t('cal.title')} onclick={() => store.showCalendar()}>
		<Icon name="calendar" /><span class="label">{t('cal.title')}</span>
	</button>
	{#if store.view === 'calendar' && store.calendars.length}
		{#each store.calendars as c (c.id)}
			<button class="nav-item cal-item" title="{c.name} · {c.email}" onclick={() => store.toggleCalendar(c.id)} role="switch" aria-checked={!store.hiddenCalendars.has(c.id)}>
				<span class="cbox" style:--c={c.color} class:off={store.hiddenCalendars.has(c.id)}></span><span class="label">{c.name}<span class="sub">{c.email}</span></span>
			</button>
		{/each}
	{/if}
	{#if custom.length || canFolders}
		<div class="nav-sep"></div>
		<div class="nav-section sec-row">
			<span>{t('m.folders')}</span>
			{#if canFolders}<button class="sec-add" onclick={() => startNaming('')} title={t('m.fo.new')} aria-label={t('m.fo.new')}><Icon name="plus" size={14} /></button>{/if}
		</div>
		{#each custom as b (b.id)}
			{#if naming === b.id}
				<form class="naming" onsubmit={(e) => (e.preventDefault(), finishNaming())}>
					<Icon name="folder" /><input use:focus bind:value={name} onblur={finishNaming} onkeydown={(e) => e.key === 'Escape' && (e.stopPropagation(), (naming = null))} aria-label={t('m.fo.name')} />
				</form>
			{:else}
				<div class="folder-row">
					<button class="nav-item" class:active={store.folder === b.id} class:has-unread={b.unread > 0} title={b.name} onclick={() => store.go(b.id)}>
						<Icon name="folder" /><span class="label">{b.name}</span>{#if b.unread}<span class="count">{b.unread}</span>{/if}
					</button>
					<div class="pop-wrap fmenu">
						<button class="fmore" class:open={menu === b.id} onclick={(e) => (e.stopPropagation(), (menu = menu === b.id ? '' : b.id))} aria-label={t('m.a.more')} aria-expanded={menu === b.id}><Icon name="more" size={16} /></button>
						{#if menu === b.id}
							<div class="pop" role="menu">
								<button role="menuitem" onclick={() => startNaming(b.id, b.name)}><Icon name="edit" size={16} />{t('m.fo.rename')}</button>
								{#if b.unread}<button role="menuitem" onclick={() => ((menu = ''), store.folderOp('readAll', { folder: b.id }))}><Icon name="mailOpen" size={16} />{t('m.fo.readAll')}</button>{/if}
								<button role="menuitem" class="danger" onclick={() => ((menu = ''), confirm(t('m.fo.deleteConfirm', { name: b.name, n: b.total })) && store.folderOp('delete', { folder: b.id }))}><Icon name="trash" size={16} />{t('m.fo.delete')}</button>
							</div>
						{/if}
					</div>
				</div>
			{/if}
		{/each}
		{#if naming === ''}
			<form class="naming" onsubmit={(e) => (e.preventDefault(), finishNaming())}>
				<Icon name="folder" /><input use:focus bind:value={name} placeholder={t('m.fo.name')} onblur={finishNaming} onkeydown={(e) => e.key === 'Escape' && (e.stopPropagation(), (naming = null))} aria-label={t('m.fo.name')} />
			</form>
		{/if}
	{/if}

	{#if store.prefs.labels.length}
		<div class="nav-sep"></div>
		<div class="nav-section">{t('m.labels')}</div>
		{#each store.prefs.labels as l (l.name)}
			{@const id = 'label:' + keywordOf(l.name)}
			<button class="nav-item" class:active={store.folder === id} title={l.name} onclick={() => store.go(id)}>
				<span class="ldot" style:background={l.color} style:margin-inline="4px"></span><span class="label">{l.name}</span>
			</button>
		{/each}
	{/if}

	{#if store.accounts.length > 1 || store.accounts.some((a) => a.error)}
		<div class="nav-sep"></div>
		<div class="nav-section">{t('m.accounts')}</div>
		{#if store.accounts.length > 1}
			{@const n = store.unread('inbox', 'all')}
			<button class="nav-item" class:active={store.account === 'all'} class:has-unread={n > 0} title={t('m.allInboxes')} onclick={() => store.go('inbox', 'all')}>
				<Icon name="layers" /><span class="label">{t('m.allInboxes')}</span>{#if n}<span class="count">{n}</span>{/if}
			</button>
		{/if}
		{#each store.accounts as a (a.id)}
			{@const n = store.unread('inbox', a.id)}
			<button
				class="nav-item"
				class:active={store.account === a.id}
				class:has-unread={n > 0}
				title={a.error ?? a.email}
				onclick={() => store.go('inbox', a.id)}
			>
				{#if a.error}<span class="warn"><Icon name="alert" size={16} /></span>{:else}<span class="dot" style:background={a.color} style:margin-inline="4px"></span>{/if}
				<span class="label">{a.displayName || a.email.split('@')[0]}<span class="sub">{a.error ?? a.email}</span></span>
				{#if n}<span class="count">{n}</span>{/if}
			</button>
		{/each}
	{/if}

	<div class="foot">
		{#each quotas as a (a.id)}
			{@const q = a.quota!}
			<div class="storage" title={a.email}>
				{#if q.limit}<div class="meter" role="meter" aria-valuenow={pct(q)} aria-valuemin={0} aria-valuemax={100} aria-label={t('m.storageOf', { email: a.email })}><i class:full={pct(q) >= 90} style:width="{pct(q)}%"></i></div>{/if}
				{q.limit ? t('m.storage', { used: fmtSize(q.used), total: fmtSize(q.limit) }) : t('m.storageUsed', { used: fmtSize(q.used) })}{#if !acc && store.accounts.length > 1}<span class="sub">{a.email}</span>{/if}
			</div>
		{/each}
		<a class="nav-item manage" href="/settings/mailboxes" title={t('m.a.mailboxes')}><Icon name="plus" /><span class="label">{t('m.a.mailboxes')}</span></a>
	</div>
</nav>

<style>
	.foot {
		margin-top: auto;
		padding-top: 12px;
	}
	.foot .storage {
		margin-top: 0;
		padding-top: 0;
		padding-bottom: 12px;
	}
	.storage .sub {
		display: block;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.meter i.full {
		background: var(--danger);
	}
	.manage {
		text-decoration: none;
		color: var(--text-3);
		font-size: 13px;
	}
	.warn {
		color: var(--danger);
		display: grid;
		place-items: center;
		width: 18px;
	}
	:global(.app.rail) .sec-add,
	:global(.app.rail) .fmenu,
	:global(.app.rail) .naming {
		display: none;
	}
	.cbox {
		width: 14px;
		height: 14px;
		border-radius: 4px;
		background: var(--c);
		border: 2px solid var(--c);
		flex: none;
		margin-inline: 2px;
	}
	.cbox.off {
		background: transparent;
	}
	.cal-item {
		min-height: 30px;
		font-size: 13px;
	}
	.sec-row {
		display: flex;
		align-items: center;
		justify-content: space-between;
	}
	.sec-add {
		border: 0;
		background: none;
		color: var(--text-3);
		width: 22px;
		height: 22px;
		border-radius: 6px;
		display: grid;
		place-items: center;
		cursor: pointer;
		margin-inline-end: -6px;
	}
	.sec-add:hover {
		background: var(--surface-3);
		color: var(--text);
	}
	.folder-row {
		position: relative;
	}
	.fmenu {
		position: absolute;
		inset-inline-end: 6px;
		top: 50%;
		transform: translateY(-50%);
	}
	.fmore {
		border: 0;
		background: var(--surface-3);
		color: var(--text-2);
		width: 26px;
		height: 26px;
		border-radius: 999px;
		display: grid;
		place-items: center;
		cursor: pointer;
		opacity: 0;
	}
	.folder-row:hover .fmore,
	.fmore:focus-visible,
	.fmore.open {
		opacity: 1;
	}
	.folder-row:hover :global(.count) {
		visibility: hidden;
	}
	.fmenu .pop {
		inset-inline-start: auto;
		inset-inline-end: 0;
		z-index: 30;
	}
	.danger {
		color: var(--danger) !important;
	}
	.naming {
		display: flex;
		align-items: center;
		gap: 14px;
		min-height: var(--side-item);
		padding-inline: 14px 8px;
		color: var(--text-2);
	}
	.naming input {
		flex: 1;
		min-width: 0;
		font: inherit;
		height: 28px;
		padding-inline: 8px;
		border-radius: 8px;
		border: 1px solid var(--accent);
		background: var(--surface);
		color: var(--text);
		outline: none;
	}
	.compose-btn:disabled {
		opacity: 0.5;
		cursor: default;
	}
</style>
