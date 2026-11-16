<script lang="ts">
	import { i18n, setLocale, t } from '@ternkit/tern';
	import Icon from './Icon.svelte';
	import { DEFAULT_PREFS, type MailStore } from './store.svelte';
	import type { Prefs } from './types';

	let { store, dark }: { store: MailStore; dark: boolean } = $props();

	const PRESETS: Record<Prefs['preset'], { name: string; light: string[]; dark: string[] }> = {
		slate: { name: 'Slate', light: ['#eef1f5', '#ffffff', '#e2e6ed', '#7d8798'], dark: ['#0d1015', '#151920', '#252b36', '#747e8d'] },
		paper: { name: 'Paper', light: ['#f3efe7', '#fffdf8', '#e5ddcd', '#8f8574'], dark: ['#15120e', '#1d1a15', '#2f2a22', '#877d6b'] },
		fjord: { name: 'Fjord', light: ['#e9eff2', '#fbfdfe', '#d8e2e7', '#7c8f99'], dark: ['#0a101c', '#101828', '#1d2a44', '#6c7a92'] },
		contrast: { name: 'Contrast', light: ['#ffffff', '#ffffff', '#111111', '#333333'], dark: ['#000000', '#000000', '#f2f2f2', '#cfcfcf'] }
	};
	const ACCENTS = [['Blue', '#3b6cf6'], ['Violet', '#7c5cff'], ['Teal', '#0f9d8f'], ['Green', '#2f9e44'], ['Orange', '#e8590c'], ['Rose', '#e64980'], ['Graphite', '#4b5563']];
	const LANGS: Record<string, string> = { en: 'English', de: 'Deutsch' };
	const custom = $derived(!ACCENTS.some(([, c]) => c === store.prefs.accent.toLowerCase()));
	const set = <K extends keyof Prefs>(k: K, v: Prefs[K]) => store.setPref(k, v);
</script>

{#snippet seg(k: 'mode' | 'density', opts: [string, string, string?][])}
	<div class="seg" role="radiogroup">
		{#each opts as [v, label, icon] (v)}
			<button class:on={store.prefs[k] === v} role="radio" aria-checked={store.prefs[k] === v} onclick={() => set(k, v as never)}>{#if icon}<Icon name={icon} size={16} />{/if}{label}</button>
		{/each}
	</div>
{/snippet}

<aside class="drawer" class:open={store.ui.settings} aria-label={t('m.s.title')} aria-hidden={!store.ui.settings}>
	<div class="s-head"><h2>{t('m.s.title')}</h2><button class="icon-btn" onclick={() => (store.ui.settings = false)} aria-label="Close"><Icon name="x" /></button></div>
	<div class="s-body">
		<h3>{t('m.s.appearance')}</h3>
		<div class="s-row"><div class="s-label">{t('m.s.mode')}</div>{@render seg('mode', [['light', t('m.s.light'), 'sun'], ['dark', t('m.s.dark'), 'moon'], ['system', t('m.s.system'), 'monitor']])}</div>
		<div class="s-row">
			<div class="s-label">{t('m.s.theme')}</div>
			<div class="cards">
				{#each Object.entries(PRESETS) as [id, p] (id)}
					{@const [bg, s, b, tx] = dark ? p.dark : p.light}
					<button class="card" class:on={store.prefs.preset === id} onclick={() => set('preset', id as Prefs['preset'])}>
						<div class="tp" style:background={bg}>
							<i style:background={s} style:border="1px solid {b}"></i>
							<i class="pnl" style:background={s} style:border="1px solid {b}"
								><b style:background={store.prefs.accent} style:width="45%"></b><b style:background={tx} style:opacity=".5" style:width="85%"></b><b
									style:background={tx}
									style:opacity=".3"
									style:width="65%"
								></b></i
							>
						</div>
						<div class="cap">{id === 'contrast' ? t('m.s.contrast') : p.name}</div>
					</button>
				{/each}
			</div>
		</div>
		<div class="s-row">
			<div class="s-label">{t('m.s.accent')}</div>
			<div class="swatches">
				{#each ACCENTS as [n, c] (c)}
					<button class="sw" class:on={store.prefs.accent.toLowerCase() === c} style:--c={c} title={n} onclick={() => set('accent', c)}>{#if store.prefs.accent.toLowerCase() === c}<Icon name="check" size={15} />{/if}</button>
				{/each}
				<label class="sw custom" class:on={custom} style:--c={custom ? store.prefs.accent : '#888'} title={t('m.s.custom')}>
					<input type="color" value={store.prefs.accent} oninput={(e) => (store.prefs = { ...store.prefs, accent: e.currentTarget.value })} onchange={() => store.savePrefs()} />
				</label>
			</div>
		</div>

		<h3>{t('m.s.layout')}</h3>
		<div class="s-row">
			<div class="s-label">{t('m.s.pane')}</div>
			<div class="cards three">
				{#each ['right', 'below', 'off'] as const as v (v)}
					<button class="card" class:on={store.prefs.layout === v} onclick={() => set('layout', v)}>
						<div class="lp {v}"><i></i><i></i>{#if v !== 'off'}<i class="r"></i>{/if}</div>
						<div class="cap">{t(`m.s.${v}`)}</div>
					</button>
				{/each}
			</div>
		</div>
		<div class="s-row"><div class="s-label">{t('m.s.density')}</div>{@render seg('density', [['compact', t('m.s.compact')], ['comfortable', t('m.s.comfortable')], ['spacious', t('m.s.spacious')]])}</div>
		<div class="s-row">
			<button class="switch-row" onclick={() => set('threads', !store.prefs.threads)} role="switch" aria-checked={store.prefs.threads}>
				<div><div class="s-label" style="margin:0">{t('m.s.threads')}</div><div class="s-hint">{t('m.s.threadsSub')}</div></div>
				<span class="switch" class:on={store.prefs.threads}></span>
			</button>
		</div>

		<h3>{t('m.s.language')}</h3>
		<div class="s-row">
			<div class="seg" role="radiogroup">
				{#each i18n.locales as l (l)}
					<button class:on={i18n.locale === l} role="radio" aria-checked={i18n.locale === l} onclick={() => setLocale(l)}>{LANGS[l] ?? l}</button>
				{/each}
			</div>
		</div>
		<div class="s-row">
			<button class="switch-row" onclick={() => set('rtl', !store.prefs.rtl)} role="switch" aria-checked={store.prefs.rtl}>
				<div><div class="s-label" style="margin:0">{t('m.s.rtl')}</div><div class="s-hint">{t('m.s.rtlSub')}</div></div>
				<span class="switch" class:on={store.prefs.rtl}></span>
			</button>
		</div>
		<h3>{t('m.a.account')}</h3>
		<div class="s-links">
			<a class="btn" href="/settings/mailboxes"><Icon name="inbox" size={16} />{t('m.a.mailboxes')}</a>
			<a class="btn" href="/settings"><Icon name="user" size={16} />{t('m.a.account')}</a>
		</div>
	</div>
	<div class="s-foot">
		<span style="flex:1">{t('m.s.saved')}</span>
		<button class="btn" onclick={() => ((store.prefs = { ...DEFAULT_PREFS, images: store.prefs.images }), store.savePrefs())}>{t('m.s.reset')}</button>
	</div>
</aside>
