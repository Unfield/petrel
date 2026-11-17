<script lang="ts">
	import { t } from '@ternkit/tern';
	import { ResetPassword } from '@ternkit/tern/ui';
	import type { PageProps } from './$types';
	import { forms } from './$forms';

	let { data, form }: PageProps = $props();
</script>

<svelte:head><title>{t('acc.reset.title')} · Petrel</title></svelte:head>

<ResetPassword token={data.token} rules={forms.default} {form} forgotHref="/forgot-password" />
