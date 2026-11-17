<script lang="ts">
	import { enhance, t } from '@ternkit/tern';
	import { Alert, Badge, Button, Card } from '@ternkit/tern/ui';
	import SignatureEditor from '$lib/SignatureEditor.svelte';
	import type { PageProps } from './$types';

	let { data, form }: PageProps = $props();
	const f = $derived(form as { done?: string; id?: number; message?: string; errors?: Record<string, string> } | null);
	// Which identity's form the last answer is about.
	let editing = $state<number>(-1);
	let adding = $state(false);
</script>

<svelte:head><title>{data.account.email} · Petrel</title></svelte:head>

<p class="back"><a href="/settings/mailboxes">← {t('acc.mb.title')}</a></p>
<h1><span class="dot" style:background={data.account.color}></span>{data.account.displayName || data.account.email}</h1>
<p class="lead">{t('acc.box.lead', { email: data.account.email })}</p>

{#snippet identityForm(i: { id: number; name: string; email: string; replyTo: string; signature: string; default: boolean })}
	<form
		class="grid"
		method="POST"
		action="?/identity"
		use:enhance={() => {
			editing = i.id;
			return async ({ update, result }) => {
				await update({ reset: false });
				if (result.type === 'success' && i.id === 0) adding = false;
			};
		}}
	>
		<input type="hidden" name="id" value={i.id} />
		<label>{t('acc.box.name')}<input name="name" value={i.name} autocomplete="off" placeholder={t('acc.box.namePlaceholder')} /></label>
		<label
			>{t('acc.box.email')}<input name="email" type="email" value={i.email} readonly={i.default} required spellcheck="false" />
			{#if i.default}<span class="hint">{t('acc.box.defaultEmail')}</span>{:else if i.id === 0}<span class="hint">{t('acc.box.aliasHint')}</span>{/if}
		</label>
		{#if editing === i.id && f?.errors?.email}<span class="err">{f.errors.email}</span>{/if}
		<label><span>{t('acc.box.replyTo')} <span class="opt">{t('acc.box.optional')}</span></span><input name="replyTo" value={i.replyTo} spellcheck="false" /></label>
		{#if editing === i.id && f?.errors?.replyTo}<span class="err">{f.errors.replyTo}</span>{/if}
		<div class="field">
			<span class="flabel">{t('acc.box.signature')}</span>
			<SignatureEditor name="signature" value={i.signature} label={t('acc.box.signature')} />
		</div>
		{#if editing === i.id && f?.message && !f.done}<Alert tone="danger" live>{f.message}</Alert>{/if}
		{#if editing === i.id && f?.done === 'saved'}<Alert tone="success" live>{t('acc.settings.saved')}</Alert>{/if}
		<div class="row">
			<Button type="submit" size="sm">{t('acc.settings.save')}</Button>
			{#if i.id === 0}<Button type="button" size="sm" variant="ghost" onclick={() => (adding = false)}>{t('acc.box.cancel')}</Button>{/if}
		</div>
	</form>
{/snippet}

<div class="sections">
	{#each data.identities as i (i.id)}
		<Card level={2}>
			<div class="head">
				<h2>{i.name || i.email}</h2>
				{#if i.default}<Badge>{t('acc.box.default')}</Badge>{:else}
					<form method="POST" action="?/remove" use:enhance={() => (editing = i.id, undefined)}>
						<input type="hidden" name="id" value={i.id} />
						<Button type="submit" size="sm" variant="ghost">{t('acc.box.remove')}</Button>
					</form>
				{/if}
			</div>
			{@render identityForm(i)}
		</Card>
	{/each}

	{#if adding}
		<Card title={t('acc.box.add')} level={2}>
			{@render identityForm({ id: 0, name: data.identities[0]?.name ?? '', email: '', replyTo: '', signature: data.identities[0]?.signature ?? '', default: false })}
		</Card>
	{:else}
		<div><Button variant="secondary" onclick={() => ((adding = true), (editing = 0))}>{t('acc.box.add')}</Button></div>
	{/if}
</div>

<style>
	.back { margin: 0 0 0.5rem; }
	.back a { color: var(--tern-muted); text-decoration: none; font-size: 0.9rem; }
	h1 { display: flex; align-items: center; gap: 0.6rem; }
	.dot { width: 12px; height: 12px; border-radius: 50%; flex: none; }
	.sections { display: grid; gap: 1rem; }
	.head { display: flex; align-items: center; justify-content: space-between; gap: 1rem; margin-bottom: 0.75rem; }
	.head h2 { margin: 0; font-size: 1.05rem; }
	.grid { display: grid; gap: 0.875rem; max-width: 36rem; }
	label, .field { display: grid; gap: 0.375rem; font-weight: 550; font-size: 0.9rem; }
	.flabel { font-weight: 550; }
	.opt, .hint { font-weight: 400; color: var(--tern-muted); font-size: 0.85rem; }
	.err { color: var(--tern-danger); font-size: 0.85rem; margin-top: -0.5rem; }
	input {
		font: inherit; font-weight: 400; height: var(--tern-control-height, 2.5rem); padding-inline: 0.75rem; border-radius: var(--tern-radius-sm, 8px);
		border: 1px solid var(--tern-border-strong); background: var(--tern-surface); color: var(--tern-fg);
	}
	input[readonly] { color: var(--tern-muted); }
	input:focus { outline: 2px solid var(--tern-ring); outline-offset: 1px; }
	.row { display: flex; gap: 0.5rem; }
</style>
