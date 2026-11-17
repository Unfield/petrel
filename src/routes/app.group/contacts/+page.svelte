<script lang="ts">
	import { enhance, goto, t } from '@ternkit/tern';
	import { Alert, Avatar, Button, Card } from '@ternkit/tern/ui';
	import type { PageProps } from './$types';

	type Contact = { email: string; name: string; phone: string; company: string; notes: string; saved: boolean; uses: number };

	let { data, form }: PageProps = $props();
	const f = $derived(form as { done?: string; n?: number; message?: string; errors?: Record<string, string> } | null);
	let editing = $state<Contact | null>(null);
	let tab = $state<'saved' | 'learned'>('saved');
	let q = $state('');
	let timer: ReturnType<typeof setTimeout>;
	let importForm = $state<HTMLFormElement>();
	$effect.pre(() => {
		q = data.q;
	});
	const list = $derived(tab === 'saved' ? data.saved : data.learned);
	const blank = (): Contact => ({ email: '', name: '', phone: '', company: '', notes: '', saved: true, uses: 0 });

	function search(v: string) {
		q = v;
		clearTimeout(timer);
		timer = setTimeout(() => goto(v ? `?q=${encodeURIComponent(v)}` : '?', { replaceState: true, keepFocus: true, noScroll: true }), 250);
	}
</script>

<svelte:head><title>{t('acc.c.title')} · Petrel</title></svelte:head>

<div class="top">
	<div>
		<h1>{t('acc.c.title')}</h1>
		<p class="lead">{t('acc.c.lead')}</p>
	</div>
	<div class="tools">
		<Button variant="secondary" size="sm" href="/api/contacts/export" download="contacts.vcf">{t('acc.c.export')}</Button>
		<form bind:this={importForm} method="POST" action="?/import" enctype="multipart/form-data" use:enhance>
			<label class="file">
				{t('acc.c.import')}
				<input type="file" name="file" accept=".vcf,text/vcard,text/x-vcard" onchange={() => importForm?.requestSubmit()} />
			</label>
		</form>
		<Button size="sm" onclick={() => (editing = blank())}>{t('acc.c.add')}</Button>
	</div>
</div>

{#if f?.done === 'imported'}<Alert tone="success" live>{t('acc.c.imported', { n: f.n ?? 0 })}</Alert>{/if}
{#if f?.message && !editing}<Alert tone="danger" live>{f.message}</Alert>{/if}

{#if editing}
	<Card title={editing.email ? t('acc.c.edit') : t('acc.c.add')} level={2}>
		<form
			class="grid"
			method="POST"
			action="?/save"
			use:enhance={() =>
				async ({ update, result }) => {
					await update({ reset: false });
					if (result.type === 'success') editing = null;
				}}
		>
			<input type="hidden" name="was" value={editing.saved ? editing.email : ''} />
			<div class="two">
				<label>{t('acc.box.name')}<input name="name" value={editing.name} autocomplete="off" /></label>
				<label>{t('acc.box.email')}<input name="email" type="email" value={editing.email} required spellcheck="false" /></label>
			</div>
			{#if f?.errors?.email}<span class="err">{f.errors.email}</span>{/if}
			<div class="two">
				<label>{t('acc.c.phone')}<input name="phone" type="tel" value={editing.phone} /></label>
				<label>{t('acc.c.company')}<input name="company" value={editing.company} /></label>
			</div>
			<label>{t('acc.c.notes')}<textarea name="notes" rows="3">{editing.notes}</textarea></label>
			<div class="row">
				<Button type="submit" size="sm">{t('acc.settings.save')}</Button>
				<Button size="sm" variant="ghost" onclick={() => (editing = null)}>{t('acc.box.cancel')}</Button>
			</div>
		</form>
	</Card>
{/if}

<div class="bar">
	<div class="tabs" role="tablist">
		<button role="tab" aria-selected={tab === 'saved'} class:on={tab === 'saved'} onclick={() => (tab = 'saved')}>{t('acc.c.saved')} <span>{data.saved.length}</span></button>
		<button role="tab" aria-selected={tab === 'learned'} class:on={tab === 'learned'} onclick={() => (tab = 'learned')}>{t('acc.c.learned')} <span>{data.learned.length}</span></button>
	</div>
	<input class="search" type="search" value={q} oninput={(e) => search(e.currentTarget.value)} placeholder={t('acc.c.search')} aria-label={t('acc.c.search')} />
</div>
{#if tab === 'learned'}<p class="hint">{t('acc.c.learnedHint')}</p>{/if}

{#if list.length}
	<ul class="people">
		{#each list as c (c.email)}
			<li>
				<Avatar name={c.name || c.email} size="2.25rem" />
				<div class="who">
					<b>{c.name || c.email}</b>
					<span>{c.name ? c.email : ''}{c.company ? (c.name ? ' · ' : '') + c.company : ''}{c.phone ? ' · ' + c.phone : ''}</span>
				</div>
				<div class="acts">
					<Button size="sm" variant="ghost" href="/mail?to={encodeURIComponent(c.name ? `${c.name} <${c.email}>` : c.email)}">{t('acc.c.write')}</Button>
					<Button size="sm" variant="ghost" onclick={() => (editing = { ...c })}>{c.saved ? t('acc.f.edit') : t('acc.c.keep')}</Button>
					<form method="POST" action="?/remove" use:enhance>
						<input type="hidden" name="email" value={c.email} />
						<Button type="submit" size="sm" variant="ghost">{t('acc.box.remove')}</Button>
					</form>
				</div>
			</li>
		{/each}
	</ul>
{:else}
	<p class="muted">{q ? t('acc.c.noMatch') : tab === 'saved' ? t('acc.c.none') : t('acc.c.noneLearned')}</p>
{/if}

<style>
	.top { display: flex; justify-content: space-between; align-items: flex-start; gap: 1rem; flex-wrap: wrap; }
	.tools { display: flex; gap: 0.5rem; align-items: center; flex-wrap: wrap; }
	.file { position: relative; display: inline-flex; align-items: center; height: 2rem; padding-inline: 0.75rem; border-radius: var(--tern-radius-sm, 8px); border: 1px solid var(--tern-border-strong); font-size: 0.875rem; font-weight: 550; cursor: pointer; }
	.file input { position: absolute; inset: 0; opacity: 0; cursor: pointer; }
	.bar { display: flex; justify-content: space-between; align-items: center; gap: 1rem; margin: 1.25rem 0 0.5rem; flex-wrap: wrap; }
	.tabs { display: flex; gap: 0.25rem; }
	.tabs button { border: 0; background: none; color: var(--tern-muted); font: inherit; font-weight: 600; padding: 0.4rem 0.75rem; border-radius: 999px; cursor: pointer; }
	.tabs button.on { background: var(--tern-accent-soft, rgb(59 108 246 / 0.15)); color: var(--tern-fg); }
	.tabs span { font-weight: 400; opacity: 0.7; margin-inline-start: 0.25rem; }
	.search { width: min(20rem, 100%); }
	.people { list-style: none; margin: 0; padding: 0; }
	.people li { display: flex; align-items: center; gap: 0.75rem; padding: 0.6rem 0; border-bottom: 1px solid var(--tern-border); }
	.who { flex: 1; min-width: 0; display: grid; }
	.who span { color: var(--tern-muted); font-size: 0.875rem; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
	.acts { display: flex; gap: 2px; align-items: center; }
	.muted, .hint { color: var(--tern-muted); }
	.hint { font-size: 0.875rem; margin: 0 0 0.5rem; }
	.grid { display: grid; gap: 0.875rem; max-width: 40rem; }
	.two { display: grid; grid-template-columns: 1fr 1fr; gap: 0.75rem; }
	label { display: grid; gap: 0.375rem; font-weight: 550; font-size: 0.9rem; }
	.err { color: var(--tern-danger); font-size: 0.85rem; margin-top: -0.5rem; }
	input:not([type='file']), textarea {
		font: inherit; font-weight: 400; height: 2.5rem; padding-inline: 0.75rem; border-radius: var(--tern-radius-sm, 8px);
		border: 1px solid var(--tern-border-strong); background: var(--tern-surface); color: var(--tern-fg);
	}
	textarea { height: auto; padding-block: 0.6rem; resize: vertical; }
	.row { display: flex; gap: 0.5rem; }
	@media (max-width: 640px) { .two { grid-template-columns: 1fr; } .acts { flex-wrap: wrap; justify-content: flex-end; } }
</style>
