<script lang="ts">
	import { SignIn } from '@ternkit/tern/ui';
	import type { PageProps } from './$types';
	import { forms } from './$forms';

	let { data, form }: PageProps = $props();
</script>

<svelte:head><title>Sign in · Petrel</title></svelte:head>

<SignIn
	rules={forms.default}
	{form}
	next={data.next}
	providers={data.providers}
	error={data.error || null}
	passkey
	remember
	forgotHref="/forgot-password"
	signUpHref={data.next === '/mail' ? '/signup' : `/signup?next=${encodeURIComponent(data.next)}`}
/>
