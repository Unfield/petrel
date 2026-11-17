<script lang="ts">
	import { t } from '@ternkit/tern';
	import { AcceptInvitation } from '@ternkit/tern/ui';
	import type { PageProps } from './$types';

	let { data, form }: PageProps = $props();
	const here = $derived(`/invitation?token=${encodeURIComponent(data.token)}`);
	const back = $derived(encodeURIComponent(here));
</script>

<svelte:head><title>{t('acc.invitation.title')} · Petrel</title></svelte:head>

<AcceptInvitation
	invitation={data.invitation}
	token={data.token}
	email={data.email}
	{form}
	signInHref="/login?next={back}"
	signUpHref="/signup?next={back}"
	signOutAction="/logout"
	signOutNext={here}
/>
