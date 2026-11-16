<script lang="ts">
	import { NewTeam } from '@ternkit/tern/ui';
	import type { PageProps } from './$types';

	let { data, form }: PageProps = $props();
</script>

<svelte:head><title>Create a team · Petrel</title></svelte:head>

<NewTeam {form} cancelHref={data.hasTeam ? '/mail' : undefined} />
