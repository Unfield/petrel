<script lang="ts">
	import { t, type MessageKey } from '@ternkit/tern';
	import Icon from './Icon.svelte';
	import LabelMenu from './LabelMenu.svelte';
	import MessageBody from './MessageBody.svelte';
	import SnoozeMenu from './SnoozeMenu.svelte';
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
	let labelOpen = $state(false);
	let snoozeOpen = $state(false);
	let moreOpen = $state(false);
	let pass = $state('');
	async function unlock() {
		if (pass && (await store.unlock(pass))) pass = '';
	}
	$effect(() => {
		const close = () => (moveOpen = labelOpen = snoozeOpen = moreOpen = false);
		window.addEventListener('petrel:closemenus', close);
		return () => window.removeEventListener('petrel:closemenus', close);
	});
	const keywords = $derived([...new Set(it?.msgs.flatMap((m) => m.keywords) ?? [])]);
	let scroller = $state<HTMLElement>();

	$effect(() => {
		// A new conversation starts at the top.
		void store.openId;
		scroller?.scrollTo(0, 0);
		moveOpen = labelOpen = snoozeOpen = moreOpen = false;
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
				<button class="tb" onclick={() => ((moveOpen = !moveOpen), (labelOpen = snoozeOpen = false))} title={t('m.a.moveTo')} aria-expanded={moveOpen}><Icon name="folder" /></button>
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
			<div class="pop-wrap">
				<button class="tb" onclick={() => ((labelOpen = !labelOpen), (moveOpen = snoozeOpen = false))} title={t('m.a.label')} aria-expanded={labelOpen}><Icon name="tag" /></button>
				{#if labelOpen}<LabelMenu {store} {items} onclose={() => (labelOpen = false)} />{/if}
			</div>
			{#if role !== 'snoozed'}
				<div class="pop-wrap">
					<button class="tb" onclick={() => ((snoozeOpen = !snoozeOpen), (moveOpen = labelOpen = false))} title={t('m.a.snooze')} aria-expanded={snoozeOpen}><Icon name="clock" /></button>
					{#if snoozeOpen}<SnoozeMenu {store} {items} onclose={() => (snoozeOpen = false)} />{/if}
				</div>
			{/if}
			<span class="spacer"></span>
			{#if idx >= 0}
				<span class="pos">{idx + 1} / {store.items.length}</span>
				<button class="tb" onclick={() => store.step(-1)} disabled={idx <= 0} title="{t('m.a.prev')} (K)"><Icon name="up" /></button>
				<button class="tb" onclick={() => store.step(1)} disabled={idx >= store.items.length - 1} title="{t('m.a.next')} (J)"><Icon name="down" /></button>
			{/if}
			<button class="tb" class:on={flagged} onclick={() => store.toggleFlag(it.id)} title="{flagged ? t('m.a.unstar') : t('m.a.star')} (S)"><Icon name="star" filled={flagged} /></button>
			{#if last}
				<div class="pop-wrap">
					<button class="tb" onclick={() => ((moreOpen = !moreOpen), (moveOpen = labelOpen = snoozeOpen = false))} title={t('m.a.more')} aria-expanded={moreOpen}><Icon name="more" /></button>
					{#if moreOpen}
						<div class="pop pop-end" role="menu">
							<a role="menuitem" href="/mail/print?id={encodeURIComponent(last.id)}{imagesOn(last) ? '&images=1' : ''}" target="_blank" rel="noopener" onclick={() => (moreOpen = false)}><Icon name="printer" size={16} />{t('m.r.print')}</a>
							<a role="menuitem" href="/api/mail/raw?id={encodeURIComponent(last.id)}" target="_blank" rel="noopener" onclick={() => (moreOpen = false)}><Icon name="code" size={16} />{t('m.r.source')}</a>
							<a role="menuitem" href="/api/mail/raw?id={encodeURIComponent(last.id)}&download=1" download="message.eml" onclick={() => (moreOpen = false)}><Icon name="download" size={16} />{t('m.r.download')}</a>
							{#if last.from[0] && !isMe(last.from[0].email)}
								<button role="menuitem" onclick={() => ((moreOpen = false), store.block(last))}><Icon name="ban" size={16} />{t('m.r.block', { email: last.from[0].email })}</button>
							{/if}
						</div>
					{/if}
				</div>
			{/if}
		</div>
		<div class="reader-scroll" bind:this={scroller}>
			<div class="reader-inner">
				<div class="subject-line">
					<div style="flex:1;min-width:0">
						<h1>{it.latest.subject || t('m.noSubject')}</h1>
						{#if (account && store.accounts.length > 1) || keywords.length}
							<div class="lchips">
								{#if account && store.accounts.length > 1}<span class="lchip" style:--c={account.color}>{account.displayName || account.email}</span>{/if}
								{#each keywords as k (k)}<button class="lchip" style:--c={store.labelColor(k)} onclick={() => store.go('label:' + k)}>{store.labelName(k)}</button>{/each}
							</div>
						{/if}
					</div>
				</div>

				{#each it.msgs as m, i (m.id)}
					{@const s = m.from[0] ?? { email: '?' }}
					{@const full = store.full.get(m.id)}
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
						<article class="msg">
							<header class="msg-head">
								<span class="av lg" style:--h={hue(s.email)}>{initials(s.name || s.email)}</span>
								<div class="mh-who">
									<div>
										<b>{display(s)}</b> <span class="mh-email">&lt;{s.email}&gt;</span>
										{#if full?.unsubscribe}<button class="unsub" onclick={() => store.unsubscribe(m.id)}>{t('m.r.unsubscribe')}</button>{/if}
									</div>
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
								{#if full.pgp}
									{@const g = full.pgp}
									<div class="pgp" class:bad={g.signature?.status === 'bad' || g.error}>
										{#if g.encrypted}<Icon name="lock" size={15} /><span>{g.decrypted ? t('m.pgp.decrypted') : g.locked ? t('m.pgp.locked') : g.error}</span>{/if}
										{#if g.signature}
											<Icon name={g.signature.status === 'good' ? 'seal' : 'alert'} size={15} />
											<span>{g.signature.status === 'good' ? t('m.pgp.goodSig', { email: g.signature.email ?? '' }) : g.signature.status === 'unknown' ? t('m.pgp.unknownSig') : t('m.pgp.badSig')}</span>
										{/if}
										{#if g.locked}
											<input type="password" bind:value={pass} placeholder={t('m.pgp.passphrase')} onkeydown={(e) => e.key === 'Enter' && unlock()} aria-label={t('m.pgp.passphrase')} />
											<button class="btn sm" onclick={unlock}>{t('m.pgp.unlock')}</button>
										{/if}
									</div>
								{/if}
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
											{#if a.type === 'application/pgp-keys' || a.name.endsWith('.asc') && a.name.includes('key')}
												<button class="btn sm" onclick={() => store.importKey(m.id, a.id)}><Icon name="lock" size={14} />{t('m.pgp.importKey')}</button>
											{/if}
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

<style>
	.pgp {
		display: flex;
		align-items: center;
		gap: 8px;
		flex-wrap: wrap;
		margin: 0 18px 10px;
		padding: 8px 12px;
		border-radius: 10px;
		font-size: 13px;
		color: var(--text-2);
		background: color-mix(in oklab, var(--accent) 9%, transparent);
	}
	.pgp.bad {
		background: color-mix(in oklab, var(--danger) 12%, transparent);
		color: var(--text);
	}
	.pgp input {
		font: inherit;
		height: 28px;
		padding-inline: 8px;
		border-radius: 8px;
		border: 1px solid var(--border);
		background: var(--surface);
		color: var(--text);
	}
	.unsub {
		border: 0;
		background: none;
		padding: 0 0 0 6px;
		font: inherit;
		font-size: 12px;
		color: var(--text-3);
		text-decoration: underline;
		cursor: pointer;
	}
	.unsub:hover {
		color: var(--text);
	}
	.pop-end {
		inset-inline-start: auto;
		inset-inline-end: 0;
	}
	.pop a[role='menuitem'] {
		color: inherit;
		text-decoration: none;
	}
</style>
