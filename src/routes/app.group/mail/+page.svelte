<script lang="ts">
	import { t } from '@ternkit/tern';
	import { Alert, Button, Form } from '@ternkit/tern/ui';
	import Logo from '$lib/Logo.svelte';
	import type { PageProps } from './$types';

	let { data, form }: PageProps = $props();
</script>

<svelte:head><title>{t('mail.title')} · Petrel</title></svelte:head>

{#if !data.emailVerified}
	<Alert tone="warning" title={t('verify.title')}>
		{t('verify.body')}
		<Form action="?/verify" reset={false} class="inline">
			<Button type="submit" variant="secondary" size="sm">{t('verify.resend')}</Button>
		</Form>
		{#if (form as { sent?: boolean } | null)?.sent}<span role="status">{t('verify.sent')}</span>{/if}
	</Alert>
{/if}

{#if data.accounts.length === 0}
	<section class="welcome">
		<div class="mark"><Logo size={56} /></div>
		<h1>{t('mail.emptyTitle')}</h1>
		<p class="lead">{t('mail.emptyBody')}</p>
		<div class="protocols">
			<span>JMAP</span><span>IMAP</span><span>POP3</span><span>SMTP</span>
		</div>
		<Button disabled>{t('mail.connect')}</Button>
		<p class="soon">{t('mail.soon')}</p>
	</section>
{:else}
	<h1>{t('mail.title')}</h1>
	<ul>
		{#each data.accounts as a (a.id)}<li>{a.displayName || a.email} · {a.protocol.toUpperCase()}</li>{/each}
	</ul>
{/if}

<style>
	.welcome {
		display: grid;
		justify-items: center;
		text-align: center;
		gap: 0.75rem;
		padding: 4rem 1rem;
	}
	.mark {
		display: grid;
		place-items: center;
		width: 6rem;
		height: 6rem;
		border-radius: 1.75rem;
		background: var(--tern-accent-soft);
		margin-bottom: 0.5rem;
	}
	.welcome h1 {
		margin: 0;
	}
	.welcome .lead {
		max-width: 32rem;
		margin: 0 0 0.5rem;
	}
	.protocols {
		display: flex;
		gap: 0.5rem;
		margin-bottom: 1rem;
	}
	.protocols span {
		font-size: 0.75rem;
		font-weight: 700;
		letter-spacing: 0.04em;
		padding: 0.25rem 0.625rem;
		border-radius: 999px;
		background: var(--tern-surface-2);
		border: 1px solid var(--tern-border);
		color: var(--tern-muted);
	}
	.soon {
		color: var(--tern-muted);
		font-size: 0.875rem;
		margin: 0;
	}
	:global(.tern-form.inline) {
		display: inline-flex;
		margin-inline: 0.5rem;
	}
</style>
