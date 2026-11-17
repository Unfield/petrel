<script lang="ts">
	import { t } from '@ternkit/tern';
	import { onMount } from 'svelte';

	// A small rich-text field for signatures and vacation replies: bold,
	// italic, links. It posts its HTML as a hidden input; the server
	// sanitizes it.
	let { name, value = '', label }: { name: string; value?: string; label: string } = $props();

	let box = $state<HTMLDivElement>();
	let html = $state('');

	onMount(() => {
		html = value;
		if (box) box.innerHTML = value;
	});

	function fmt(cmd: string) {
		box?.focus();
		if (cmd === 'createLink') {
			const url = prompt('URL', 'https://');
			if (url && /^(https?:|mailto:)/i.test(url)) document.execCommand('createLink', false, url);
		} else document.execCommand(cmd);
		html = box?.innerHTML ?? '';
	}
</script>

<div class="sig">
	<div class="bar" role="toolbar" aria-label={t('acc.box.format')}>
		<button type="button" onclick={() => fmt('bold')} aria-label={t('m.c.bold')}><b>B</b></button>
		<button type="button" onclick={() => fmt('italic')} aria-label={t('m.c.italic')}><i>I</i></button>
		<button type="button" onclick={() => fmt('createLink')} aria-label={t('m.c.link')}>🔗</button>
		<button type="button" onclick={() => fmt('removeFormat')} aria-label={t('acc.box.plain')}>T̸</button>
	</div>
	<div
		bind:this={box}
		class="edit"
		contenteditable="true"
		role="textbox"
		aria-multiline="true"
		aria-label={label}
		tabindex="0"
		oninput={() => (html = box?.innerHTML ?? '')}
	></div>
	<input type="hidden" {name} value={html} />
</div>

<style>
	.sig {
		border: 1px solid var(--tern-border);
		border-radius: var(--tern-radius, 8px);
		overflow: hidden;
		background: var(--tern-surface);
	}
	.bar {
		display: flex;
		gap: 2px;
		padding: 4px;
		border-bottom: 1px solid var(--tern-border);
	}
	.bar button {
		border: 0;
		background: none;
		color: inherit;
		min-width: 28px;
		height: 28px;
		border-radius: 6px;
		cursor: pointer;
	}
	.bar button:hover {
		background: var(--tern-muted-bg, rgb(128 128 128 / 0.15));
	}
	.edit {
		min-height: 5.5rem;
		padding: 0.6rem 0.75rem;
		outline: none;
		line-height: 1.45;
		font-weight: 400;
		font-size: 0.95rem;
	}
	.edit:focus-visible {
		box-shadow: inset 0 0 0 2px var(--tern-ring);
	}
</style>
