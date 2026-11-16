<script lang="ts">
	import { t } from '@ternkit/tern';
	import MailApp from '$lib/mail/MailApp.svelte';
	import type { Prefs } from '$lib/mail/types';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
</script>

<svelte:head><title>{t('m.inbox')} · Petrel</title></svelte:head>

<MailApp user={data.user} accounts={data.accounts} prefs={data.prefs as Prefs} />
