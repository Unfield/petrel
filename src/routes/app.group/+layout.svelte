<script lang="ts">
	import { t } from '@ternkit/tern';
	import { Avatar, Button, Sidebar } from '@ternkit/tern/ui';
	import Logo from '$lib/Logo.svelte';
	import LanguageSwitch from '$lib/LanguageSwitch.svelte';
	import type { LayoutProps } from './$types';

	let { data, children }: LayoutProps = $props();
	const groups = $derived([
		{ items: [{ label: t('nav.mail'), href: '/mail', icon: mailIcon }] },
		{
			items: [
				{ label: t('nav.contacts'), href: '/contacts', icon: contactsIcon },
				{ label: t('nav.mailboxes'), href: '/settings/mailboxes', icon: mailIcon },
				{ label: t('nav.settings'), href: '/settings', icon: settingsIcon },
				{ label: t('nav.security'), href: '/settings/security', icon: securityIcon },
				{ label: t('nav.encryption'), href: '/settings/encryption', icon: keyIcon },
				...(data.showTeams ? [{ label: t('nav.team'), href: '/settings/team', icon: settingsIcon }] : [])
			]
		}
	]);
</script>

{#snippet mailIcon()}
	<svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path d="M3 13l2.5-8h13L21 13v6H3z" /><path d="M3 13h5l1.5 2.5h5L16 13h5" /></svg>
{/snippet}
{#snippet contactsIcon()}
	<svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><rect x="4" y="3" width="16" height="18" rx="2" /><circle cx="12" cy="10" r="3" /><path d="M7.5 17c1-2 2.6-3 4.5-3s3.5 1 4.5 3" /></svg>
{/snippet}
{#snippet keyIcon()}
	<svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><circle cx="8" cy="15" r="4" /><path d="M11 12l9-9M16 7l3 3M14 9l2 2" /></svg>
{/snippet}
{#snippet settingsIcon()}
	<svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><circle cx="12" cy="8" r="4" /><path d="M4 21c0-4 4-6 8-6s8 2 8 6" /></svg>
{/snippet}
{#snippet securityIcon()}
	<svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path d="M12 3l8 3v6c0 5-3.5 8-8 9-4.5-1-8-4-8-9V6z" /></svg>
{/snippet}

<Sidebar {groups} label="App">
	{#snippet header()}
		<div class="head">
			<a class="brand" href="/mail"><Logo size={28} icon />Petrel</a>
		</div>
	{/snippet}
	{#snippet footer()}
		<div class="me">
			<Avatar name={data.user.name || data.user.email} size="2rem" />
			<div class="who">
				<span class="name">{data.user.name || data.user.email}</span>
				{#if data.user.name}<span class="email">{data.user.email}</span>{/if}
			</div>
		</div>
		<div class="actions">
			<LanguageSwitch />
			<form method="POST" action="/logout"><Button type="submit" variant="ghost" size="sm">{t('nav.signOut')}</Button></form>
		</div>
	{/snippet}
	<main id="main" class="content">
		{@render children()}
	</main>
</Sidebar>

<style>
	.head {
		display: grid;
		justify-items: start;
		gap: 0.5rem;
		min-width: 0;
	}
	.head :global(.tern-menu) {
		max-width: 100%;
		margin-inline-start: -0.5rem;
	}
	/* In the phone's top bar: side by side. */
	@media (max-width: 48rem) {
		.head {
			grid-auto-flow: column;
			align-items: center;
			gap: 0.75rem;
		}
		.head :global(.tern-menu) {
			margin-inline-start: 0;
		}
	}
	.actions {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}
	.brand {
		display: inline-flex;
		align-items: center;
		gap: 0.5rem;
		font-size: 1.0625rem;
		letter-spacing: -0.02em;
		font-weight: 700;
		color: var(--tern-fg);
		text-decoration: none;
	}
	.me {
		display: flex;
		align-items: center;
		gap: 0.625rem;
		min-width: 0;
	}
	.who {
		display: grid;
		min-width: 0;
		font-size: 0.875rem;
	}
	.name {
		font-weight: 600;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.email {
		color: var(--tern-muted);
		font-size: 0.8125rem;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.content {
		max-width: 56rem;
		padding: 2rem 1.5rem 4rem;
	}
	.content :global(h1) {
		margin: 0 0 0.25rem;
		font-size: 1.5rem;
		letter-spacing: -0.02em;
	}
	.content :global(.lead) {
		margin: 0 0 2rem;
		color: var(--tern-muted);
	}
</style>
