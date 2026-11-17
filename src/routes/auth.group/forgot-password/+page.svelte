<script lang="ts">
	import { t } from '@ternkit/tern';
	import { ForgotPassword } from '@ternkit/tern/ui';
	import type { PageProps } from './$types';
	import { forms } from './$forms';

	let { form }: PageProps = $props();
</script>

<svelte:head><title>{t('acc.forgot.title')} · Petrel</title></svelte:head>

<ForgotPassword rules={forms.default} {form} signInHref="/login" />
