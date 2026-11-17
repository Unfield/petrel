<script lang="ts">
	import { t } from '@ternkit/tern';
	import { onDestroy, onMount } from 'svelte';
	import Icon from './Icon.svelte';
	import type { MailStore } from './store.svelte';
	import type { Compose, OutFile } from './types';
	import { fmtSize, isMac, signatureBlock } from './util';

	let { store, c }: { store: MailStore; c: Compose } = $props();

	let body = $state<HTMLDivElement>();
	let toInput = $state<HTMLInputElement>();
	let fileInput = $state<HTMLInputElement>();
	let shake = $state(false);
	let dragging = $state(false);
	let savedAt = $state<Date | null>(null);
	let dirty = false;
	let saving = false;
	let editTimer: ReturnType<typeof setTimeout>;
	let saveTimer: ReturnType<typeof setTimeout>;
	let seq = 0;

	const update = (patch: Partial<Compose>) => store.updateCompose(c.key, patch);
	const html = () => body?.innerHTML ?? '';
	const text = () => body?.innerText.trim() ?? '';

	onMount(() => {
		if (body) body.innerHTML = c.html;
		const target = c.to ? body : toInput;
		target?.focus();
		if (target === body && body) {
			const r = document.createRange();
			r.setStart(body, 0);
			r.collapse(true);
			getSelection()?.removeAllRanges();
			getSelection()?.addRange(r);
		}
	});
	onDestroy(() => {
		clearTimeout(editTimer);
		clearTimeout(saveTimer);
	});

	// Every change is kept in the store at once, and saved to the server's
	// Drafts a few seconds after typing stops.
	function changed() {
		dirty = true;
		clearTimeout(editTimer);
		editTimer = setTimeout(() => update({ html: html() }), 400);
		clearTimeout(saveTimer);
		saveTimer = setTimeout(save, 4000);
	}

	async function save() {
		if (!dirty || saving || (!(c.to + c.subject).trim() && !text() && !c.attachments.length)) return;
		if (c.attachments.some((a) => a.progress !== undefined)) return void (saveTimer = setTimeout(save, 2000));
		saving = true;
		dirty = false;
		const latest = store.composes.find((x) => x.key === c.key);
		if (latest && (await store.saveDraft(latest, html(), text()))) savedAt = new Date();
		saving = false;
	}

	function send() {
		if (!(c.to + c.cc + c.bcc).trim()) {
			shake = false;
			requestAnimationFrame(() => (shake = true));
			toInput?.focus();
			store.notify(t('m.t.needTo'), undefined, true);
			return;
		}
		if (c.attachments.some((a) => a.progress !== undefined)) return store.notify(t('m.c.uploading'));
		clearTimeout(saveTimer);
		store.send({ ...c, attachments: c.attachments.filter((a) => !a.error) }, html(), text());
	}

	// Switching the sender switches the signature too, and a different
	// mailbox means a new draft there.
	function from(identityId: number) {
		const next = store.identities.find((i) => i.id === identityId);
		if (!next) return;
		const old = body?.querySelector('[data-signature]');
		if (old && next.signature) old.innerHTML = next.signature;
		else if (old) old.remove();
		else if (next.signature && body) {
			// Above the quoted message of a reply, else at the end.
			const tmp = document.createElement('div');
			tmp.innerHTML = signatureBlock(next.signature);
			const before = body.querySelector(':scope > blockquote');
			body.insertBefore(document.createElement('br'), before);
			body.insertBefore(tmp.firstChild!, before);
		}
		update({ identityId, accountId: next.accountId, ...(next.accountId !== c.accountId ? { draftId: undefined } : {}), html: html() });
		changed();
	}

	function fmt(cmd: string) {
		body?.focus();
		if (cmd === 'createLink') {
			const url = prompt('URL', 'https://');
			if (url && /^(https?:|mailto:)/i.test(url)) document.execCommand('createLink', false, url);
		} else document.execCommand(cmd);
		changed();
	}

	function discard() {
		clearTimeout(saveTimer);
		const keep = { ...c, html: html() };
		store.closeCompose(c.key);
		store.discardDraft(c);
		store.notify(t('m.t.discarded'), () => (store.composes = [...store.composes, { ...keep, draftId: undefined, min: false }]));
	}

	async function close() {
		clearTimeout(saveTimer);
		update({ html: html() });
		const hasContent = (c.to + c.subject).trim() || text() || c.attachments.length;
		store.closeCompose(c.key);
		if (!hasContent) return;
		if (await store.saveDraft({ ...c, html: html() }, html(), text())) {
			store.notify(t('m.t.draftSaved'));
			store.refreshSoon(600);
		} else {
			// The account can't keep drafts (POP3): keep it on this device.
			const drafts = JSON.parse(localStorage.getItem('petrel-drafts') || '[]');
			drafts.push({ ...c, html: html(), saved: Date.now() });
			localStorage.setItem('petrel-drafts', JSON.stringify(drafts.slice(-20)));
			store.notify(t('m.t.saved'));
		}
	}

	// ─── Attachments ──────────────────────────────────────

	function patchFile(key: number, patch: Partial<OutFile>) {
		const cur = store.composes.find((x) => x.key === c.key);
		if (cur) update({ attachments: cur.attachments.map((a) => (a.key === key ? { ...a, ...patch } : a)) });
	}

	function attach(files: FileList | File[] | null) {
		for (const f of files ?? []) {
			if (f.size > 25 * 1024 * 1024) {
				store.notify(t('m.t.tooBig', { x: f.name }), undefined, true);
				continue;
			}
			const key = Date.now() * 100 + seq++;
			update({ attachments: [...(store.composes.find((x) => x.key === c.key)?.attachments ?? []), { key, name: f.name, size: f.size, type: f.type, progress: 0 }] });
			const form = new FormData();
			form.append('file', f);
			const xhr = new XMLHttpRequest();
			xhr.open('POST', '/api/mail/upload');
			xhr.setRequestHeader('accept', 'application/json');
			xhr.upload.onprogress = (e) => e.lengthComputable && patchFile(key, { progress: e.loaded / e.total });
			xhr.onload = () => {
				if (xhr.status === 200) {
					const u = JSON.parse(xhr.responseText);
					patchFile(key, { upload: u.id, type: u.type, progress: undefined });
					changed();
				} else {
					let msg = xhr.statusText;
					try {
						msg = JSON.parse(xhr.responseText).message || msg;
					} catch {
						/* not JSON */
					}
					patchFile(key, { progress: undefined, error: msg });
					store.notify(t('m.t.uploadFailed', { x: f.name, y: msg }), undefined, true);
				}
			};
			xhr.onerror = () => patchFile(key, { progress: undefined, error: 'network' });
			xhr.send(form);
		}
	}

	function removeFile(key: number) {
		update({ attachments: c.attachments.filter((a) => a.key !== key) });
		changed();
	}

	function paste(e: ClipboardEvent) {
		const files = [...(e.clipboardData?.files ?? [])];
		if (files.length) {
			e.preventDefault();
			attach(files);
		}
	}

	// ─── Address suggestions ──────────────────────────────

	type Field = 'to' | 'cc' | 'bcc';
	let field = $state<Field | null>(null);
	let options = $state<{ name?: string; email: string }[]>([]);
	let active = $state(0);
	let suggestTimer: ReturnType<typeof setTimeout>;

	function typed(f: Field, value: string) {
		update({ [f]: value });
		changed();
		field = f;
		const term = value.split(/[,;]/).pop()?.trim() ?? '';
		clearTimeout(suggestTimer);
		if (term.length < 1) return void (options = []);
		suggestTimer = setTimeout(async () => {
			const taken = value.toLowerCase();
			options = (await store.suggest(term)).filter((o) => !taken.includes(o.email + ','));
			active = 0;
		}, 120);
	}

	function choose(o: { name?: string; email: string }) {
		if (!field) return;
		const parts = c[field]
			.split(/[,;]/)
			.slice(0, -1)
			.map((s) => s.trim())
			.filter(Boolean);
		parts.push(o.name ? `${o.name.replace(/[",<>]/g, '')} <${o.email}>` : o.email);
		update({ [field]: parts.join(', ') + ', ' });
		options = [];
	}

	function fieldKeys(e: KeyboardEvent) {
		if (!options.length) return;
		if (e.key === 'ArrowDown') (e.preventDefault(), (active = (active + 1) % options.length));
		else if (e.key === 'ArrowUp') (e.preventDefault(), (active = (active - 1 + options.length) % options.length));
		else if (e.key === 'Enter' || e.key === 'Tab') (e.preventDefault(), choose(options[active]));
		else if (e.key === 'Escape') (e.stopPropagation(), (options = []));
	}

	function keydown(e: KeyboardEvent) {
		if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
			e.preventDefault();
			send();
		}
	}

	const mod = isMac() ? '⌘' : 'Ctrl';
	const ext = (n: string) => n.split('.').pop()?.toUpperCase().slice(0, 4) ?? '';
</script>

{#snippet address(f: Field, label: string)}
	<div class="c-field">
		<label for="{f}{c.key}">{label}</label>
		<div class="ac">
			{#if f === 'to'}
				<input
					id="to{c.key}"
					bind:this={toInput}
					value={c.to}
					oninput={(e) => typed('to', e.currentTarget.value)}
					onkeydown={fieldKeys}
					onblur={() => setTimeout(() => field === 'to' && (options = []), 150)}
					autocomplete="off"
					spellcheck="false"
				/>
			{:else}
				<input
					id="{f}{c.key}"
					value={c[f]}
					oninput={(e) => typed(f, e.currentTarget.value)}
					onkeydown={fieldKeys}
					onblur={() => setTimeout(() => field === f && (options = []), 150)}
					autocomplete="off"
					spellcheck="false"
				/>
			{/if}
			{#if field === f && options.length}
				<div class="ac-list" role="listbox">
					{#each options as o, i (o.email)}
						<button class:on={i === active} role="option" aria-selected={i === active} onmousedown={(e) => (e.preventDefault(), choose(o))}>
							<b>{o.name || o.email}</b>{#if o.name}<span>{o.email}</span>{/if}
						</button>
					{/each}
				</div>
			{/if}
		</div>
		{#if f === 'to' && !c.showCc}<button class="cc-toggle" onclick={() => update({ showCc: true })}>{t('m.c.ccBcc')}</button>{/if}
	</div>
{/snippet}

<div
	role="dialog"
	tabindex="-1"
	class="compose"
	class:min={c.min}
	class:max={c.max}
	class:shake
	onkeydown={keydown}
	ondragover={(e) => (e.preventDefault(), (dragging = true))}
	ondragleave={(e) => e.currentTarget === e.target && (dragging = false)}
	ondrop={(e) => (e.preventDefault(), (dragging = false), attach(e.dataTransfer?.files ?? null))}
	aria-label={c.subject || t('m.c.new')}
>
	<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
	<header class="c-head" onclick={() => c.min && update({ min: false })}>
		<span class="c-title">{c.subject || t('m.c.new')}</span>
		<button class="tb" onclick={(e) => (e.stopPropagation(), update({ min: !c.min, max: false }))} title={t('m.c.min')}><Icon name="minus" /></button>
		<button class="tb" onclick={(e) => (e.stopPropagation(), update({ max: !c.max, min: false }))} title={t('m.c.max')}><Icon name="expand" size={16} /></button>
		<button class="tb" onclick={(e) => (e.stopPropagation(), close())} title={t('m.c.close')}><Icon name="x" /></button>
	</header>
	<div class="c-field">
		<label for="from{c.key}">{t('m.c.from')}</label>
		<select id="from{c.key}" class="from" value={c.identityId ?? 0} onchange={(e) => from(+e.currentTarget.value)}>
			{#each store.accounts as a (a.id)}
				{#if a.identities?.length}
					{#each a.identities as i (i.id)}<option value={i.id}>{i.name ? `${i.name} <${i.email}>` : i.email}</option>{/each}
				{/if}
			{/each}
		</select>
	</div>
	{@render address('to', t('m.c.to'))}
	{#if c.showCc}
		{@render address('cc', t('m.c.cc'))}
		{@render address('bcc', t('m.c.bcc'))}
	{/if}
	<div class="c-field">
		<input
			placeholder={t('m.c.subject')}
			value={c.subject}
			oninput={(e) => (update({ subject: e.currentTarget.value }), changed())}
			onkeydown={(e) => e.key === 'Enter' && (e.preventDefault(), body?.focus())}
		/>
	</div>
	<div class="c-body" contenteditable="true" bind:this={body} data-placeholder={t('m.c.body')} oninput={changed} onpaste={paste} role="textbox" aria-multiline="true" tabindex="0"></div>
	{#if c.attachments.length}
		<div class="c-files">
			{#each c.attachments as a (a.key)}
				<div class="c-file" class:error={!!a.error} title={a.error ?? a.name}>
					<span class="att-ic" class:img={a.type.startsWith('image/')} class:doc={!a.type.startsWith('image/') && a.type !== 'application/pdf'}>{ext(a.name)}</span>
					<span class="c-file-meta"><b>{a.name}</b><span>{a.error ?? (a.progress !== undefined ? `${Math.round(a.progress * 100)} %` : fmtSize(a.size))}</span></span>
					{#if a.progress !== undefined}<i class="c-progress" style:width="{a.progress * 100}%"></i>{/if}
					<button class="tb" onclick={() => removeFile(a.key)} aria-label={t('m.s.remove')}><Icon name="x" size={14} /></button>
				</div>
			{/each}
		</div>
	{/if}
	{#if dragging}<div class="c-drop"><Icon name="clip" size={22} />{t('m.c.attachDrop')}</div>{/if}
	<footer class="c-foot">
		<button class="btn primary" onclick={send} title="{mod}+Enter">{t('m.c.send')}</button>
		<button class="tb" onmousedown={(e) => e.preventDefault()} onclick={() => fmt('bold')} title={t('m.c.bold')}><Icon name="bold" size={17} /></button>
		<button class="tb" onmousedown={(e) => e.preventDefault()} onclick={() => fmt('italic')} title={t('m.c.italic')}><Icon name="italic" size={17} /></button>
		<button class="tb" onmousedown={(e) => e.preventDefault()} onclick={() => fmt('insertUnorderedList')} title={t('m.c.list')}><Icon name="list" size={17} /></button>
		<button class="tb" onmousedown={(e) => e.preventDefault()} onclick={() => fmt('createLink')} title={t('m.c.link')}><Icon name="link" size={17} /></button>
		<button class="tb" onclick={() => fileInput?.click()} title={t('m.c.attach')}><Icon name="clip" size={17} /></button>
		<input type="file" multiple hidden bind:this={fileInput} onchange={(e) => (attach(e.currentTarget.files), (e.currentTarget.value = ''))} />
		<span class="c-saved">{savedAt ? t('m.c.saved') : ''}</span><span class="spacer"></span>
		<button class="tb" onclick={discard} title={t('m.c.discard')}><Icon name="trash" size={17} /></button>
	</footer>
</div>
