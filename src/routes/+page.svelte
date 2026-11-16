<script lang="ts">
	import { t } from '@ternkit/tern';
	import { Button, Card } from '@ternkit/tern/ui';
	import Logo from '$lib/Logo.svelte';
	import LanguageSwitch from '$lib/LanguageSwitch.svelte';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	const features = ['f1', 'f2', 'f3', 'f4'] as const;
	const repo = 'https://github.com/Unfield/petrel';
</script>

<svelte:head>
	<title>Petrel · {t('home.tagline')}</title>
	<meta name="description" content={t('home.lead')} />
</svelte:head>

<header class="top">
	<a class="brand" href="/"><Logo size={30} icon />Petrel</a>
	<nav aria-label="Account">
		<LanguageSwitch />
		{#if data.user}
			<Button href="/mail" size="sm">{t('home.openMail')}</Button>
		{:else}
			<Button href="/login" variant="ghost" size="sm">{t('home.signIn')}</Button>
			<Button href="/signup" size="sm">{t('home.getStarted')}</Button>
		{/if}
	</nav>
</header>

<main>
	<section class="hero">
		<div class="mark"><Logo size={72} /></div>
		<h1>{t('home.tagline')}</h1>
		<p>{t('home.lead')}</p>
		<div class="cta">
			{#if data.user}
				<Button href="/mail" size="lg">{t('home.openMail')}</Button>
			{:else}
				<Button href="/signup" size="lg">{t('home.getStarted')}</Button>
			{/if}
			<Button href={repo} variant="secondary" size="lg">{t('home.source')}</Button>
		</div>
	</section>

	<section class="features" aria-label="Features">
		{#each features as f (f)}
			<Card title={t(`home.${f}Title`)} level={2}><p>{t(`home.${f}`)}</p></Card>
		{/each}
	</section>
</main>

<footer>
	<p>{t('home.footer', { tern: 'Tern' })} · <a href={repo}>GitHub</a></p>
</footer>

<style>
	.top {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 1rem;
		max-width: 64rem;
		margin: 0 auto;
		padding: 1rem;
	}
	.brand {
		display: inline-flex;
		align-items: center;
		gap: 0.625rem;
		font-weight: 700;
		font-size: 1.1875rem;
		letter-spacing: -0.02em;
		color: var(--tern-fg);
		text-decoration: none;
	}
	nav {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}
	main {
		max-width: 64rem;
		margin: 0 auto;
		padding: 0 1rem;
	}
	.hero {
		padding: 4.5rem 0 3.5rem;
		max-width: 42rem;
	}
	.mark {
		margin-bottom: 1.25rem;
	}
	h1 {
		margin: 0 0 1rem;
		font-size: clamp(2.25rem, 5.5vw, 3.5rem);
		line-height: 1.05;
		letter-spacing: -0.035em;
		text-wrap: balance;
	}
	.hero p {
		margin: 0;
		color: var(--tern-muted);
		font-size: 1.1875rem;
		line-height: 1.6;
	}
	.cta {
		display: flex;
		flex-wrap: wrap;
		gap: 0.75rem;
		margin-top: 2rem;
	}
	.features {
		display: grid;
		grid-template-columns: repeat(auto-fit, minmax(min(14rem, 100%), 1fr));
		gap: 1rem;
		padding-bottom: 4rem;
	}
	.features p {
		margin: 0;
		color: var(--tern-muted);
		line-height: 1.55;
	}
	footer {
		max-width: 64rem;
		margin: 0 auto;
		padding: 2rem 1rem;
		border-top: 1px solid var(--tern-border);
		color: var(--tern-muted);
		font-size: 0.875rem;
	}
	footer a {
		color: inherit;
	}
</style>
