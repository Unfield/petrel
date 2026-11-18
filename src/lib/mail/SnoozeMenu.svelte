<script lang="ts">
	import { t } from '@ternkit/tern';
	import Icon from './Icon.svelte';
	import type { MailStore } from './store.svelte';
	import { locale } from './util';

	let { store, items, onclose }: { store: MailStore; items: string[]; onclose: () => void } = $props();

	function at(days: number, hour: number) {
		const d = new Date();
		d.setDate(d.getDate() + days);
		d.setHours(hour, 0, 0, 0);
		return d;
	}
	const now = new Date();
	const later = new Date(now.getTime() + 3 * 3600_000);
	later.setMinutes(0, 0, 0);
	const saturday = at((6 - now.getDay() + 7) % 7 || 7, 9);
	const monday = at((8 - now.getDay()) % 7 || 7, 8);
	const options = [
		{ label: t('m.a.later'), when: later, show: later.getDate() === now.getDate() },
		{ label: t('m.a.tomorrow'), when: at(1, 8), show: true },
		{ label: t('m.a.weekend'), when: saturday, show: now.getDay() < 5 && now.getDay() > 0 },
		{ label: t('m.a.nextWeek'), when: monday, show: true }
	].filter((o) => o.show);
	const fmt = (d: Date) => d.toLocaleString(locale(), { weekday: 'short', hour: 'numeric', minute: '2-digit' });

	let custom = $state('');
	function go(d: Date) {
		onclose();
		store.snooze(items, d);
	}
</script>

<div class="pop snooze" role="menu">
	<div class="pop-title">{t('m.a.snooze')}</div>
	{#each options as o (o.label)}
		<button class="lbl" role="menuitem" onclick={() => go(o.when)}><Icon name="clock" size={16} />{o.label}<span class="when">{fmt(o.when)}</span></button>
	{/each}
	<div class="pop-title">{t('m.a.pick')}</div>
	<input type="datetime-local" bind:value={custom} min={new Date(Date.now() - new Date().getTimezoneOffset() * 60000).toISOString().slice(0, 16)} onchange={() => custom && go(new Date(custom))} />
</div>
