<script lang="ts">
	import { t, type MessageKey } from '@ternkit/tern';
	import Icon from './Icon.svelte';
	import type { MailStore } from './store.svelte';
	import type { Item } from './types';
	import { display, first, fmtShort, hue, initials } from './util';

	let { store }: { store: MailStore } = $props();

	const n = $derived(store.checked.size);
	const allChecked = $derived(n > 0 && store.items.every((i) => store.checked.has(i.id)));
	const unreadItems = $derived(store.items.filter((i) => i.msgs.some((m) => !m.seen)).length);
	const filtered = $derived(!!store.search.trim() || store.filters.size > 0);
	const outgoing = $derived(store.folder === 'sent' || store.folder === 'drafts');
	let spinning = $state(false);

	function toggleAll() {
		if (allChecked) store.checked.clear();
		else for (const i of store.items) store.checked.add(i.id);
	}

	function toggle(e: Event, id: string) {
		e.stopPropagation();
		if (store.checked.has(id)) store.checked.delete(id);
		else store.checked.add(id);
	}

	function refresh() {
		spinning = true;
		Promise.all([store.loadAccounts(), store.loadList({ silent: true })]).finally(() => {
			spinning = false;
			store.notify(t('m.t.upToDate'));
		});
	}

	function names(it: Item): string {
		if (outgoing) return t('m.toX', { x: it.latest.to.map(display).join(', ') || '—' });
		const list: string[] = [];
		for (const m of it.msgs) {
			const s = m.from[0];
			const me = store.accounts.some((a) => a.email === s?.email?.toLowerCase());
			const nm = me ? t('m.me') : it.msgs.length > 1 ? first(s) : display(s);
			if (nm && !list.includes(nm)) list.push(nm);
		}
		return list.join(', ') || '—';
	}

	function who(it: Item) {
		if (outgoing) return it.latest.to[0] ?? it.latest.from[0];
		const other = it.msgs.find((m) => !store.accounts.some((a) => a.email === m.from[0]?.email?.toLowerCase()));
		return (other ?? it.latest).from[0] ?? { email: '?' };
	}

	const bulk = (fn: (ids: string[]) => void) => () => fn([...store.checked]);
	const ids = (it: Item) => it.msgs.map((m) => m.id);
</script>

<section class="pane list-pane" aria-label={store.folderName()}>
	{#if n}
		<div class="list-head">
			<button class="icon-btn sm" onclick={toggleAll} title={t('m.a.selectAll')}><Icon name={allChecked ? 'checkSquare' : 'square'} size={20} /></button>
			<span class="bulk-count">{t('m.selected', { n })}</span><span class="spacer"></span>
			{#if !store.inTrash}<button class="tb" onclick={bulk((x) => store.archive(x))} title={t('m.a.archive')}><Icon name="archive" /></button>{/if}
			<button class="tb" onclick={bulk((x) => store.trash(x))} title={store.inTrash ? t('m.a.deleteForever') : t('m.a.delete')}><Icon name="trash" /></button>
			<button class="tb" onclick={bulk((x) => store.setSeen(store.idsOf(x).map((m) => m.id), true))} title={t('m.a.read')}><Icon name="mailOpen" /></button>
			<button class="tb" onclick={bulk((x) => store.setSeen(store.idsOf(x).map((m) => m.id), false))} title={t('m.a.unread')}><Icon name="mail" /></button>
			<button class="tb" onclick={bulk((x) => store.junk(x))} title={store.folder === 'junk' ? t('m.a.notJunk') : t('m.a.junk')}><Icon name="spam" /></button>
			<button class="tb" onclick={() => store.checked.clear()} title={t('m.a.clear')}><Icon name="x" /></button>
		</div>
	{:else}
		<div class="list-head">
			<button class="icon-btn sm" onclick={toggleAll} title={t('m.a.selectAll')} disabled={!store.items.length}><Icon name="square" size={20} /></button>
			<div class="lh-title">
				<div class="list-title">{store.folderName()}</div>
				<div class="list-sub">
					{t(store.prefs.threads ? 'm.convCount' : 'm.msgCount', { n: store.items.length })}{#if unreadItems}&nbsp;· {t('m.unreadCount', { n: unreadItems })}{/if}
				</div>
			</div>
			<span class="spacer"></span>
			<button class="tb" class:spin={spinning || store.loading} onclick={refresh} title={t('m.a.refresh')}><Icon name="refresh" /></button>
		</div>
	{/if}
	<div class="chips">
		{#each ['unread', 'att', 'starred', 'week'] as f (f)}
			<button
				class="chip"
				class:on={store.filters.has(f)}
				onclick={() => {
					if (store.filters.has(f)) store.filters.delete(f);
					else store.filters.add(f);
					store.loadList();
				}}
				>{#if store.filters.has(f)}<Icon name="check" size={14} />{/if}{t(`m.f.${f}` as MessageKey)}</button
			>
		{/each}
	</div>
	{#each store.errors as e (e)}<div class="list-error"><Icon name="alert" size={16} />{e}</div>{/each}
	<div class="list" class:selecting={n > 0} role="listbox" aria-label={store.folderName()}>
		{#if store.loading && !store.items.length}
			{#each Array(8) as _, i (i)}
				<div class="row skeleton"><span class="av"></span><div class="r-main"><i style:width="{40 + ((i * 17) % 30)}%"></i><i style:width="{60 + ((i * 13) % 35)}%"></i></div></div>
			{/each}
		{:else if !store.items.length}
			<div class="empty">
				<div class="big"><Icon name={filtered ? 'search' : 'check'} size={30} /></div>
				<b>{filtered ? t('m.empty.search') : t('m.empty.title')}</b>
				{#if !filtered}<span>{t('m.empty.sub')}</span>{/if}
			</div>
		{:else}
			{#each store.items as it (it.id)}
				{@const unread = it.msgs.some((m) => !m.seen)}
				{@const flagged = it.msgs.some((m) => m.flagged)}
				{@const p = who(it)}
				{@const account = store.accountById(it.latest.accountId)}
				<div
					class="row"
					class:unread
					class:open={store.openId === it.id}
					class:cursor={store.cursorId === it.id && store.openId !== it.id}
					class:checked={store.checked.has(it.id)}
					data-id={it.id}
					role="option"
					aria-selected={store.openId === it.id}
					tabindex="-1"
					onclick={() => store.open(it.id)}
					onkeydown={(e) => e.key === 'Enter' && store.open(it.id)}
				>
					<button class="av-btn" onclick={(e) => toggle(e, it.id)} title={t('m.h.select')}>
						<span class="av" style:--h={hue(p.email)}>{initials(p.name || p.email)}<span class="chk"><Icon name="check" size={14} /></span></span>
					</button>
					<div class="r-main">
						<div class="r-from">
							{#if unread}<span class="udot"></span>{/if}
							{#if it.latest.mailboxRole === 'drafts'}<span class="draft-tag">{t('m.draftTag')}</span>{/if}
							<span class="name">{names(it)}</span>
							{#if it.msgs.length > 1}<span class="r-count">{it.msgs.length}</span>{/if}
						</div>
						<div class="r-meta">
							{#if it.msgs.some((m) => m.hasAttachment)}<Icon name="clip" size={14} />{/if}
							{#if store.account === 'all' && store.accounts.length > 1 && account}<span class="acc-dot" style:background={account.color} title={account.email}></span>{/if}
							<time datetime={it.latest.date}>{fmtShort(it.latest.date)}</time>
							<button class="r-star" class:on={flagged} onclick={(e) => (e.stopPropagation(), store.toggleFlag(it.id))} title={flagged ? t('m.a.unstar') : t('m.a.star')}>
								<Icon name="star" size={16} filled={flagged} />
							</button>
						</div>
						<div class="r-text">
							<span class="r-subj">{it.latest.subject || t('m.noSubject')}</span><span class="r-sep"> — </span><span class="r-snip">{it.latest.preview}</span>
						</div>
					</div>
					<div class="r-actions">
						{#if !store.inTrash}<button onclick={(e) => (e.stopPropagation(), store.archive([it.id]))} title={t('m.a.archive')}><Icon name="archive" size={17} /></button>{/if}
						<button onclick={(e) => (e.stopPropagation(), store.trash([it.id]))} title={store.inTrash ? t('m.a.deleteForever') : t('m.a.delete')}><Icon name="trash" size={17} /></button>
						<button onclick={(e) => (e.stopPropagation(), store.setSeen(ids(it), unread))} title={unread ? t('m.a.read') : t('m.a.unread')}><Icon name={unread ? 'mailOpen' : 'mail'} size={17} /></button>
					</div>
				</div>
			{/each}
			{#if store.messages.length < store.total}
				<div class="more"><button class="btn" onclick={() => store.loadList({ more: true, silent: true })}>{t('m.a.loadMore')}</button></div>
			{/if}
		{/if}
	</div>
</section>
