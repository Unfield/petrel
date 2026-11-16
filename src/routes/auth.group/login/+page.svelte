<script lang="ts">
	import { enhance } from '@ternkit/tern';
	import { Alert, Button, SignIn } from '@ternkit/tern/ui';
	import type { PageProps } from './$types';
	import { forms } from './$forms';

	let { data, form }: PageProps = $props();
	const f = $derived(form as { message?: string; errors?: Record<string, string> } | null);
	let busy = $state(false);
</script>

<svelte:head><title>Sign in · Petrel</title></svelte:head>

{#if data.direct}
	<!-- Direct mode: the mailbox's own address and password. -->
	<form
		class="direct"
		method="POST"
		action="?/mailbox"
		use:enhance={() => {
			busy = true;
			return async ({ update }) => {
				await update({ reset: false });
				busy = false;
			};
		}}
	>
		<h1>Sign in to your mail</h1>
		<p>Use your email address and its password.</p>
		{#if f?.message}<Alert tone="danger" live>{f.message}</Alert>{/if}
		<input type="hidden" name="next" value={data.next} />
		<label>Email address<input name="email" type="email" autocomplete="email" required spellcheck="false" /></label>
		{#if f?.errors?.email}<span class="err">{f.errors.email}</span>{/if}
		<label>Password<input name="password" type="password" autocomplete="current-password" required /></label>
		<Button type="submit" disabled={busy}>{busy ? 'Signing in…' : 'Sign in'}</Button>
	</form>
{:else}
	<SignIn
		rules={forms.default}
		{form}
		next={data.next}
		providers={data.providers}
		error={data.error || null}
		passkey
		remember
		forgotHref="/forgot-password"
		signUpHref={data.next === '/mail' ? '/signup' : `/signup?next=${encodeURIComponent(data.next)}`}
	/>
{/if}

<style>
	.direct { display: grid; gap: 0.875rem; width: min(24rem, 100%); }
	.direct h1 { margin: 0; font-size: 1.5rem; letter-spacing: -0.02em; }
	.direct p { margin: -0.5rem 0 0.25rem; color: var(--tern-muted); }
	label { display: grid; gap: 0.375rem; font-weight: 550; font-size: 0.9rem; }
	input { font: inherit; font-weight: 400; height: 2.5rem; padding-inline: 0.75rem; border-radius: 8px; border: 1px solid var(--tern-border-strong); background: var(--tern-surface); color: var(--tern-fg); }
	.err { color: var(--tern-danger); font-size: 0.85rem; margin-top: -0.5rem; }
</style>
