<script lang="ts">
	import { t } from '@ternkit/tern';
	import { Button, SignUp } from '@ternkit/tern/ui';
	import type { PageProps } from './$types';
	import { forms } from './$forms';

	let { data, form }: PageProps = $props();
</script>

<svelte:head><title>{t('acc.signup.title')} · Petrel</title></svelte:head>

<SignUp rules={forms.default} {form} signInHref={data.next === '/mail' ? '/login' : `/login?next=${encodeURIComponent(data.next)}`}>
	{#snippet footer()}
		{#if data.providers.length}
			<div class="providers">
				{#each data.providers as p (p.name)}
					<Button variant="secondary" href="/auth/{p.name}/login?next={encodeURIComponent(data.next)}">{t('acc.signup.continueWith', { provider: p.label })}</Button>
				{/each}
			</div>
		{/if}
	{/snippet}
</SignUp>

<style>
	.providers {
		display: grid;
		gap: 0.5rem;
	}
	.providers :global(.tern-btn) {
		width: 100%;
	}
</style>
