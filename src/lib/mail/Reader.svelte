<script lang="ts">
	import { t, type MessageKey } from '@ternkit/tern';
	import Icon from './Icon.svelte';
	import MessageBody from './MessageBody.svelte';
	import type { MailStore } from './store.svelte';
	import type { Summary } from './types';
	import { display, fmtLong, fmtShort, fmtSize, hue, initials } from './util';

	let { store, mobile }: { store: MailStore; mobile: boolean } = $props();

	const it = $derived(store.item(store.openId));
	const idx = $derived(it ? store.items.findIndex((i) => i.id === it.id) : -1);
	const flagged = $derived(!!it?.msgs.some((m) => m.flagged));
	const account = $derived(it ? store.accountById(it.latest.accountId) : undefined);
	const role = $derived(it?.latest.mailboxRole ?? '');
	const offList = $derived(store.prefs.layout === 'off' || mobile);
	let moveOpen = $state(false);
	let scroller = $state<HTMLElement>();

	$effect(() => {
		// A new conversation starts at the top.
		void store.openId;
		scroller?.scrollTo(0, 0);
		moveOpen = false;
	});

	const isOpen = (m: Summary, i: number, n: number) => i === n - 1 || !m.seen || store.expanded.has(m.id);
	const hideMiddle = $derived(!!it && it.msgs.length >= 4 && !store.expanded.has(it.id + ':all'));
	const isMe = (email?: string) => store.accounts.some((a) => a.email === email?.toLowerCase());

	function expand(m: Summary) {
		store.expanded.add(m.id);
		store.load(m.id);
	}

	function imagesOn(m: Summary) {
		const from = m.from[0]?.email ?? '';
		return store.shownImages.has(m.id) || store.prefs.images.includes(from);
	}

	const reply = (m: Summary, mode: 'reply' | 'replyAll' | 'forward') => store.reply(m, mode);

	const items = $derived(it ? [it.id] : []);
	const folders = $derived(account?.mailboxes.filter((b) => b.id !== it?.latest.mailboxId) ?? []);
	const last = $derived(it?.msgs[it.msgs.length - 1]);
</script>

<section class="pane reader-pane" aria-label="Reading pane">
	{#if !it}
		<div class="reader-empty">
			<div class="mark">
				<svg width="56" height="56" viewBox="0 0 64 64" aria-hidden="true"
					><path d="M6 31c8.5-7.5 17.5-7 26 3.5C40.5 24 49.5 23.5 58 31c-8.6-2.4-17 .8-26 10.8C23 31.8 14.6 28.6 6 31z" fill="var(--accent)" opacity=".8" /><path
						d="M14 50c3-2.6 6-2.6 9 0s6 2.6 9 0 6-2.6 9 0 6 2.6 9 0"
						fill="none"
						stroke="var(--accent)"
						stroke-width="3.6"
						stroke-linecap="round"
						opacity=".4"
					/></svg
				>
			</div>
			<b>{t('m.reader.empty')}</b><span>{t('m.reader.emptySub')}</span>
		</div>
	{:else}
		<div class="reader-head">
			{#if offList}<button class="tb" onclick={() => store.close()} title={t('m.a.back')}><Icon name="back" /></button><span class="tb-sep"></span>{/if}
			{#if role === 'trash' || role === 'junk' || role === 'archive'}
				<button class="tb" onclick={() => store.restore(items)} title={t('m.a.restore')}><Icon name="inbox" /></button>
			{:else}
				<button class="tb" onclick={() => store.archive(items)} title="{t('m.a.archive')} (E)"><Icon name="archive" /></button>
			{/if}
			<button class="tb" onclick={() => store.trash(items)} title="{store.inTrash ? t('m.a.deleteForever') : t('m.a.delete')} (#)"><Icon name="trash" /></button>
			<button class="tb" onclick={() => (store.setSeen(it.msgs.map((m) => m.id), false), offList && store.close())} title={t('m.a.unread')}><Icon name="mail" /></button>
			<button class="tb" onclick={() => store.junk(items)} title={role === 'junk' ? t('m.a.notJunk') : t('m.a.junk')}><Icon name="spam" /></button>
			<div class="pop-wrap">
				<button class="tb" onclick={() => (moveOpen = !moveOpen)} title={t('m.a.moveTo')} aria-expanded={moveOpen}><Icon name="folder" /></button>
				{#if moveOpen}
					<div class="pop" role="menu">
						<div class="pop-title">{t('m.a.moveTo')}</div>
						{#each folders as b (b.id)}
							<button role="menuitem" onclick={() => ((moveOpen = false), store.move(items, 'move', 'm.t.moved', b.id))}
								><Icon name={b.role === 'inbox' ? 'inbox' : b.role === 'trash' ? 'trash' : b.role === 'archive' ? 'archive' : b.role === 'junk' ? 'spam' : b.role === 'sent' ? 'send' : b.role === 'drafts' ? 'file' : 'folder'} size={16} />{b.role ? t(`m.${b.role}` as MessageKey) : b.name}</button
							>
						{/each}
					</div>
				{/if}
			</div>
			<span class="spacer"></span>
			{#if idx >= 0}
				<span class="pos">{idx + 1} / {store.items.length}</span>
				<button class="tb" onclick={() => store.step(-1)} disabled={idx <= 0} title="{t('m.a.prev')} (K)"><Icon name="up" /></button>
				<button class="tb" onclick={() => store.step(1)} disabled={idx >= store.items.length - 1} title="{t('m.a.next')} (J)"><Icon name="down" /></button>
			{/if}
			<button class="tb" class:on={flagged} onclick={() => store.toggleFlag(it.id)} title="{flagged ? t('m.a.unstar') : t('m.a.star')} (S)"><Icon name="star" filled={flagged} /></button>
		</div>
		<div class="reader-scroll" bind:this={scroller}>
			<div class="reader-inner">
				<div class="subject-line">
					<div style="flex:1;min-width:0">
						<h1>{it.latest.subject || t('m.noSubject')}</h1>
						{#if account && store.accounts.length > 1}
							<div class="lchips"><span class="lchip" style:--c={account.color}>{account.displayName || account.email}</span></div>
						{/if}
					</div>
				</div>

				{#each it.msgs as m, i (m.id)}
					{@const s = m.from[0] ?? { email: '?' }}
					{#if hideMiddle && i > 0 && i < it.msgs.length - 1}
						{#if i === 1}
							<div class="earlier"><button onclick={() => store.expanded.add(it.id + ':all')}>{t('m.earlier', { n: it.msgs.length - 2 })}</button></div>
						{/if}
					{:else if !isOpen(m, i, it.msgs.length)}
						<article class="msg collapsed">
							<button class="msg-head" onclick={() => expand(m)}>
								<span class="av lg" style:--h={hue(s.email)}>{initials(s.name || s.email)}</span>
								<div class="mh-who"><b>{display(s)}</b><div class="mh-snip">{m.preview}</div></div>
								<div class="mh-side">{#if m.hasAttachment}<Icon name="clip" size={14} />{/if}<time>{fmtShort(m.date)}</time></div>
							</button>
						</article>
					{:else}
						{@const full = store.full.get(m.id)}
						<article class="msg">
							<header class="msg-head">
								<span class="av lg" style:--h={hue(s.email)}>{initials(s.name || s.email)}</span>
								<div class="mh-who">
									<div><b>{display(s)}</b> <span class="mh-email">&lt;{s.email}&gt;</span></div>
									<div class="mh-to">{t('m.toLine', { x: [...m.to, ...m.cc].map((a) => (isMe(a.email) ? t('m.me') : display(a))).join(', ') || '—' })}</div>
								</div>
								<div class="mh-side">
									<time datetime={m.date} title={new Date(m.date).toLocaleString()}>{fmtLong(m.date)}</time>
									<button class="tb" onclick={() => reply(m, 'reply')} title={t('m.a.reply')}><Icon name="reply" /></button>
									{#if it.msgs.length > 1 && i !== it.msgs.length - 1}
										<button class="tb" onclick={() => store.expanded.delete(m.id)} title="Collapse"><Icon name="up" /></button>
									{/if}
								</div>
							</header>
							{#if full}
								{@const files = full.attachments.filter((a) => !a.inline || !a.cid)}
								{#if full.body.hasRemote && !imagesOn(m)}
									<div class="img-banner">
										<Icon name="shield" size={16} /><span style="flex:1;min-width:180px">{t('m.imgBlocked')}</span>
										<button class="btn" onclick={() => store.shownImages.add(m.id)}>{t('m.showImages')}</button>
										<button class="btn" onclick={() => store.setPref('images', [...store.prefs.images, s.email])}>{t('m.alwaysShow')}</button>
									</div>
								{/if}
								<div class="msg-body"><MessageBody html={full.body.html} plain={full.body.plain} images={imagesOn(m)} messageId={m.id} /></div>
								{#if files.length}
									<div class="atts">
										{#each files as a (a.id)}
											{@const ext = a.name.split('.').pop()?.toUpperCase() ?? ''}
											<a class="att" href={a.url + '&download=1'} download={a.name} title="{t('m.a.download')} {a.name}">
												<span class="att-ic" class:img={a.type.startsWith('image/')} class:doc={!a.type.startsWith('image/') && a.type !== 'application/pdf'}>{ext.slice(0, 4)}</span>
												<span style="min-width:0"><div class="att-name">{a.name}</div><div class="att-size">{fmtSize(a.size)}</div></span>
											</a>
										{/each}
									</div>
								{/if}
							{:else if store.failed.get(m.id)}
								<div class="msg-error"><Icon name="alert" size={16} />{store.failed.get(m.id)}</div>
							{:else}
								<div class="msg-loading"><span class="spinner"></span>{t('m.reader.loading')}</div>
							{/if}
						</article>
					{/if}
				{/each}

				{#if last}
					<div class="reply-bar">
						<button class="btn" onclick={() => reply(last, 'reply')}><Icon name="reply" size={16} />{t('m.a.reply')}</button>
						{#if last.to.length + last.cc.length > 1}<button class="btn" onclick={() => reply(last, 'replyAll')}><Icon name="replyAll" size={16} />{t('m.a.replyAll')}</button>{/if}
						<button class="btn" onclick={() => reply(last, 'forward')}><Icon name="forward" size={16} />{t('m.a.forward')}</button>
					</div>
				{/if}
			</div>
		</div>
	{/if}
</section>
