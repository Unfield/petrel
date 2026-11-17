<script lang="ts">
	import { t } from '@ternkit/tern';
	import { onMount } from 'svelte';
	import MessageBody from '$lib/mail/MessageBody.svelte';
	import { fmtLong } from '$lib/mail/util';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	const m = $derived(data.message);
	const list = (l: { name?: string; email: string }[]) => l.map((a) => (a.name ? `${a.name} <${a.email}>` : a.email)).join(', ');

	// Print once the body has had a moment to lay out its images.
	onMount(() => {
		const timer = setTimeout(() => print(), 700);
		return () => clearTimeout(timer);
	});
</script>

<svelte:head><title>{m.subject || t('m.noSubject')}</title></svelte:head>

<article class="print">
	<h1>{m.subject || t('m.noSubject')}</h1>
	<dl>
		<dt>{t('m.c.from')}</dt><dd>{list(m.from)}</dd>
		<dt>{t('m.c.to')}</dt><dd>{list(m.to)}</dd>
		{#if m.cc.length}<dt>{t('m.c.cc')}</dt><dd>{list(m.cc)}</dd>{/if}
		<dt>{t('m.r.date')}</dt><dd>{fmtLong(m.date)}</dd>
		{#if m.attachments.length}<dt>{t('m.r.attachments')}</dt><dd>{m.attachments.map((a) => a.name).join(', ')}</dd>{/if}
	</dl>
	<MessageBody html={m.body.html} plain={m.body.plain} images={data.images} messageId={m.id} />
	<button class="again" onclick={() => print()}>{t('m.r.print')}</button>
</article>

<style>
	:global(html), :global(body) {
		background: #fff !important;
		color: #111 !important;
	}
	.print {
		max-width: 46rem;
		margin: 0 auto;
		padding: 2rem 1.25rem;
		font: 14px/1.5 ui-sans-serif, system-ui, -apple-system, 'Segoe UI', Roboto, sans-serif;
	}
	h1 {
		font-size: 1.4rem;
		margin: 0 0 0.75rem;
	}
	dl {
		display: grid;
		grid-template-columns: auto 1fr;
		gap: 0.15rem 0.75rem;
		margin: 0 0 1rem;
		padding-bottom: 0.75rem;
		border-bottom: 1px solid #ccc;
	}
	dt {
		color: #666;
	}
	dd {
		margin: 0;
		overflow-wrap: anywhere;
	}
	.print :global(iframe) {
		width: 100%;
		border: 0;
	}
	.again {
		margin-top: 1rem;
		font: inherit;
		padding: 0.4rem 0.9rem;
		border: 1px solid #ccc;
		border-radius: 6px;
		background: #f6f6f6;
		cursor: pointer;
	}
	@media print {
		.again {
			display: none;
		}
		.print {
			padding: 0;
			max-width: none;
		}
	}
</style>
