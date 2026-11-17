<script lang="ts">
	import { enhance, t } from '@ternkit/tern';
	import { Alert, Badge, Button, Card } from '@ternkit/tern/ui';
	import type { PageProps } from './$types';

	let { data, form }: PageProps = $props();
	const f = $derived(form as { done?: string; emails?: string[]; message?: string; errors?: Record<string, string>; values?: Record<string, string> } | null);
	// Which form the last answer is for, from the fields it echoes back.
	const which = $derived(f?.values ? (f.values.again !== undefined ? 'generate' : f.values.new !== undefined ? 'import' : f.values.key !== undefined ? 'public' : f.values.email !== undefined ? 'lookup' : '') : '');
	const fp = (s: string) => s.replace(/(.{4})/g, '$1 ').trim();
	const date = (s: string) => new Date(s).toLocaleDateString();
	let showImport = $state(false);
	const keyed = $derived(new Set(data.own.map((k) => k.email)));
	const free = $derived(data.addresses.filter((a) => !keyed.has(a.email)));
</script>

<svelte:head><title>{t('acc.pgp.title')} · Petrel</title></svelte:head>

<h1>{t('acc.pgp.title')}</h1>
<p class="lead">{t('acc.pgp.lead')}</p>

<div class="sections">
	<Card title={t('acc.pgp.own')} level={2}>
		{#if f?.done === 'generated'}<Alert tone="success" live>{t('acc.pgp.generated')}</Alert>{/if}
		{#if f?.done === 'imported'}<Alert tone="success" live>{t('acc.pgp.importedOwn')}</Alert>{/if}
		{#if data.own.length}
			<ul class="keys">
				{#each data.own as k (k.id)}
					<li>
						<div class="who">
							<b>{k.email}</b>
							<code>{fp(k.key.fingerprint)}</code>
							<span>{t('acc.pgp.created', { date: date(k.key.created) })}{k.key.expires ? ' · ' + t('acc.pgp.expires', { date: date(k.key.expires) }) : ''}</span>
						</div>
						<Button size="sm" variant="ghost" href="/api/pgp/public?id={k.id}" download="{k.email}.asc">{t('acc.pgp.downloadPublic')}</Button>
						<form method="POST" action="?/delete" use:enhance={({ cancel }) => { if (!confirm(t('acc.pgp.deleteConfirm'))) cancel(); }}>
							<input type="hidden" name="id" value={k.id} />
							<Button type="submit" size="sm" variant="ghost">{t('acc.box.remove')}</Button>
						</form>
					</li>
				{/each}
			</ul>
			<div class="row">
				{#if data.unlocked}
					<Badge tone="success">{t('acc.pgp.unlocked')}</Badge>
					<form method="POST" action="?/lock" use:enhance><Button type="submit" size="sm" variant="secondary">{t('acc.pgp.lock')}</Button></form>
				{:else}
					<span class="muted">{t('acc.pgp.lockedHint')}</span>
				{/if}
			</div>
		{:else}
			<p class="muted">{t('acc.pgp.noOwn')}</p>
		{/if}

		{#if free.length}
			<h3>{t('acc.pgp.generate')}</h3>
			<form class="grid" method="POST" action="?/generate" use:enhance={() => async ({ update }) => update({ reset: true })}>
				<div class="two">
					<label>{t('acc.box.email')}
						<select name="email">{#each free as a (a.id)}<option value={a.email}>{a.email}</option>{/each}</select>
					</label>
					<label>{t('acc.box.name')}<input name="name" value={free[0]?.name ?? ''} /></label>
				</div>
				<div class="two">
					<label>{t('acc.pgp.passphrase')}<input name="passphrase" type="password" autocomplete="new-password" required minlength="8" /></label>
					<label>{t('acc.pgp.again')}<input name="again" type="password" autocomplete="new-password" required /></label>
				</div>
				{#if which === 'generate' && (f?.errors?.passphrase || f?.errors?.again || f?.errors?.email)}<span class="err">{f?.errors?.passphrase || f?.errors?.again || f?.errors?.email}</span>{/if}
				<p class="hint">{t('acc.pgp.passphraseHint')}</p>
				<div class="row">
						<Button type="submit" size="sm">{t('acc.pgp.generateButton')}</Button>
						{#if !showImport}<Button type="button" size="sm" variant="ghost" onclick={() => (showImport = true)}>{t('acc.pgp.importOwn')}</Button>{/if}
					</div>
			</form>
		{/if}

		{#if showImport}
			<h3>{t('acc.pgp.importOwn')}</h3>
			<form class="grid" method="POST" action="?/import" use:enhance>
				<label>{t('acc.pgp.privateKey')}<textarea name="key" rows="6" spellcheck="false" placeholder="-----BEGIN PGP PRIVATE KEY BLOCK-----" required></textarea></label>
				{#if which === 'import' && f?.errors?.key}<span class="err">{f.errors.key}</span>{/if}
				<div class="two">
					<label>{t('acc.pgp.itsPassphrase')}<input name="passphrase" type="password" autocomplete="off" /></label>
					<label>{t('acc.pgp.newPassphrase')} <input name="new" type="password" autocomplete="new-password" /></label>
				</div>
				{#if which === 'import' && f?.errors?.passphrase}<span class="err">{f.errors.passphrase}</span>{/if}
				<div class="row"><Button type="submit" size="sm">{t('acc.pgp.importButton')}</Button><Button size="sm" variant="ghost" onclick={() => (showImport = false)}>{t('acc.box.cancel')}</Button></div>
			</form>
		{:else if !free.length}
			<div class="row gap"><Button size="sm" variant="secondary" onclick={() => (showImport = true)}>{t('acc.pgp.importOwn')}</Button></div>
		{/if}
	</Card>

	<Card title={t('acc.pgp.others')} level={2}>
		<p class="muted">{t('acc.pgp.othersLead')}</p>
		{#if f?.done === 'public'}<Alert tone="success" live>{t('acc.pgp.added', { emails: (f.emails ?? []).join(', ') })}</Alert>{/if}
		{#if data.public.length}
			<ul class="keys">
				{#each data.public as k (k.email)}
					<li>
						<div class="who">
							<b>{k.email}</b>
							<code>{fp(k.key.fingerprint)}</code>
							<span>{t(`acc.pgp.source.${k.source}` as never)}</span>
						</div>
						<form method="POST" action="?/forget" use:enhance>
							<input type="hidden" name="email" value={k.email} />
							<Button type="submit" size="sm" variant="ghost">{t('acc.box.remove')}</Button>
						</form>
					</li>
				{/each}
			</ul>
		{/if}
		<div class="two top">
			<form class="grid" method="POST" action="?/lookup" use:enhance>
				<label>{t('acc.pgp.lookup')}<input name="email" type="email" placeholder="name@example.com" required spellcheck="false" /></label>
				{#if which === 'lookup' && f?.errors?.email}<span class="err">{f.errors.email}</span>{/if}
				<div class="row"><Button type="submit" size="sm" variant="secondary">{t('acc.pgp.lookupButton')}</Button></div>
			</form>
			<form class="grid" method="POST" action="?/public" use:enhance>
				<label>{t('acc.pgp.paste')}<textarea name="key" rows="3" spellcheck="false" placeholder="-----BEGIN PGP PUBLIC KEY BLOCK-----" required></textarea></label>
				{#if which === 'public' && f?.errors?.key}<span class="err">{f.errors.key}</span>{/if}
				<div class="row"><Button type="submit" size="sm" variant="secondary">{t('acc.pgp.addButton')}</Button></div>
			</form>
		</div>
	</Card>
</div>

<style>
	.sections { display: grid; gap: 1rem; }
	.keys { list-style: none; margin: 0 0 0.75rem; padding: 0; }
	.keys li { display: flex; align-items: center; gap: 0.5rem; padding: 0.6rem 0; border-bottom: 1px solid var(--tern-border); }
	.who { flex: 1; min-width: 0; display: grid; gap: 0.1rem; }
	.who code { font-size: 0.78rem; color: var(--tern-muted); overflow-wrap: anywhere; }
	.who span, .muted, .hint { color: var(--tern-muted); font-size: 0.875rem; }
	.muted { margin: 0 0 0.75rem; }
	.hint { margin: -0.25rem 0 0; }
	h3 { font-size: 0.95rem; margin: 1.25rem 0 0.6rem; }
	.grid { display: grid; gap: 0.75rem; }
	.two { display: grid; grid-template-columns: 1fr 1fr; gap: 0.75rem; }
	.two.top { align-items: start; margin-top: 0.5rem; }
	label { display: grid; gap: 0.375rem; font-weight: 550; font-size: 0.9rem; }
	.row { display: flex; flex-wrap: wrap; gap: 0.5rem; align-items: center; }
	.row.gap { margin-top: 1rem; }
	.err { color: var(--tern-danger); font-size: 0.85rem; }
	input, select, textarea {
		font: inherit; font-weight: 400; height: 2.5rem; padding-inline: 0.75rem; border-radius: var(--tern-radius-sm, 8px);
		border: 1px solid var(--tern-border-strong); background: var(--tern-surface); color: var(--tern-fg);
	}
	textarea { height: auto; padding-block: 0.6rem; font-family: ui-monospace, monospace; font-size: 0.8rem; resize: vertical; }
	@media (max-width: 720px) { .two { grid-template-columns: 1fr; } }
</style>
