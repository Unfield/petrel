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
		<button class="nav-item" class:active={store.folder === f.id} class:has-unread={n > 0} title={t(`m.${f.id}`)} onclick={() => store.go(f.id)}>
			<Icon name={f.icon} /><span class="label">{t(`m.${f.id}`)}</span>{#if n}<span class="count">{n}</span>{/if}
		</button>
	{/each}

	{#if custom.length}
		<div class="nav-sep"></div>
		<div class="nav-section">{t('m.folders')}</div>
		{#each custom as b (b.id)}
			<button class="nav-item" class:active={store.folder === b.id} class:has-unread={b.unread > 0} title={b.name} onclick={() => store.go(b.id)}>
				<Icon name="folder" /><span class="label">{b.name}</span>{#if b.unread}<span class="count">{b.unread}</span>{/if}
			</button>
		{/each}
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
	.compose-btn:disabled {
		opacity: 0.5;
		cursor: default;
	}
</style>
