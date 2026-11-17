<script lang="ts">
	import { t } from '@ternkit/tern';
	import { Button } from '@ternkit/tern/ui';
</script>

<svelte:head><title>{t('nav.signOut')} · Petrel</title></svelte:head>

<main class="center">
	<form method="POST"><Button type="submit">{t('nav.signOut')}</Button></form>
</main>

<style>
	.center {
		display: grid;
		place-items: center;
		min-height: 60vh;
	}
</style>
