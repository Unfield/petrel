<script lang="ts">
	import { t } from '@ternkit/tern';
	import { Alert, Button } from '@ternkit/tern/ui';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
</script>

<svelte:head><title>{t('acc.verify.title')} · Petrel</title></svelte:head>

<section class="card" aria-labelledby="verify-title">
	<h1 id="verify-title">{t('acc.verify.title')}</h1>
	{#if data.verified}
		<Alert tone="success">{t('acc.verify.confirmed', { email: data.email })}</Alert>
	{:else}
		<Alert tone="danger">{t('acc.verify.invalid')}</Alert>
	{/if}
	<Button href="/mail" variant="secondary">{t('acc.verify.toMail')}</Button>
</section>

<style>
	.card {
		display: grid;
		gap: 1rem;
		justify-items: start;
		width: 100%;
		max-width: 24rem;
		box-sizing: border-box;
		padding: 2rem;
		background: var(--tern-surface);
		border: 1px solid var(--tern-border);
		border-radius: calc(var(--tern-radius) + 4px);
	}
	h1 {
		margin: 0;
		font-size: 1.375rem;
	}
</style>
