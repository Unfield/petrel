<script lang="ts">
	import { enhance, t, type MessageKey } from '@ternkit/tern';
	import { Alert, Badge, Button, Card } from '@ternkit/tern/ui';
	import type { PageProps } from './$types';

	let { data, form }: PageProps = $props();

	type Settings = {
		protocol: 'imap' | 'jmap' | 'pop3';
		username: string;
		incoming: { host: string; port: number; security: string };
		smtp: { host: string; port: number; security: string };
		jmapUrl?: string;
	};

	let email = $state('');
	let name = $state('');
	let step = $state<'email' | 'details'>('email');
	let finding = $state(false);
	let submitting = $state(false);
	let source = $state('');
	let advanced = $state(false);
	let s = $state<Settings>({
		protocol: 'imap',
		username: '',
		incoming: { host: '', port: 993, security: 'tls' },
		smtp: { host: '', port: 587, security: 'starttls' },
		jmapUrl: ''
	});

	const f = $derived(form as { done?: string; email?: string; message?: string; errors?: Record<string, string> } | null);

	async function find(e: SubmitEvent) {
		e.preventDefault();
		if (!email.includes('@')) return;
		finding = true;
		try {
			const res = await fetch('/api/mail/discover?email=' + encodeURIComponent(email), { headers: { accept: 'application/json' } });
			if (res.ok) {
				const d = (await res.json()) as { settings: Settings; source: string };
				s = { ...s, ...d.settings, incoming: d.settings.incoming?.host ? d.settings.incoming : s.incoming, smtp: d.settings.smtp?.host ? d.settings.smtp : s.smtp };
				source = d.source;
				advanced = d.source === 'guess';
			}
		} finally {
			finding = false;
			step = 'details';
		}
	}

	function useDevServer() {
		s = {
			protocol: 'imap',
			username: email || 'sam@petrel.test',
			incoming: { host: '127.0.0.1', port: 1143, security: 'plain' },
			smtp: { host: '127.0.0.1', port: 1587, security: 'plain' },
			jmapUrl: ''
		};
		if (!email) email = 'sam@petrel.test';
		source = 'dev';
		advanced = true;
		step = 'details';
	}

</script>

<svelte:head><title>{t('acc.mb.title')} · Petrel</title></svelte:head>

<h1>{t('acc.mb.title')}</h1>
<p class="lead">{t('acc.mb.lead')}</p>

<div class="sections">
	<Card title={t('acc.mb.connected')} level={2}>
		{#if f?.done === 'connected'}<Alert tone="success" live>{t('acc.mb.connectedTo', { email: f.email ?? '' })} <a href="/mail">{t('acc.mb.open')}</a></Alert>{/if}
		{#if data.connected}<Alert tone="success" live>{t('acc.mb.connectedTo', { email: data.connected })} <a href="/mail">{t('acc.mb.open')}</a></Alert>{/if}
		{#if data.error}<Alert tone="danger" live>{data.error}</Alert>{/if}
		{#if f?.done === 'removed'}<Alert tone="success" live>{t('acc.mb.removed')}</Alert>{/if}
		{#if data.accounts.length}
			<ul class="accounts">
				{#each data.accounts as a (a.id)}
					<li>
						<span class="dot" style:background={a.color}></span>
						<a class="who" href="/settings/mailboxes/{a.id}"><b>{a.displayName || a.email}</b>{#if a.displayName}<span>{a.email}</span>{/if}</a>
						<Badge>{a.protocol.toUpperCase()}</Badge>
						{#if !data.direct}
							<form method="POST" action="?/remove" use:enhance>
								<input type="hidden" name="id" value={a.id} />
								<Button type="submit" variant="ghost" size="sm">{t('acc.mb.disconnect')}</Button>
							</form>
						{/if}
					</li>
				{/each}
			</ul>
		{:else}
			<p class="muted">{t('acc.mb.none')}</p>
		{/if}
	</Card>

	{#if data.direct}
		<p class="muted">{t('acc.mb.directNote')}</p>
	{:else}
	<Card title={t('acc.mb.connectTitle')} level={2}>
		{#if data.oauth.length}
			<div class="row oauth">
				{#each data.oauth as p (p.name)}<Button href="/api/mail/oauth/start?provider={p.name}" variant="secondary">{t('acc.mb.connectWith', { provider: p.label })}</Button>{/each}
			</div>
			<p class="hint">{t('acc.mb.orAny')}</p>
		{/if}
		{#if step === 'email'}
			<form class="grid" onsubmit={find}>
				<label>{t('acc.mb.email')}<input type="email" bind:value={email} required autocomplete="email" spellcheck="false" placeholder="you@example.com" /></label>
				<div class="row">
					<Button type="submit" disabled={finding}>{finding ? t('acc.mb.finding') : t('acc.mb.continue')}</Button>
					{#if data.allowPrivate}<Button type="button" variant="ghost" onclick={useDevServer}>{t('acc.mb.useDev')}</Button>{/if}
				</div>
			</form>
		{:else}
			<form
				class="grid"
				method="POST"
				action="?/connect"
				use:enhance={() => {
					submitting = true;
					return async ({ update, result }) => {
						await update({ reset: false });
						submitting = false;
						if (result.type === 'success') {
							step = 'email';
							email = '';
						}
					};
				}}
			>
				{#if source}<Alert tone={source === 'guess' ? 'warning' : 'info'}>{t(`acc.mb.src.${source}` as MessageKey)}</Alert>{/if}
				{#if f?.message && !f.done}<Alert tone="danger" live>{f.message}</Alert>{/if}
				<label>{t('acc.mb.email')}<input name="email" type="email" bind:value={email} required /></label>
				<label>{t('acc.mb.yourName')} <span class="opt">{t('acc.mb.yourNameHint')}</span><input name="name" bind:value={name} autocomplete="name" /></label>
				<label>{t('acc.mb.password')}<input name="password" type="password" required autocomplete="current-password" /></label>
				<p class="hint">{t('acc.mb.appPassword')}</p>

				<button type="button" class="toggle" onclick={() => (advanced = !advanced)}>{advanced ? '▾' : '▸'} {t('acc.mb.server')}</button>
				<input type="hidden" name="protocol" value={s.protocol} />
				<div class="advanced" hidden={!advanced}>
					<label>{t('acc.mb.protocol')}
						<select bind:value={s.protocol}>
							<option value="imap">{t('acc.mb.recommended', { protocol: 'IMAP' })}</option>
							<option value="jmap">JMAP</option>
							<option value="pop3">POP3</option>
						</select>
					</label>
					<label>{t('acc.mb.username')}<input name="username" bind:value={s.username} placeholder={email} /></label>
					{#if s.protocol === 'jmap'}
						<label class="wide">{t('acc.mb.jmapUrl')}<input name="jmapUrl" bind:value={s.jmapUrl} placeholder="https://example.com/.well-known/jmap" /></label>
					{:else}
						<fieldset>
							<legend>{t('acc.mb.incoming', { protocol: s.protocol.toUpperCase() })}</legend>
							<label>{t('acc.mb.host')}<input name="host" bind:value={s.incoming.host} /></label>
							<label>{t('acc.mb.port')}<input name="port" type="number" bind:value={s.incoming.port} /></label>
							<label>{t('acc.mb.security')}
								<select name="security" bind:value={s.incoming.security}>
									<option value="tls">TLS</option><option value="starttls">STARTTLS</option>
									{#if data.allowPrivate}<option value="plain">{t('acc.mb.plain')}</option>{/if}
								</select>
							</label>
						</fieldset>
						<fieldset>
							<legend>{t('acc.mb.outgoing')}</legend>
							<label>{t('acc.mb.host')}<input name="smtpHost" bind:value={s.smtp.host} /></label>
							<label>{t('acc.mb.port')}<input name="smtpPort" type="number" bind:value={s.smtp.port} /></label>
							<label>{t('acc.mb.security')}
								<select name="smtpSecurity" bind:value={s.smtp.security}>
									<option value="tls">TLS</option><option value="starttls">STARTTLS</option>
									{#if data.allowPrivate}<option value="plain">{t('acc.mb.plain')}</option>{/if}
								</select>
							</label>
						</fieldset>
					{/if}
				</div>
				{#if s.protocol === 'jmap'}
					<input type="hidden" name="security" value="tls" /><input type="hidden" name="smtpSecurity" value="tls" />
				{/if}
				<div class="row">
					<Button type="submit" disabled={submitting}>{submitting ? t('acc.mb.connecting') : t('acc.mb.connect')}</Button>
					<Button type="button" variant="ghost" onclick={() => (step = 'email')}>{t('acc.mb.back')}</Button>
				</div>
			</form>
		{/if}
	</Card>
	{/if}
</div>

<style>
	.sections { display: grid; gap: 1rem; }
	.accounts { list-style: none; margin: 0; padding: 0; display: grid; gap: 0.25rem; }
	.accounts li { display: flex; align-items: center; gap: 0.75rem; padding: 0.5rem 0; border-bottom: 1px solid var(--tern-border); }
	.accounts li:last-child { border-bottom: 0; }
	.dot { width: 10px; height: 10px; border-radius: 50%; flex: none; }
	.who { flex: 1; display: grid; min-width: 0; color: inherit; text-decoration: none; }
	a.who:hover b { text-decoration: underline; }
	.who span { color: var(--tern-muted); font-size: 0.875rem; }
	.muted { color: var(--tern-muted); margin: 0; }
	.grid { display: grid; gap: 0.875rem; max-width: 34rem; }
	label { display: grid; gap: 0.375rem; font-weight: 550; font-size: 0.9rem; }
	.opt { font-weight: 400; color: var(--tern-muted); }
	input, select {
		font: inherit; font-weight: 400; height: var(--tern-control-height, 2.5rem); padding-inline: 0.75rem; border-radius: var(--tern-radius-sm, 8px);
		border: 1px solid var(--tern-border-strong); background: var(--tern-surface); color: var(--tern-fg);
	}
	input:focus, select:focus { outline: 2px solid var(--tern-ring); outline-offset: 1px; }
	.hint { margin: -0.25rem 0 0; color: var(--tern-muted); font-size: 0.85rem; }
	.row { display: flex; gap: 0.5rem; flex-wrap: wrap; }
	.oauth { margin-bottom: 0.75rem; }
	.toggle { justify-self: start; border: 0; background: none; color: var(--tern-accent); font: inherit; font-weight: 600; cursor: pointer; padding: 0; }
	.advanced { display: grid; gap: 0.875rem; }
	.advanced[hidden] { display: none; }
	fieldset { border: 1px solid var(--tern-border); border-radius: var(--tern-radius); padding: 0.75rem 1rem 1rem; display: grid; grid-template-columns: 1fr 6rem 9rem; gap: 0.75rem; margin: 0; }
	legend { font-weight: 650; font-size: 0.85rem; padding-inline: 0.25rem; }
	@media (max-width: 40rem) { fieldset { grid-template-columns: 1fr 1fr; } fieldset label:first-of-type { grid-column: 1 / -1; } }
</style>
