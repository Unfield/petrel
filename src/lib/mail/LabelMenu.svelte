<script lang="ts">
	import { t } from '@ternkit/tern';
	import Icon from './Icon.svelte';
	import { keywordOf, type MailStore } from './store.svelte';

	let { store, items, onclose }: { store: MailStore; items: string[]; onclose: () => void } = $props();

	let name = $state('');
	const msgs = $derived(store.idsOf(items));
	const has = (k: string) => msgs.length > 0 && msgs.every((m) => m.keywords.includes(k));

	function create(e: SubmitEvent) {
		e.preventDefault();
		const n = name.trim();
		if (!n) return;
		const palette = ['#3b82f6', '#e64980', '#2f9e44', '#f59f00', '#7c5cff', '#0f9d8f', '#e8590c'];
		if (!store.prefs.labels.some((l) => keywordOf(l.name) === keywordOf(n)))
			store.setPref('labels', [...store.prefs.labels, { name: n, color: palette[store.prefs.labels.length % palette.length] }]);
		store.label(items, keywordOf(n), true);
		name = '';
		onclose();
	}
</script>

<div class="pop" role="menu">
	<div class="pop-title">{t('m.a.label')}</div>
	{#each store.prefs.labels as l (l.name)}
		{@const k = keywordOf(l.name)}
		{@const on = has(k)}
		<button class="lbl" role="menuitemcheckbox" aria-checked={on} onclick={() => store.label(items, k, !on)}>
			<span class="box" class:on style:--c={l.color}>{#if on}<Icon name="check" size={12} />{/if}</span>{l.name}
		</button>
	{/each}
	<form onsubmit={create}><input class="newlabel" bind:value={name} placeholder={t('m.a.newLabel')} /></form>
</div>
