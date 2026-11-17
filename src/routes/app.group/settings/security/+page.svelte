<script lang="ts">
	import { t } from '@ternkit/tern';
	import { ActivityLog, Card, Devices, LinkedAccounts, Passkeys, TwoStep } from '@ternkit/tern/ui';
	import type { PageProps } from './$types';

	let { data, form }: PageProps = $props();
	const recoveryCodes = $derived((form as { recoveryCodes?: string[] } | null)?.recoveryCodes);
</script>

<svelte:head><title>{t('acc.security.title')} · Petrel</title></svelte:head>

<h1>{t('acc.security.title')}</h1>
<p class="lead">{t('acc.security.lead')}</p>

<div class="sections">
	<Card>
		<TwoStep
			enabled={data.user.twoStep}
			setup={data.setup}
			{recoveryCodes}
			codesLeft={data.codesLeft}
			setupAction="?/twoStepSetup"
			confirmAction="?/twoStepConfirm"
			offAction="?/twoStepOff"
			codesAction="?/twoStepCodes"
			{form}
		/>
	</Card>
	<Card><Passkeys passkeys={data.passkeys} removeAction="?/removePasskey" /></Card>
	{#if data.providers.length || data.identities.length}
		<Card><LinkedAccounts identities={data.identities} providers={data.providers} next="/settings/security" unlinkAction="?/unlink" /></Card>
	{/if}
	<Card><Devices sessions={data.sessions} action="?/revoke" othersAction="?/others" /></Card>
	<Card><ActivityLog entries={data.activity} /></Card>
</div>

<style>
	.sections {
		display: grid;
		gap: 1rem;
	}
</style>
