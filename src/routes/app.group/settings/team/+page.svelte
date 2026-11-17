<script lang="ts">
	import { t } from '@ternkit/tern';
	import { ActivityLog, Alert, Card, TeamInvitations, TeamMembers, TeamSettings } from '@ternkit/tern/ui';
	import type { PageProps } from './$types';

	let { data, form }: PageProps = $props();
	const done = $derived((form as { done?: string } | null)?.done);
	const manages = $derived(data.team.role === 'owner' || data.team.role === 'admin');
</script>

<svelte:head><title>{t('acc.team.title')} · Petrel</title></svelte:head>

<h1>{data.team.team.name}</h1>
<p class="lead">{t('acc.team.lead')}</p>

<div class="sections">
	{#if manages}
		<Card>
			<TeamSettings team={data.team.team} role={data.team.role} renameAction="?/rename" deleteAction="?/delete" {form} level={2} />
			{#if done === 'renamed'}<Alert tone="success" live>{t('acc.settings.saved')}</Alert>{/if}
		</Card>
	{/if}

	<Card>
		<TeamMembers members={data.members} me={data.me} role={data.team.role} roleAction="?/role" removeAction="?/remove" leaveAction="?/leave" {form} level={2} />
	</Card>

	{#if manages}
		<Card>
			<TeamInvitations invitations={data.invitations} role={data.team.role} inviteAction="?/invite" cancelAction="?/cancel" {form} level={2} />
		</Card>

		<Card>
			<ActivityLog
				entries={data.activity}
				showActor
				title={t('acc.team.activity')}
				description={t('acc.team.activityBody')}
				moreHref={data.older ? `?before=${data.older}` : undefined}
				level={2}
			/>
		</Card>
	{/if}
</div>

<style>
	.sections {
		display: grid;
		gap: 1rem;
	}
</style>
