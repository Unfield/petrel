<script lang="ts">
	import { t } from '@ternkit/tern';
	import { Alert, AlertDialog, Badge, Button, Card, Field, Form, Input, PasswordInput } from '@ternkit/tern/ui';
	import type { PageProps } from './$types';
	import { forms } from './$forms';

	let { data, form }: PageProps = $props();
	const done = $derived((form as { done?: string } | null)?.done);
	// Why the account wasn't deleted, such as a team that needs another owner.
	const deleteError = $derived.by(() => {
		const f = form as { message?: string; values?: { form?: string } } | null;
		return f?.values?.form === 'delete' ? f.message : undefined;
	});
	let deleteOpen = $state(false);
	let deleteForm = $state<HTMLFormElement>();
</script>

<svelte:head><title>{t('acc.settings.title')} · Petrel</title></svelte:head>

<h1>{t('acc.settings.title')}</h1>
<p class="lead">{t('acc.settings.lead')}</p>

<div class="sections">
	<Card title={t('acc.settings.profile')} level={2}>
		<Form action="?/name" rules={forms.name} {form} reset={false}>
			<Field label={t('acc.settings.name')} name="name"><Input value={data.user.name} autocomplete="name" /></Field>
			<div><Button type="submit" size="sm">{t('acc.settings.save')}</Button></div>
		</Form>
		{#if done === 'name'}<Alert tone="success" live>{t('acc.settings.saved')}</Alert>{/if}
	</Card>

	{#if !data.direct}
	<Card title={t('acc.settings.email')} level={2}>
		<div class="email">
			<span>{data.user.email}</span>
			{#if data.user.emailVerified}<Badge tone="success">{t('acc.settings.confirmed')}</Badge>{:else}<Badge tone="warning">{t('acc.settings.notConfirmed')}</Badge>{/if}
			{#if !data.user.emailVerified}
				<Form action="?/verify" reset={false}>
					<Button type="submit" variant="secondary" size="sm">{t('acc.settings.resend')}</Button>
				</Form>
			{/if}
		</div>
		{#if done === 'verify'}<Alert tone="success" live>{t('acc.settings.resent')}</Alert>{/if}
		<Form action="?/email" rules={forms.email} {form}>
			<Field label={t('acc.settings.newEmail')} name="email" description={t('acc.settings.newEmailHelp')}>
				<Input type="email" autocomplete="email" spellcheck="false" />
			</Field>
			<div><Button type="submit" size="sm">{t('acc.settings.changeEmail')}</Button></div>
		</Form>
		{#if done === 'email'}<Alert tone="success" live>{t('acc.settings.changeSent')}</Alert>{/if}
	</Card>

	<Card title={t('acc.settings.password')} level={2}>
		<Form action="?/password" rules={forms.password} {form}>
			{#if data.user.hasPassword}
				<Field label={t('acc.settings.current')} name="current"><PasswordInput autocomplete="current-password" /></Field>
			{/if}
			<Field label={t('acc.settings.newPassword')} name="password" description={t('acc.settings.newPasswordHelp')}>
				<PasswordInput autocomplete="new-password" />
			</Field>
			<div><Button type="submit" size="sm">{data.user.hasPassword ? t('acc.settings.changePassword') : t('acc.settings.setPassword')}</Button></div>
		</Form>
		{#if done === 'password'}<Alert tone="success" live>{t('acc.settings.passwordChanged')}</Alert>{/if}
	</Card>
	{/if}

	<Card title={t('acc.settings.deleteTitle')} level={2}>
		<p>{data.teams ? t('acc.settings.deleteBody') : t('acc.settings.deleteBodyMail')}</p>
		<form class="delete" method="POST" action="?/delete" bind:this={deleteForm}>
			<input type="hidden" name="form" value="delete" />
			<Button type="button" variant="danger" size="sm" onclick={() => (deleteOpen = true)}>{t('acc.settings.deleteButton')}</Button>
		</form>
		{#if deleteError}<Alert tone="danger" live>{deleteError}</Alert>{/if}
	</Card>
</div>

<AlertDialog
	bind:open={deleteOpen}
	title={t('acc.settings.deleteConfirm')}
	description={t('acc.settings.deleteUndone')}
	confirmLabel={t('acc.settings.deleteButton')}
	danger
	onconfirm={() => deleteForm?.requestSubmit()}
/>

<style>
	.sections {
		display: grid;
		gap: 1rem;
	}
	.sections :global(.tern-form) {
		max-width: 26rem;
		margin-block: 0.75rem;
	}
	.email {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.5rem;
	}
	.email :global(.tern-form) {
		margin: 0;
		margin-inline-start: 0.25rem;
	}
	p {
		margin: 0;
	}
	.delete {
		margin-top: 0.75rem;
	}
</style>
