<script lang="ts">
	import { t } from '@ternkit/tern';
	import { onMount } from 'svelte';
	import Icon from './Icon.svelte';
	import type { MailStore } from './store.svelte';
	import type { Compose } from './types';
	import { isMac } from './util';

	let { store, c }: { store: MailStore; c: Compose } = $props();

	let body = $state<HTMLDivElement>();
	let toInput = $state<HTMLInputElement>();
	let shake = $state(false);
	let timer: ReturnType<typeof setTimeout>;

	const update = (patch: Partial<Compose>) => store.updateCompose(c.key, patch);

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

	// Keeps the text in the store, so it survives minimizing and undo.
	function edited() {
		clearTimeout(timer);
		timer = setTimeout(() => update({ html: body?.innerHTML ?? '' }), 600);
	}

	function send() {
		if (!(c.to + c.cc + c.bcc).trim()) {
			shake = false;
			requestAnimationFrame(() => (shake = true));
			toInput?.focus();
			store.notify(t('m.t.needTo'), undefined, true);
			return;
		}
		store.send(c, body?.innerHTML ?? '', body?.innerText.trim() ?? '');
	}

	function fmt(cmd: string) {
		body?.focus();
		if (cmd === 'createLink') {
			const url = prompt('URL', 'https://');
			if (url && /^(https?:|mailto:)/i.test(url)) document.execCommand('createLink', false, url);
		} else document.execCommand(cmd);
	}

	function discard() {
		const keep = { ...c, html: body?.innerHTML ?? '' };
		store.closeCompose(c.key);
		store.notify(t('m.t.discarded'), () => (store.composes = [...store.composes, { ...keep, min: false }]));
	}

	function close() {
		update({ html: body?.innerHTML ?? '' });
		const hasContent = (c.to + c.subject).trim() || body?.innerText.trim();
		store.closeCompose(c.key);
		if (hasContent) {
			const drafts = JSON.parse(localStorage.getItem('petrel-drafts') || '[]');
			drafts.push({ ...c, html: body?.innerHTML ?? '', saved: Date.now() });
			localStorage.setItem('petrel-drafts', JSON.stringify(drafts.slice(-20)));
			store.notify(t('m.t.saved'));
		}
	}

	function keydown(e: KeyboardEvent) {
		if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
			e.preventDefault();
			send();
		}
	}

	const mod = isMac() ? '⌘' : 'Ctrl';
</script>

<div role="dialog" tabindex="-1" class="compose" class:min={c.min} class:max={c.max} class:shake onkeydown={keydown} aria-label={c.subject || t('m.c.new')}>
	<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
	<header class="c-head" onclick={() => c.min && update({ min: false })}>
		<span class="c-title">{c.subject || t('m.c.new')}</span>
		<button class="tb" onclick={(e) => (e.stopPropagation(), update({ min: !c.min, max: false }))} title={t('m.c.min')}><Icon name="minus" /></button>
		<button class="tb" onclick={(e) => (e.stopPropagation(), update({ max: !c.max, min: false }))} title={t('m.c.max')}><Icon name="expand" size={16} /></button>
		<button class="tb" onclick={(e) => (e.stopPropagation(), close())} title={t('m.c.close')}><Icon name="x" /></button>
	</header>
	<div class="c-field">
		<label for="from{c.key}">{t('m.c.from')}</label>
		<select id="from{c.key}" class="from" value={c.accountId} onchange={(e) => update({ accountId: +e.currentTarget.value })}>
			{#each store.accounts as a (a.id)}<option value={a.id}>{a.displayName ? `${a.displayName} <${a.email}>` : a.email}</option>{/each}
		</select>
	</div>
	<div class="c-field">
		<label for="to{c.key}">{t('m.c.to')}</label>
		<input id="to{c.key}" bind:this={toInput} value={c.to} oninput={(e) => update({ to: e.currentTarget.value })} autocomplete="email" spellcheck="false" />
		{#if !c.showCc}<button class="cc-toggle" onclick={() => update({ showCc: true })}>{t('m.c.ccBcc')}</button>{/if}
	</div>
	{#if c.showCc}
		<div class="c-field">
			<label for="cc{c.key}">{t('m.c.cc')}</label>
			<input id="cc{c.key}" value={c.cc} oninput={(e) => update({ cc: e.currentTarget.value })} spellcheck="false" />
		</div>
		<div class="c-field">
			<label for="bcc{c.key}">{t('m.c.bcc')}</label>
			<input id="bcc{c.key}" value={c.bcc} oninput={(e) => update({ bcc: e.currentTarget.value })} spellcheck="false" />
		</div>
	{/if}
	<div class="c-field">
		<input
			placeholder={t('m.c.subject')}
			value={c.subject}
			oninput={(e) => update({ subject: e.currentTarget.value })}
			onkeydown={(e) => e.key === 'Enter' && (e.preventDefault(), body?.focus())}
		/>
	</div>
	<div class="c-body" contenteditable="true" bind:this={body} data-placeholder={t('m.c.body')} oninput={edited} role="textbox" aria-multiline="true" tabindex="0"></div>
	<footer class="c-foot">
		<button class="btn primary" onclick={send} title="{mod}+Enter">{t('m.c.send')}</button>
		<button class="tb" onmousedown={(e) => e.preventDefault()} onclick={() => fmt('bold')} title={t('m.c.bold')}><Icon name="bold" size={17} /></button>
		<button class="tb" onmousedown={(e) => e.preventDefault()} onclick={() => fmt('italic')} title={t('m.c.italic')}><Icon name="italic" size={17} /></button>
		<button class="tb" onmousedown={(e) => e.preventDefault()} onclick={() => fmt('insertUnorderedList')} title={t('m.c.list')}><Icon name="list" size={17} /></button>
		<button class="tb" onmousedown={(e) => e.preventDefault()} onclick={() => fmt('createLink')} title={t('m.c.link')}><Icon name="link" size={17} /></button>
		<button class="tb" onclick={() => store.notify(t('m.t.attachSoon'))} title={t('m.c.attach')}><Icon name="clip" size={17} /></button>
		<span class="spacer"></span>
		<button class="tb" onclick={discard} title={t('m.c.discard')}><Icon name="trash" size={17} /></button>
	</footer>
</div>
