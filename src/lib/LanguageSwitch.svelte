<script lang="ts">
	import { i18n, setLocale, t } from '@ternkit/tern';

	const names: Record<string, string> = { en: 'English', de: 'Deutsch' };
</script>

<label class="lang">
	<span class="sr">{t('nav.language')}</span>
	<select value={i18n.locale} onchange={(e) => setLocale(e.currentTarget.value)}>
		{#each i18n.locales as l (l)}<option value={l}>{names[l] ?? l}</option>{/each}
	</select>
</label>

<style>
	.sr {
		position: absolute;
		width: 1px;
		height: 1px;
		overflow: hidden;
		clip: rect(0 0 0 0);
	}
	select {
		font: inherit;
		font-size: 0.875rem;
		color: var(--tern-fg);
		background: var(--tern-surface);
		border: 1px solid var(--tern-border);
		border-radius: var(--tern-radius-sm, 8px);
		height: 2rem;
		padding-inline: 0.5rem;
		cursor: pointer;
	}
</style>
