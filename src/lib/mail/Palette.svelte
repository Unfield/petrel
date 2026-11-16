<script lang="ts">
	import { i18n, setLocale, t } from '@ternkit/tern';
	import { onMount } from 'svelte';
	import Icon from './Icon.svelte';
	import { ROLE_FOLDERS, type MailStore } from './store.svelte';

	let { store }: { store: MailStore } = $props();

	type Cmd = { g: string; icon: string; label: string; kbd?: string; run: () => void };
	let q = $state('');
	let index = $state(0);
	let input = $state<HTMLInputElement>();
	let list = $state<HTMLElement>();
	const close = () => (store.ui.palette = false);

	const commands = $derived.by<Cmd[]>(() => {
		const c: Cmd[] = [];
		const A = t('m.p.actions'), G = t('m.p.goto'), AP = t('m.p.appearance'), L = t('m.p.layout'), LG = t('m.p.language');
		c.push({ g: A, icon: 'pencil', label: t('m.compose'), kbd: 'C', run: () => store.compose() });
		c.push({ g: A, icon: 'sliders', label: t('m.s.title'), run: () => (store.ui.settings = true) });
		c.push({ g: A, icon: 'keyboard', label: t('m.h.title'), kbd: '?', run: () => (store.ui.help = true) });
		c.push({ g: A, icon: 'inbox', label: t('m.a.mailboxes'), run: () => (location.href = '/settings/mailboxes') });
		for (const f of ROLE_FOLDERS) c.push({ g: G, icon: f.icon, label: t('m.p.goTo', { x: t(`m.${f.id}`) }), run: () => store.go(f.id) });
		for (const a of store.accounts) c.push({ g: G, icon: 'layers', label: t('m.p.goTo', { x: a.displayName || a.email }), run: () => store.go('inbox', a.id) });
		for (const [m, i] of [['light', 'sun'], ['dark', 'moon'], ['system', 'monitor']] as const)
			c.push({ g: AP, icon: i, label: t('m.p.mode', { x: t(`m.s.${m}`) }), run: () => store.setPref('mode', m) });
		for (const [id, n] of [['slate', 'Slate'], ['paper', 'Paper'], ['fjord', 'Fjord'], ['contrast', t('m.s.contrast')]] as const)
			c.push({ g: AP, icon: 'palette', label: t('m.p.theme', { x: n }), run: () => store.setPref('preset', id) });
		for (const v of ['right', 'below', 'off'] as const) c.push({ g: L, icon: 'layout', label: t('m.p.pane', { x: t(`m.s.${v}`) }), run: () => store.setPref('layout', v) });
		for (const v of ['compact', 'comfortable', 'spacious'] as const) c.push({ g: L, icon: 'rows', label: t('m.p.density', { x: t(`m.s.${v}`) }), run: () => store.setPref('density', v) });
		c.push({ g: L, icon: 'thread', label: t('m.p.threads'), run: () => store.setPref('threads', !store.prefs.threads) });
		for (const [v, n] of [['en', 'English'], ['de', 'Deutsch']]) if (i18n.locales.includes(v)) c.push({ g: LG, icon: 'globe', label: t('m.p.lang', { x: n }), run: () => setLocale(v) });
		c.push({ g: LG, icon: 'rtl', label: t('m.p.rtl'), run: () => store.setPref('rtl', !store.prefs.rtl) });
		const words = q.toLowerCase().trim().split(/\s+/).filter(Boolean);
		return c.filter((x) => words.every((w) => (x.label + ' ' + x.g).toLowerCase().includes(w)));
	});

	$effect(() => {
		void index;
		list?.querySelector('.p-item.on')?.scrollIntoView({ block: 'nearest' });
	});

	function run(i: number) {
		const c = commands[i];
		close();
		c?.run();
	}

	function keydown(e: KeyboardEvent) {
		const n = Math.max(commands.length, 1);
		if (e.key === 'ArrowDown') (e.preventDefault(), (index = (index + 1) % n));
		else if (e.key === 'ArrowUp') (e.preventDefault(), (index = (index - 1 + n) % n));
		else if (e.key === 'Enter') (e.preventDefault(), run(index));
		else if (e.key === 'Escape') (e.preventDefault(), close());
	}

	onMount(() => input?.focus());
</script>

<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
<div class="modal-wrap" onclick={(e) => e.target === e.currentTarget && close()}>
	<div class="palette" role="dialog" aria-modal="true" aria-label={t('m.a.palette')}>
		<div class="p-input"><Icon name="command" /><input bind:this={input} bind:value={q} oninput={() => (index = 0)} onkeydown={keydown} placeholder={t('m.p.placeholder')} autocomplete="off" spellcheck="false" /><span class="kbd">Esc</span></div>
		<div class="p-list" bind:this={list}>
			{#each commands as c, i (c.label + c.g)}
				{#if i === 0 || commands[i - 1].g !== c.g}<div class="p-group">{c.g}</div>{/if}
				<!-- svelte-ignore a11y_click_events_have_key_events -->
				<div class="p-item" class:on={i === index} role="option" aria-selected={i === index} tabindex="-1" onclick={() => run(i)} onmousemove={() => (index = i)}>
					<Icon name={c.icon} /><span>{c.label}</span>{#if c.kbd}<span class="kbd">{c.kbd}</span>{/if}
				</div>
			{:else}
				<div class="p-empty">{t('m.p.empty')}</div>
			{/each}
		</div>
	</div>
</div>
