<script lang="ts">
	import { t } from '@ternkit/tern';
	import Icon from './Icon.svelte';
	import { ROLE_FOLDERS, type MailStore } from './store.svelte';

	let { store }: { store: MailStore } = $props();

	const acc = $derived(store.account === 'all' ? undefined : store.accountById(store.account));
	// With one account chosen, only the roles it has, and its own folders too.
	const roles = $derived(
		ROLE_FOLDERS.filter((f) => f.id === 'starred' || !acc || acc.mailboxes.some((b) => b.role === f.id))
	);
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

	<a class="nav-item manage" href="/settings/mailboxes" title={t('m.a.mailboxes')}><Icon name="plus" /><span class="label">{t('m.a.mailboxes')}</span></a>
</nav>

<style>
	.manage {
		margin-top: auto;
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
