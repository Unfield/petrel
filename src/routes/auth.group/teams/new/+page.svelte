<script lang="ts">
	import { t } from '@ternkit/tern';
	import { NewTeam } from '@ternkit/tern/ui';
	import type { PageProps } from './$types';

	let { data, form }: PageProps = $props();
</script>

<svelte:head><title>{t('acc.newTeam.title')} · Petrel</title></svelte:head>

<NewTeam {form} cancelHref={data.hasTeam ? '/mail' : undefined} />
