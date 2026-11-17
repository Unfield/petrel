import { SvelteMap, SvelteSet } from 'svelte/reactivity';
import { t, type MessageKey } from '@ternkit/tern';
import type { Account, Compose, Identity, Item, Message, Prefs, Summary, Toast } from './types';
import { display, escapeHTML, quoteHeader, signatureBlock } from './util';

export const DEFAULT_PREFS: Prefs = {
	mode: 'system',
	preset: 'slate',
	accent: '#3b6cf6',
	density: 'comfortable',
	layout: 'right',
	threads: true,
	rtl: false,
	listW: 420,
	listH: 44,
	rail: false,
	images: [],
	labels: [
		{ name: 'Work', color: '#3b82f6' },
		{ name: 'Personal', color: '#e64980' }
	]
};

/** A label's keyword on the server, as internal/… labelKeyword makes it. */
export const keywordOf = (name: string) =>
	name
		.trim()
		.toLowerCase()
		.replace(/ /g, '-')
		.replace(/[^a-z0-9\-_.\u0080-\uffff]/g, '');

export const ROLE_FOLDERS = [
	{ id: 'inbox', icon: 'inbox' },
	{ id: 'starred', icon: 'star' },
	{ id: 'snoozed', icon: 'clock' },
	{ id: 'drafts', icon: 'file' },
	{ id: 'sent', icon: 'send' },
	{ id: 'archive', icon: 'archive' },
	{ id: 'junk', icon: 'spam' },
	{ id: 'trash', icon: 'trash' }
] as const;

async function api<T>(url: string, init?: RequestInit): Promise<T> {
	const res = await fetch(url, {
		...init,
		headers: { accept: 'application/json', ...(init?.body ? { 'content-type': 'application/json' } : {}) }
	});
	if (!res.ok) {
		let msg = res.statusText;
		try {
			const body = await res.json();
			msg = body.message || body.error || msg;
		} catch {
			/* not JSON */
		}
		throw new Error(msg);
	}
	return res.status === 204 ? (undefined as T) : res.json();
}

const post = <T>(url: string, body: unknown) => api<T>(url, { method: 'POST', body: JSON.stringify(body) });

/** The mail app's state: what's shown, and every action on it. */
export class MailStore {
	accounts = $state<Account[]>([]);
	accountsLoaded = $state(false);
	account = $state<'all' | number>('all');
	folder = $state('inbox'); // a role, "starred", or a folder's ID
	messages = $state<Summary[]>([]);
	total = $state(0);
	loading = $state(false);
	errors = $state<string[]>([]);
	search = $state('');
	filters = new SvelteSet<string>();
	checked = new SvelteSet<string>();
	openId = $state<string | null>(null);
	cursorId = $state<string | null>(null);
	/** The open item as it was when opened, so it stays while the list changes. */
	opened = $state<Item | null>(null);
	full = new SvelteMap<string, Message>();
	failed = new SvelteMap<string, string>();
	expanded = new SvelteSet<string>();
	shownImages = new SvelteSet<string>();
	composes = $state<Compose[]>([]);
	toast = $state<Toast | null>(null);
	ui = $state({ settings: false, palette: false, help: false, drawer: false });
	prefs = $state<Prefs>({ ...DEFAULT_PREFS });

	#seq = 0;
	#listToken = 0;
	#toastTimer: ReturnType<typeof setTimeout> | undefined;

	constructor(prefs: Prefs, accountsCount: number) {
		this.prefs = { ...DEFAULT_PREFS, ...prefs };
		this.accountsLoaded = accountsCount === 0;
	}

	// ─── Derived ───────────────────────────────────────────

	items = $derived.by<Item[]>(() => {
		if (!this.prefs.threads) return this.messages.map((m) => ({ id: m.id, msgs: [m], latest: m }));
		const byThread = new Map<string, Summary[]>();
		for (const m of this.messages) {
			const list = byThread.get(m.threadId);
			if (list) list.push(m);
			else byThread.set(m.threadId, [m]);
		}
		return [...byThread.entries()].map(([id, msgs]) => {
			const sorted = [...msgs].sort((a, b) => +new Date(a.date) - +new Date(b.date));
			return { id, msgs: sorted, latest: sorted[sorted.length - 1] };
		});
	});

	/** The folder's mailboxes in each account the view covers. */
	accountById = (id: number) => this.accounts.find((a) => a.id === id);

	unread(role: string, account: 'all' | number = this.account): number {
		let n = 0;
		for (const a of this.accounts) {
			if (account !== 'all' && a.id !== account) continue;
			for (const b of a.mailboxes) if (b.role === role) n += b.unread;
		}
		return n;
	}

	folderName(): string {
		if (this.folder.startsWith('label:')) return this.labelName(this.folder.slice(6));
		const role = ROLE_FOLDERS.find((f) => f.id === this.folder);
		if (role) {
			const name = t(`m.${role.id}`);
			const acc = this.account !== 'all' ? this.accountById(this.account) : undefined;
			return acc && this.folder === 'inbox' ? `${name} · ${acc.displayName || acc.email}` : name;
		}
		for (const a of this.accounts) for (const b of a.mailboxes) if (b.id === this.folder) return b.name;
		return '';
	}

	/** Whether the folder shown is where deleting is forever. */
	inTrash = $derived(this.folder === 'trash' || this.folder === 'junk' || this.boxRole(this.folder) === 'trash');

	boxRole(id: string): string {
		for (const a of this.accounts) for (const b of a.mailboxes) if (b.id === id) return b.role;
		return '';
	}

	// ─── Loading ───────────────────────────────────────────

	async loadAccounts() {
		try {
			this.accounts = await api<Account[]>('/api/mail/accounts');
		} catch (e) {
			this.notify(t('m.t.error', { x: (e as Error).message }), undefined, true);
		} finally {
			this.accountsLoaded = true;
		}
	}

	#refreshTimer: ReturnType<typeof setTimeout> | undefined;
	/** Refreshes counts and the list soon, once, after changes settle. */
	refreshSoon(delay = 1200) {
		clearTimeout(this.#refreshTimer);
		this.#refreshTimer = setTimeout(() => {
			this.loadAccounts();
			this.loadList({ silent: true });
		}, delay);
	}

	labelName(keyword: string): string {
		return this.prefs.labels.find((l) => keywordOf(l.name) === keyword)?.name ?? keyword;
	}

	labelColor(keyword: string): string {
		return this.prefs.labels.find((l) => keywordOf(l.name) === keyword)?.color ?? '#7d8798';
	}

	query(offset = 0): string {
		const p = new URLSearchParams();
		const role = ROLE_FOLDERS.some((f) => f.id === this.folder) || this.folder.startsWith('label:');
		p.set('account', String(this.account));
		p.set('mailbox', this.folder);
		if (!role) p.set('account', 'all');
		if (this.search.trim()) p.set('q', this.search.trim());
		if (this.filters.has('unread')) p.set('unread', 'true');
		if (this.filters.has('starred')) p.set('flagged', 'true');
		if (this.filters.has('att')) p.set('attachments', 'true');
		if (this.filters.has('week')) p.set('week', 'true');
		p.set('offset', String(offset));
		p.set('limit', '50');
		return p.toString();
	}

	async loadList({ silent = false, more = false } = {}) {
		if (!this.accounts.length && this.accountsLoaded) {
			this.messages = [];
			return;
		}
		const token = ++this.#listToken;
		if (!silent) this.loading = true;
		try {
			const res = await api<{ messages: Summary[]; total: number; errors: string[] }>(
				'/api/mail/messages?' + this.query(more ? this.messages.length : 0)
			);
			if (token !== this.#listToken) return;
			this.messages = more ? [...this.messages, ...res.messages.filter((m) => !this.messages.some((x) => x.id === m.id))] : res.messages;
			this.total = res.total;
			this.errors = res.errors;
		} catch (e) {
			if (token === this.#listToken) this.errors = [(e as Error).message];
		} finally {
			if (token === this.#listToken) this.loading = false;
		}
	}

	go(folder: string, account: 'all' | number = this.account) {
		this.folder = folder;
		this.account = account;
		this.openId = this.cursorId = null;
		this.opened = null;
		this.checked.clear();
		this.ui.drawer = false;
		this.messages = [];
		this.loadList();
	}

	// ─── Opening ───────────────────────────────────────────

	item(id: string | null): Item | null {
		if (!id) return null;
		return this.items.find((i) => i.id === id) ?? (this.opened?.id === id ? this.opened : null);
	}

	open(id: string) {
		const it = this.item(id);
		if (!it) return;
		// A draft opens in the compose window, to finish writing it.
		if (it.latest.mailboxRole === 'drafts') {
			this.editDraft(it.latest);
			return;
		}
		this.openId = this.cursorId = id;
		this.opened = it;
		const unseen = it.msgs.filter((m) => !m.seen);
		// The newest message, and unread ones, open expanded: load them.
		for (const m of it.msgs) if (m === it.latest || !m.seen || this.expanded.has(m.id)) this.load(m.id);
		if (unseen.length) this.setSeen(unseen.map((m) => m.id), true, false);
	}

	async load(id: string) {
		if (this.full.has(id)) return;
		try {
			this.full.set(id, await api<Message>('/api/mail/message?id=' + encodeURIComponent(id)));
			this.failed.delete(id);
		} catch (e) {
			this.failed.set(id, (e as Error).message);
		}
	}

	close() {
		this.openId = null;
		this.opened = null;
	}

	// ─── Actions ───────────────────────────────────────────

	notify(msg: string, undo?: () => void, error = false) {
		clearTimeout(this.#toastTimer);
		this.toast = { id: ++this.#seq, msg, undo, error };
		this.#toastTimer = setTimeout(() => (this.toast = null), undo ? 6500 : error ? 7000 : 3500);
	}

	idsOf(itemIds: string[]): Summary[] {
		const out: Summary[] = [];
		for (const id of itemIds) {
			const it = this.item(id);
			if (it) out.push(...it.msgs);
		}
		return out;
	}

	/** Patches messages in the list and in loaded messages. */
	#patch(ids: string[], patch: Partial<Summary>) {
		const set = new Set(ids);
		this.messages = this.messages.map((m) => (set.has(m.id) ? { ...m, ...patch } : m));
		if (this.opened) this.opened = { ...this.opened, msgs: this.opened.msgs.map((m) => (set.has(m.id) ? { ...m, ...patch } : m)) };
		for (const id of ids) {
			const f = this.full.get(id);
			if (f) this.full.set(id, { ...f, ...patch });
		}
	}

	async #act(ids: string[], action: string, mailbox?: string, extra: Record<string, unknown> = {}): Promise<Record<string, string>> {
		const res = await post<{ moved?: Record<string, string> }>('/api/mail/actions', { ids, action, mailbox, ...extra });
		return res?.moved ?? {};
	}

	setSeen(ids: string[], seen: boolean, refresh = true) {
		const changed = this.messages.filter((m) => ids.includes(m.id) && m.seen !== seen).map((m) => m.id);
		this.#patch(ids, { seen });
		// Keep the sidebar's counts right until the server's arrive.
		this.accounts = this.accounts.map((a) => ({
			...a,
			mailboxes: a.mailboxes.map((b) => {
				const n = this.messages.filter((m) => changed.includes(m.id) && m.mailboxId === b.id).length;
				return n ? { ...b, unread: Math.max(0, b.unread + (seen ? -n : n)) } : b;
			})
		}));
		this.#act(ids, seen ? 'read' : 'unread')
			.then(() => refresh && this.refreshSoon(4000))
			.catch((e) => this.#failed(e));
	}

	setFlag(ids: string[], flagged: boolean) {
		this.#patch(ids, { flagged });
		this.#act(ids, flagged ? 'flag' : 'unflag').catch((e) => this.#failed(e));
	}

	toggleFlag(itemId: string) {
		const msgs = this.idsOf([itemId]);
		const on = !msgs.some((m) => m.flagged);
		// In a conversation, starring stars the newest message.
		const target = on ? [msgs[msgs.length - 1]] : msgs.filter((m) => m.flagged);
		this.setFlag(target.map((m) => m.id), on);
	}

	#failed(e: unknown) {
		this.notify(t('m.t.error', { x: (e as Error).message }), undefined, true);
		this.refreshSoon(300);
	}

	/** Removes items from the view, moves their messages, and offers undo. */
	move(itemIds: string[], action: 'archive' | 'trash' | 'junk' | 'inbox' | 'delete' | 'move' | 'snooze', toast: MessageKey, mailbox?: string, extra: Record<string, unknown> = {}) {
		const msgs = this.idsOf(itemIds);
		if (!msgs.length) return;
		const ids = msgs.map((m) => m.id);
		const next = this.#neighbour(itemIds);
		const before = this.messages;
		this.messages = this.messages.filter((m) => !ids.includes(m.id));
		this.checked.clear();
		if (this.openId && itemIds.includes(this.openId)) {
			if (next && this.prefs.layout !== 'off') this.open(next);
			else this.close();
		}
		const done = this.#act(ids, action, mailbox, extra);
		done.then(() => this.refreshSoon()).catch((e) => {
			this.messages = before;
			this.#failed(e);
		});
		const undo =
			action === 'delete'
				? undefined
				: async () => {
						const moved = await done.catch(() => ({}) as Record<string, string>);
						this.messages = before;
						// Put each message back where it was, by its ID where it is now.
						const byBox = new Map<string, string[]>();
						for (const m of msgs) byBox.set(m.mailboxId, [...(byBox.get(m.mailboxId) ?? []), moved[m.id] ?? m.id]);
						try {
							await Promise.all([...byBox].map(([box, list]) => this.#act(list, 'move', box)));
						} catch (e) {
							this.#failed(e);
						}
						this.refreshSoon(400);
					};
		this.notify(t(toast, { n: itemIds.length, when: String(extra.when ?? '') }), undo);
	}

	label(itemIds: string[], keyword: string, on: boolean) {
		const msgs = this.idsOf(itemIds);
		const ids = msgs.map((m) => m.id);
		this.messages = this.messages.map((m) =>
			ids.includes(m.id) ? { ...m, keywords: on ? [...new Set([...m.keywords, keyword])] : m.keywords.filter((k) => k !== keyword) } : m
		);
		if (this.opened) this.opened = this.item(this.opened.id) ?? this.opened;
		this.#act(ids, on ? 'label' : 'unlabel', undefined, { label: keyword }).catch((e) => this.#failed(e));
		if (!on && this.folder === 'label:' + keyword) this.messages = this.messages.filter((m) => !ids.includes(m.id));
	}

	snooze(itemIds: string[], until: Date) {
		const fmt = until.toLocaleString(undefined, { weekday: 'short', hour: 'numeric', minute: '2-digit' });
		this.move(itemIds, 'snooze', 'm.t.snoozed', undefined, { until: +until, when: fmt });
	}

	archive(ids: string[]) {
		this.move(ids, 'archive', 'm.t.archived');
	}
	trash(ids: string[]) {
		if (this.inTrash) this.move(ids, 'delete', 'm.t.deleted');
		else this.move(ids, 'trash', 'm.t.trashed');
	}
	junk(ids: string[]) {
		if (this.folder === 'junk') this.move(ids, 'inbox', 'm.t.restored');
		else this.move(ids, 'junk', 'm.t.junked');
	}
	restore(ids: string[]) {
		this.move(ids, 'inbox', 'm.t.restored');
	}

	#neighbour(removed: string[]): string | null {
		const items = this.items;
		const idx = items.findIndex((i) => i.id === this.openId);
		for (let j = idx + 1; j < items.length; j++) if (!removed.includes(items[j].id)) return items[j].id;
		for (let j = idx - 1; j >= 0; j--) if (!removed.includes(items[j].id)) return items[j].id;
		return null;
	}

	/** Moves the selection: in the list, or between open conversations. */
	step(dir: number) {
		const items = this.items;
		if (!items.length) return;
		const reading = !!this.openId;
		const off = this.prefs.layout === 'off';
		const cur = off && !reading ? this.cursorId : this.openId;
		let idx = items.findIndex((i) => i.id === cur);
		idx = idx < 0 ? 0 : Math.max(0, Math.min(items.length - 1, idx + dir));
		if (off && !reading) this.cursorId = items[idx].id;
		else this.open(items[idx].id);
		return items[idx].id;
	}

	// ─── Compose ───────────────────────────────────────────

	defaultAccount(): number {
		if (this.account !== 'all') return this.account;
		return this.accounts[0]?.id ?? 0;
	}

	compose(c: Partial<Compose> = {}) {
		const accountId = c.accountId ?? this.defaultAccount();
		const id = this.identity(accountId, c.identityId);
		let html = c.html ?? '';
		// A new message, reply or forward starts with the sender's signature,
		// above any quoted text; a reopened draft has it already.
		if (!c.draftId && id?.signature) html = '<br>' + signatureBlock(id.signature) + html;
		this.composes = [
			...this.composes.map((x) => ({ ...x, min: true, max: false })),
			{ key: ++this.#seq, to: '', cc: '', bcc: '', subject: '', attachments: [], min: false, max: false, showCc: false, ...c, accountId, identityId: id?.id, html }
		];
	}

	/** The identity to send as: the one asked for, else the mailbox's default. */
	identity(accountId: number, id?: number): Identity | undefined {
		const ids = this.accountById(accountId)?.identities ?? [];
		return ids.find((i) => i.id === id) ?? ids.find((i) => i.default) ?? ids[0];
	}

	/** Every identity of every mailbox, for the From menu. */
	get identities(): Identity[] {
		return this.accounts.flatMap((a) => a.identities ?? []);
	}

	isMe = (email?: string) => this.accounts.some((a) => a.email === email?.toLowerCase());

	/** Opens a reply, reply-all or forward of a message, quoting it. */
	async reply(m: Summary, mode: 'reply' | 'replyAll' | 'forward') {
		await this.load(m.id);
		const full = this.full.get(m.id);
		const body = full?.body.html ?? escapeHTML(m.preview);
		const subj = (m.subject || '').replace(/^((re|fwd?|aw|wg)\s*:\s*)+/i, '');
		const from = m.from[0];
		if (mode === 'forward') {
			const head = `${t('m.forwarded')}<br>${escapeHTML(t('m.c.from'))}: ${escapeHTML(display(from))} &lt;${escapeHTML(from?.email ?? '')}&gt;<br>${escapeHTML(t('m.c.subject'))}: ${escapeHTML(m.subject)}`;
			const attachments = (full?.attachments ?? []).filter((a) => !a.inline || !a.cid).map((a) => ({ key: ++this.#seq, name: a.name, size: a.size, type: a.type, message: m.id, part: a.id }));
			this.compose({ accountId: m.accountId, subject: 'Fwd: ' + subj, html: `<br><br><div>${head}<br><br>${body}</div>`, attachments });
			return;
		}
		const replyTo = full?.replyTo?.length ? full.replyTo : m.from;
		const to = (this.isMe(from?.email) ? m.to : replyTo).map((a) => a.email);
		const cc = mode === 'replyAll' ? [...m.to, ...m.cc].map((a) => a.email).filter((e) => !this.isMe(e) && !to.includes(e)) : [];
		this.compose({
			accountId: m.accountId,
			to: to.join(', '),
			cc: cc.join(', '),
			showCc: cc.length > 0,
			subject: 'Re: ' + subj,
			replyTo: m.id,
			html: `<br><br><blockquote>${escapeHTML(quoteHeader(m.date, display(from)))}<br>${body}</blockquote>`
		});
	}

	/** The message replies go to: the open conversation's newest. */
	replyTarget(): Summary | undefined {
		const it = this.item(this.openId);
		return it?.msgs[it.msgs.length - 1];
	}

	closeCompose(key: number) {
		this.composes = this.composes.filter((c) => c.key !== key);
	}

	updateCompose(key: number, patch: Partial<Compose>) {
		this.composes = this.composes.map((c) => (c.key === key ? { ...c, ...patch } : c));
	}

	#payload(c: Compose, html: string, text: string) {
		return {
			accountId: c.accountId, identityId: c.identityId ?? 0, to: c.to, cc: c.cc, bcc: c.bcc, subject: c.subject, html, text, replyTo: c.replyTo ?? '', draftId: c.draftId ?? '',
			attachments: c.attachments.filter((a) => a.upload || a.message).map((a) => ({ upload: a.upload, message: a.message, part: a.part }))
		};
	}

	/** Saves a draft to the account's Drafts, replacing the last save. */
	async saveDraft(c: Compose, html: string, text: string): Promise<boolean> {
		try {
			const res = await post<{ id: string }>('/api/mail/draft', this.#payload(c, html, text));
			if (!res.id) return false;
			this.updateCompose(c.key, { draftId: res.id });
			return true;
		} catch {
			return false;
		}
	}

	async discardDraft(c: Compose) {
		if (c.draftId) await this.#act([c.draftId], 'delete').catch(() => {});
		this.refreshSoon(800);
	}

	/** Opens a saved draft in the compose window. */
	async editDraft(m: Summary) {
		if (this.composes.some((c) => c.draftId === m.id)) return;
		await this.load(m.id);
		const full = this.full.get(m.id);
		if (!full) return;
		const list = (l: { name?: string; email: string }[]) => l.map((a) => (a.name ? `${a.name} <${a.email}>` : a.email)).join(', ');
		this.compose({
			accountId: m.accountId, to: list(full.to), cc: list(full.cc), bcc: list(full.bcc), showCc: full.cc.length + full.bcc.length > 0,
			subject: full.subject, html: full.body.plain ? full.body.html.replace(/^<div class="petrel-plain">|<\/div>$/g, '') : full.body.html, draftId: m.id,
			identityId: this.accountById(m.accountId)?.identities?.find((i) => i.email === full.from[0]?.email.toLowerCase())?.id,
			attachments: full.attachments.map((a) => ({ key: ++this.#seq, name: a.name, size: a.size, type: a.type, message: m.id, part: a.id }))
		});
	}

	/** Sends after a few seconds, so "Undo" can still stop it. */
	send(c: Compose, html: string, text: string) {
		this.closeCompose(c.key);
		let cancelled = false;
		const timer = setTimeout(async () => {
			if (cancelled) return;
			try {
				await post('/api/mail/send', this.#payload(c, html, text));
				this.notify(t('m.t.sent'));
				if (c.replyTo) this.#patch([c.replyTo], { answered: true });
				this.refreshSoon(800);
			} catch (e) {
				this.notify(t('m.t.error', { x: (e as Error).message }), undefined, true);
				this.composes = [...this.composes, { ...c, html, min: false }];
			}
		}, 5000);
		this.notify(t('m.t.sending'), () => {
			cancelled = true;
			clearTimeout(timer);
			this.composes = [...this.composes, { ...c, html, min: false }];
		});
	}

	// ─── Preferences ───────────────────────────────────────

	setPref<K extends keyof Prefs>(k: K, v: Prefs[K]) {
		if (k === 'threads' && this.openId) {
			const it = this.item(this.openId);
			this.close();
			if (it) queueMicrotask(() => this.open(v ? it.latest.threadId : it.latest.id));
		}
		this.prefs = { ...this.prefs, [k]: v };
		this.savePrefs();
	}

	#prefsTimer: ReturnType<typeof setTimeout> | undefined;
	/** Keeps preferences in a cookie (for the first paint) and on the server
	 * (to follow the user to other browsers). */
	savePrefs() {
		const { images: _images, labels: _labels, ...look } = this.prefs;
		document.cookie = `petrel_prefs=${encodeURIComponent(JSON.stringify(look))}; path=/; max-age=31536000; samesite=lax`;
		clearTimeout(this.#prefsTimer);
		this.#prefsTimer = setTimeout(() => post('/api/mail/prefs', this.prefs).catch(() => {}), 800);
	}

	async suggest(q: string): Promise<{ name?: string; email: string }[]> {
		try {
			return await api('/api/mail/contacts?q=' + encodeURIComponent(q));
		} catch {
			return [];
		}
	}
}
