<script lang="ts">
	import { enhance, t, type MessageKey } from '@ternkit/tern';
	import { Alert, Button } from '@ternkit/tern/ui';

	type Condition = { field: string; op: string; value: string };
	type Action = { type: string; value: string };
	type Rule = { id: string; name: string; enabled: boolean; any: boolean; if: Condition[]; then: Action[] };
	type Folder = { path: string; name: string; role: string };

	let { rules, folders, message, saved }: { rules: Rule[]; folders: Folder[]; message?: string; saved: boolean } = $props();

	const FIELDS = ['from', 'to', 'subject', 'body', 'list', 'size', 'header'];
	const TEXT_OPS = ['contains', 'notContains', 'is', 'matches'];
	const ACTIONS = ['move', 'copy', 'label', 'read', 'star', 'forward', 'delete', 'stop'];
	const needsValue = (a: string) => ['move', 'copy', 'label', 'forward'].includes(a);

	let list = $state<Rule[]>([]);
	let editing = $state<Rule | null>(null);
	let editIndex = $state(-1);
	let form = $state<HTMLFormElement>();
	let json = $state('');
	let busy = $state(false);
	$effect.pre(() => {
		list = structuredClone($state.snapshot(rules) as Rule[]);
	});

	const blank = (): Rule => ({ id: '', name: '', enabled: true, any: false, if: [{ field: 'from', op: 'contains', value: '' }], then: [{ type: 'move', value: folders.find((f) => !f.role)?.path ?? folders[0]?.path ?? '' }] });

	function save(next: Rule[]) {
		json = JSON.stringify(next);
		requestAnimationFrame(() => form?.requestSubmit());
	}

	function fieldOf(c: Condition) {
		return c.field.startsWith('header:') ? 'header' : c.field;
	}
	function setField(c: Condition, f: string) {
		c.field = f === 'header' ? 'header:' : f;
		if (f === 'size') ((c.op = 'over'), (c.value = '1000'));
		else if (!TEXT_OPS.includes(c.op)) c.op = 'contains';
	}

	function summary(r: Rule): string {
		const cond = r.if
			.map((c) => {
				const field = fieldOf(c) === 'header' ? c.field.slice(7) : t(`acc.f.field.${fieldOf(c)}` as MessageKey);
				return `${field} ${t(`acc.f.op.${c.op}` as MessageKey)} ${c.field === 'size' ? c.value + ' KB' : '“' + c.value + '”'}`;
			})
			.join(r.any ? ` ${t('acc.f.or')} ` : ` ${t('acc.f.and')} `);
		const act = r.then
			.map((a) => {
				const name = folders.find((f) => f.path === a.value)?.name ?? a.value;
				return t(`acc.f.do.${a.type}` as MessageKey) + (needsValue(a.type) ? ` ${a.type === 'move' || a.type === 'copy' ? name : a.value}` : '');
			})
			.join(', ');
		return `${cond} → ${act}`;
	}

	function edit(i: number) {
		editIndex = i;
		editing = i < 0 ? blank() : structuredClone($state.snapshot(list[i]) as Rule);
	}

	function done() {
		if (!editing) return;
		const r = { ...editing, name: editing.name.trim() || t('acc.f.untitled') };
		const next = editIndex < 0 ? [...list, r] : list.map((x, i) => (i === editIndex ? r : x));
		editing = null;
		save(next);
	}

	function move(i: number, d: number) {
		const next = [...list];
		[next[i], next[i + d]] = [next[i + d], next[i]];
		save(next);
	}
</script>

<form
	bind:this={form}
	method="POST"
	action="?/filters"
	use:enhance={() => {
		busy = true;
		return async ({ update }) => {
			await update({ reset: false });
			busy = false;
		};
	}}
>
	<input type="hidden" name="rules" value={json} />
</form>

{#if message}<Alert tone="danger" live>{message}</Alert>{/if}
{#if saved}<Alert tone="success" live>{t('acc.f.saved')}</Alert>{/if}

{#if list.length}
	<ol class="rules">
		{#each list as r, i (r.id || i)}
			<li class:off={!r.enabled}>
				<button
					type="button"
					class="switch"
					class:on={r.enabled}
					role="switch"
					aria-checked={r.enabled}
					aria-label={t('acc.f.enabled', { name: r.name })}
					disabled={busy}
					onclick={() => save(list.map((x, j) => (j === i ? { ...x, enabled: !x.enabled } : x)))}
				></button>
				<div class="what">
					<b>{r.name}</b>
					<span>{summary(r)}</span>
				</div>
				<div class="tools">
					<button type="button" class="icon" disabled={i === 0 || busy} onclick={() => move(i, -1)} aria-label={t('acc.f.up')}>↑</button>
					<button type="button" class="icon" disabled={i === list.length - 1 || busy} onclick={() => move(i, 1)} aria-label={t('acc.f.down')}>↓</button>
					<Button size="sm" variant="ghost" onclick={() => edit(i)}>{t('acc.f.edit')}</Button>
					<Button size="sm" variant="ghost" disabled={busy} onclick={() => save(list.filter((_, j) => j !== i))}>{t('acc.box.remove')}</Button>
				</div>
			</li>
		{/each}
	</ol>
{:else if !editing}
	<p class="muted">{t('acc.f.none')}</p>
{/if}

{#if editing}
	<div class="editor">
		<label class="name">{t('acc.f.name')}<input bind:value={editing.name} placeholder={t('acc.f.namePlaceholder')} /></label>

		<div class="when">
			<span>{t('acc.f.when')}</span>
			<select bind:value={editing.any} aria-label={t('acc.f.match')}>
				<option value={false}>{t('acc.f.all')}</option>
				<option value={true}>{t('acc.f.any')}</option>
			</select>
		</div>
		{#each editing.if as c, i (i)}
			<div class="line">
				<select value={fieldOf(c)} onchange={(e) => setField(c, e.currentTarget.value)} aria-label={t('acc.f.fieldLabel')}>
					{#each FIELDS as f (f)}<option value={f}>{t(`acc.f.field.${f}` as MessageKey)}</option>{/each}
				</select>
				{#if fieldOf(c) === 'header'}
					<input class="hdr" value={c.field.slice(7)} oninput={(e) => (c.field = 'header:' + e.currentTarget.value)} placeholder="X-Spam-Flag" aria-label={t('acc.f.header')} />
				{/if}
				<select bind:value={c.op} aria-label={t('acc.f.opLabel')}>
					{#each c.field === 'size' ? ['over', 'under'] : TEXT_OPS as o (o)}<option value={o}>{t(`acc.f.op.${o}` as MessageKey)}</option>{/each}
				</select>
				<input bind:value={c.value} type={c.field === 'size' ? 'number' : 'text'} min="1" aria-label={t('acc.f.value')} />
				{#if c.field === 'size'}<span class="unit">KB</span>{/if}
				<button type="button" class="icon" disabled={editing.if.length === 1} onclick={() => editing && (editing.if = editing.if.filter((_, j) => j !== i))} aria-label={t('acc.box.remove')}>×</button>
			</div>
		{/each}
		<button type="button" class="add" onclick={() => editing?.if.push({ field: 'subject', op: 'contains', value: '' })}>+ {t('acc.f.addCondition')}</button>

		<div class="when"><span>{t('acc.f.then')}</span></div>
		{#each editing.then as a, i (i)}
			<div class="line">
				<select bind:value={a.type} aria-label={t('acc.f.actionLabel')}>
					{#each ACTIONS as x (x)}<option value={x}>{t(`acc.f.do.${x}` as MessageKey)}</option>{/each}
				</select>
				{#if a.type === 'move' || a.type === 'copy'}
					<select bind:value={a.value} aria-label={t('acc.f.folder')}>
						{#each folders as f (f.path)}<option value={f.path}>{f.role ? t(`m.${f.role}` as MessageKey) : f.path}</option>{/each}
					</select>
				{:else if a.type === 'label'}
					<input bind:value={a.value} placeholder={t('acc.f.labelPlaceholder')} aria-label={t('acc.f.do.label')} />
				{:else if a.type === 'forward'}
					<input bind:value={a.value} type="email" placeholder="name@example.com" aria-label={t('acc.f.do.forward')} />
				{/if}
				<button type="button" class="icon" disabled={editing.then.length === 1} onclick={() => editing && (editing.then = editing.then.filter((_, j) => j !== i))} aria-label={t('acc.box.remove')}>×</button>
			</div>
		{/each}
		<button type="button" class="add" onclick={() => editing?.then.push({ type: 'label', value: '' })}>+ {t('acc.f.addAction')}</button>

		<div class="row">
			<Button size="sm" disabled={busy} onclick={done}>{t('acc.settings.save')}</Button>
			<Button size="sm" variant="ghost" onclick={() => (editing = null)}>{t('acc.box.cancel')}</Button>
		</div>
	</div>
{:else}
	<div><Button variant="secondary" size="sm" onclick={() => edit(-1)}>{t('acc.f.new')}</Button></div>
{/if}

<style>
	.rules {
		list-style: none;
		margin: 0 0 0.75rem;
		padding: 0;
		display: grid;
	}
	.rules li {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		padding: 0.6rem 0;
		border-bottom: 1px solid var(--tern-border);
	}
	.rules li.off .what {
		opacity: 0.55;
	}
	.what {
		flex: 1;
		min-width: 0;
		display: grid;
	}
	.what span {
		color: var(--tern-muted);
		font-size: 0.85rem;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.tools {
		display: flex;
		align-items: center;
		gap: 2px;
	}
	.icon {
		border: 0;
		background: none;
		color: inherit;
		width: 28px;
		height: 28px;
		border-radius: 6px;
		cursor: pointer;
	}
	.icon:disabled {
		opacity: 0.3;
		cursor: default;
	}
	.switch {
		width: 34px;
		height: 20px;
		border-radius: 999px;
		border: 0;
		background: var(--tern-border-strong);
		position: relative;
		cursor: pointer;
		flex: none;
	}
	.switch::after {
		content: '';
		position: absolute;
		top: 3px;
		inset-inline-start: 3px;
		width: 14px;
		height: 14px;
		border-radius: 50%;
		background: white;
		transition: transform 0.15s;
	}
	.switch.on {
		background: var(--tern-accent);
	}
	.switch.on::after {
		transform: translateX(14px);
	}
	:global([dir='rtl']) .switch.on::after {
		transform: translateX(-14px);
	}
	.editor {
		display: grid;
		gap: 0.6rem;
		padding: 1rem;
		border: 1px solid var(--tern-border);
		border-radius: var(--tern-radius, 8px);
		max-width: 44rem;
	}
	.name {
		display: grid;
		gap: 0.375rem;
		font-weight: 550;
		font-size: 0.9rem;
	}
	.when {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-weight: 600;
		font-size: 0.9rem;
		margin-top: 0.4rem;
	}
	.line {
		display: flex;
		gap: 0.4rem;
		align-items: center;
		flex-wrap: wrap;
	}
	.line input:not(.hdr) {
		flex: 1;
		min-width: 8rem;
	}
	.hdr {
		width: 9rem;
	}
	input,
	select {
		font: inherit;
		font-weight: 400;
		height: 2.25rem;
		padding-inline: 0.6rem;
		border-radius: var(--tern-radius-sm, 8px);
		border: 1px solid var(--tern-border-strong);
		background: var(--tern-surface);
		color: var(--tern-fg);
	}
	.unit {
		color: var(--tern-muted);
		font-size: 0.85rem;
	}
	.add {
		justify-self: start;
		border: 0;
		background: none;
		color: var(--tern-accent);
		font: inherit;
		font-size: 0.875rem;
		font-weight: 600;
		cursor: pointer;
		padding: 0;
	}
	.row {
		display: flex;
		gap: 0.5rem;
		margin-top: 0.4rem;
	}
	.muted {
		color: var(--tern-muted);
		margin: 0 0 0.75rem;
	}
</style>
