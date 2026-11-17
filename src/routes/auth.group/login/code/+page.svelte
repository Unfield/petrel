<script lang="ts">
	import { t } from '@ternkit/tern';
	import { TwoStepCode } from '@ternkit/tern/ui';
	import type { PageProps } from './$types';
	import { forms } from './$forms';

	let { data, form }: PageProps = $props();
</script>

<svelte:head><title>{t('acc.code.title')} · Petrel</title></svelte:head>

<TwoStepCode action="?next={encodeURIComponent(data.next)}" rules={forms.default} {form} signInHref="/login" />
