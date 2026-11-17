<script lang="ts">
	import { i18n, t, type MessageKey } from '@ternkit/tern';
	import Icon from './Icon.svelte';
	import type { MailStore } from './store.svelte';
	import type { CalEvent } from './types';
	import { locale } from './util';

	let { store }: { store: MailStore } = $props();

	const HOUR = 48; // px per hour in the week and day views
	type View = 'month' | 'week' | 'day' | 'agenda';
	let view = $state<View>('week');
	let cursor = $state(startOfDay(new Date()));
	let events = $state<CalEvent[]>([]);
	let loading = $state(false);
	let failed = $state('');
	let editing = $state<Edit | null>(null);
	let saving = $state(false);
	let grid = $state<HTMLElement>();

	type Edit = {
		id: string;
		calendar: string;
		title: string;
		date: string;
		start: string;
		endDate: string;
		end: string;
		allDay: boolean;
		location: string;
		description: string;
		repeat: string;
		organizer?: CalEvent['organizer'];
		attendees?: CalEvent['attendees'];
	};

	// Monday first in most of the world, Sunday in the US.
	const weekStart = $derived(i18n.locale === 'en' && !/GB|IE|AU|NZ/.test(navigator.language) ? 0 : 1);
	const fmt = (o: Intl.DateTimeFormatOptions, d: Date) => new Intl.DateTimeFormat(locale(), o).format(d);
	const time = (d: Date) => fmt({ hour: 'numeric', minute: '2-digit' }, d);

	function startOfDay(d: Date) {
		const x = new Date(d);
		x.setHours(0, 0, 0, 0);
		return x;
	}
	function addDays(d: Date, n: number) {
		const x = new Date(d);
		x.setDate(x.getDate() + n);
		return x;
	}
	function startOfWeek(d: Date) {
		const x = startOfDay(d);
		return addDays(x, -((x.getDay() - weekStart + 7) % 7));
	}
	const sameDay = (a: Date, b: Date) => a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate();
	const ymd = (d: Date) => `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
	const hm = (d: Date) => `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`;

	// The days the view shows.
	const range = $derived.by(() => {
		if (view === 'day') return { from: cursor, to: addDays(cursor, 1) };
		if (view === 'week') return { from: startOfWeek(cursor), to: addDays(startOfWeek(cursor), 7) };
		if (view === 'agenda') return { from: cursor, to: addDays(cursor, 30) };
		const first = new Date(cursor.getFullYear(), cursor.getMonth(), 1);
		const from = startOfWeek(first);
		return { from, to: addDays(from, 42) };
	});
	const days = $derived.by(() => {
		const out: Date[] = [];
		for (let d = range.from; d < range.to; d = addDays(d, 1)) out.push(d);
		return out;
	});
	const title = $derived.by(() => {
		if (view === 'month') return fmt({ month: 'long', year: 'numeric' }, cursor);
		if (view === 'day') return fmt({ weekday: 'long', day: 'numeric', month: 'long', year: 'numeric' }, cursor);
		const last = addDays(range.to, -1);
		return `${fmt({ day: 'numeric', month: 'short' }, range.from)} – ${fmt({ day: 'numeric', month: 'short', year: 'numeric' }, last)}`;
	});

	const shown = $derived(events.filter((e) => !store.hiddenCalendars.has(e.calendar)));
	const on = (d: Date) => shown.filter((e) => new Date(e.start) < addDays(d, 1) && new Date(e.end) > d && !(e.allDay && sameDay(new Date(e.end), d) && new Date(e.end).getHours() === 0 && !sameDay(new Date(e.start), d)));
	const timed = (d: Date) => on(d).filter((e) => !e.allDay && new Date(e.end).getTime() - new Date(e.start).getTime() < 24 * 3600e3);
	const allDayOn = (d: Date) => on(d).filter((e) => e.allDay || new Date(e.end).getTime() - new Date(e.start).getTime() >= 24 * 3600e3);

	// Side by side when events overlap: each gets a column of its group.
	function layoutDay(d: Date) {
		const list = timed(d).sort((a, b) => +new Date(a.start) - +new Date(b.start));
		const placed: { e: CalEvent; col: number; cols: number; top: number; height: number }[] = [];
		let group: typeof placed = [];
		let groupEnd = 0;
		const close = () => {
			const cols = Math.max(1, ...group.map((g) => g.col + 1));
			for (const g of group) g.cols = cols;
			group = [];
		};
		for (const e of list) {
			const s = Math.max(+new Date(e.start), +d);
			const en = Math.min(+new Date(e.end), +addDays(d, 1));
			if (s >= groupEnd) close();
			const used = new Set(group.filter((g) => +new Date(g.e.end) > s).map((g) => g.col));
			let col = 0;
			while (used.has(col)) col++;
			const p = { e, col, cols: 1, top: ((s - +d) / 3600e3) * HOUR, height: Math.max(18, ((en - s) / 3600e3) * HOUR) };
			placed.push(p);
			group.push(p);
			groupEnd = Math.max(groupEnd, en);
		}
		close();
		return placed;
	}

	let seq = 0;
	async function load() {
		const n = ++seq;
		loading = true;
		failed = '';
		try {
			if (!store.calendars.length) await store.loadCalendars();
			const list = await store.loadEvents(range.from, range.to);
			if (n === seq) events = list;
		} catch (e) {
			if (n === seq) failed = (e as Error).message;
		} finally {
			if (n === seq) loading = false;
		}
	}
	$effect(() => {
		void range.from;
		void range.to;
		load();
	});
	// Start the week and day views at 7:00, or at the first event.
	$effect(() => {
		if ((view === 'week' || view === 'day') && grid) grid.scrollTop = 7 * HOUR - 8;
	});

	function step(n: number) {
		if (view === 'month') cursor = new Date(cursor.getFullYear(), cursor.getMonth() + n, 1);
		else cursor = addDays(cursor, n * (view === 'week' ? 7 : view === 'agenda' ? 30 : 1));
	}

	function create(start: Date, allDay = false) {
		const cal = store.calendars.find((c) => !store.hiddenCalendars.has(c.id)) ?? store.calendars[0];
		if (!cal) return store.notify(t('cal.noCalendars'), undefined, true);
		const end = allDay ? start : new Date(+start + 3600e3);
		editing = { id: '', calendar: cal.id, title: '', date: ymd(start), start: hm(start), endDate: ymd(end), end: hm(end), allDay, location: '', description: '', repeat: '' };
	}

	function open(e: CalEvent) {
		const s = new Date(e.start);
		// All-day events end at the start of the day after.
		const en = e.allDay ? addDays(new Date(e.end), -1) : new Date(e.end);
		editing = { id: e.id, calendar: e.calendar, title: e.title, date: ymd(s), start: hm(s), endDate: ymd(en), end: hm(en), allDay: e.allDay, location: e.location ?? '', description: e.description ?? '', repeat: e.repeat ?? '', organizer: e.organizer, attendees: e.attendees };
	}

	async function save() {
		if (!editing) return;
		const ed = editing;
		const start = ed.allDay ? new Date(`${ed.date}T00:00`) : new Date(`${ed.date}T${ed.start}`);
		let end = ed.allDay ? addDays(new Date(`${ed.endDate}T00:00`), 1) : new Date(`${ed.endDate}T${ed.end}`);
		if (end <= start) end = ed.allDay ? addDays(start, 1) : new Date(+start + 3600e3);
		saving = true;
		const ok = await store.saveEvent({ id: ed.id, calendar: ed.calendar, title: ed.title, start: start.toISOString(), end: end.toISOString(), allDay: ed.allDay, location: ed.location, description: ed.description, repeat: ed.repeat });
		saving = false;
		if (ok) {
			editing = null;
			load();
		}
	}

	async function remove() {
		if (!editing?.id || !confirm(editing.repeat ? t('cal.deleteSeries') : t('cal.deleteConfirm'))) return;
		if (await store.deleteEvent(editing.id)) {
			editing = null;
			load();
		}
	}

	const focus = (el: HTMLInputElement) => void requestAnimationFrame(() => el.focus());
	const calOf = (id: string) => store.calendars.find((c) => c.id === id);
	const today = startOfDay(new Date());
	const hours = Array.from({ length: 24 }, (_, h) => h);
	const hourLabel = (h: number) => fmt({ hour: 'numeric' }, new Date(2000, 0, 1, h));
</script>

<section class="pane cal" aria-label={t('cal.title')}>
	<div class="cal-head">
		<button class="btn sm" onclick={() => (cursor = startOfDay(new Date()))}>{t('cal.today')}</button>
		<button class="tb" onclick={() => step(-1)} title={t('cal.prev')}><Icon name="back" /></button>
		<button class="tb flip" onclick={() => step(1)} title={t('cal.next')}><Icon name="back" /></button>
		<h2>{title}</h2>
		{#if loading}<span class="spinner"></span>{/if}
		<span class="spacer"></span>
		<div class="seg" role="radiogroup" aria-label={t('cal.view')}>
			{#each ['day', 'week', 'month', 'agenda'] as const as v (v)}
				<button class:on={view === v} role="radio" aria-checked={view === v} onclick={() => (view = v)}>{t(`cal.${v}` as MessageKey)}</button>
			{/each}
		</div>
		<button class="btn primary sm" onclick={() => create(new Date(Math.max(+cursor, +today) + 10 * 3600e3))}><Icon name="plus" size={16} />{t('cal.new')}</button>
	</div>

	{#if failed}<div class="list-error"><Icon name="alert" size={16} />{failed}</div>{/if}
	{#if !store.calendars.length && !loading}
		<div class="cal-empty"><Icon name="clock" size={28} /><b>{t('cal.none')}</b><span>{t('cal.noneSub')}</span></div>
	{:else if view === 'month'}
		<div class="month">
			{#each days.slice(0, 7) as d (+d)}<div class="dow">{fmt({ weekday: 'short' }, d)}</div>{/each}
			{#each days as d (+d)}
				{@const list = on(d)}
				<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
				<div class="mday" class:other={d.getMonth() !== cursor.getMonth()} class:today={sameDay(d, today)} ondblclick={() => create(new Date(+d + 9 * 3600e3))}>
					<button class="dnum" onclick={() => ((cursor = d), (view = 'day'))}>{d.getDate()}</button>
					{#each list.slice(0, 3) as e (e.id + e.start)}
						<button class="chip-ev" class:solid={e.allDay} style:--c={e.color} onclick={() => open(e)} title={e.title}>
							{#if !e.allDay}<i>{time(new Date(e.start))}</i>{/if}{e.title}
						</button>
					{/each}
					{#if list.length > 3}<button class="more" onclick={() => ((cursor = d), (view = 'day'))}>{t('cal.more', { n: list.length - 3 })}</button>{/if}
				</div>
			{/each}
		</div>
	{:else if view === 'agenda'}
		<div class="agenda">
			{#each days as d (+d)}
				{@const list = on(d)}
				{#if list.length}
					<div class="aday" class:today={sameDay(d, today)}>
						<div class="adate"><b>{d.getDate()}</b><span>{fmt({ weekday: 'short', month: 'short' }, d)}</span></div>
						<div class="alist">
							{#each list as e (e.id + e.start)}
								<button class="aev" style:--c={e.color} onclick={() => open(e)}>
									<span class="dot"></span>
									<span class="atime">{e.allDay ? t('cal.allDay') : `${time(new Date(e.start))} – ${time(new Date(e.end))}`}</span>
									<b>{e.title}</b>{#if e.location}<span class="aloc">{e.location}</span>{/if}
								</button>
							{/each}
						</div>
					</div>
				{/if}
			{:else}
				<div></div>
			{/each}
			{#if !shown.length && !loading}<div class="cal-empty"><b>{t('cal.nothing')}</b></div>{/if}
		</div>
	{:else}
		<div class="tgrid-head" style:--n={days.length}>
			<div class="gutter"></div>
			{#each days as d (+d)}
				<button class="th" class:today={sameDay(d, today)} onclick={() => ((cursor = d), (view = 'day'))}><span>{fmt({ weekday: 'short' }, d)}</span><b>{d.getDate()}</b></button>
			{/each}
			<div class="gutter allday-label">{t('cal.allDay')}</div>
			{#each days as d (+d)}
				<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
				<div class="allday" ondblclick={() => create(d, true)}>
					{#each allDayOn(d) as e (e.id + e.start)}<button class="chip-ev solid" style:--c={e.color} onclick={() => open(e)} title={e.title}>{e.title}</button>{/each}
				</div>
			{/each}
		</div>
		<div class="tgrid" bind:this={grid} style:--n={days.length} style:--hour="{HOUR}px">
			<div class="gutter">
				{#each hours as h (h)}<div class="hl">{h ? hourLabel(h) : ''}</div>{/each}
			</div>
			{#each days as d (+d)}
				<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
				<div class="col" class:today={sameDay(d, today)} onclick={(e) => { if (e.target === e.currentTarget) { const y = e.offsetY; create(new Date(+d + Math.floor(y / (HOUR / 2)) * 1800e3)); } }}>
					{#each layoutDay(d) as p (p.e.id + p.e.start)}
						<button
							class="ev"
							style:--c={p.e.color}
							style:top="{p.top}px"
							style:height="{p.height}px"
							style:left="calc({(p.col / p.cols) * 100}% + 2px)"
							style:width="calc({100 / p.cols}% - 4px)"
							onclick={() => open(p.e)}
						>
							<b>{p.e.title}</b><span>{time(new Date(p.e.start))}{p.e.location ? ' · ' + p.e.location : ''}</span>
						</button>
					{/each}
					{#if sameDay(d, today)}<div class="now" style:top="{((Date.now() - +d) / 3600e3) * HOUR}px"></div>{/if}
				</div>
			{/each}
		</div>
	{/if}
</section>

{#if editing}
	<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
	<div class="ev-scrim" onclick={() => (editing = null)}></div>
	<div class="ev-dialog" role="dialog" tabindex="-1" aria-modal="true" aria-label={editing.id ? t('cal.edit') : t('cal.new')} onkeydown={(e) => e.key === 'Escape' && (editing = null)}>
		<input class="ev-title" bind:value={editing.title} placeholder={t('cal.titlePlaceholder')} use:focus onkeydown={(e) => e.key === 'Enter' && save()} />
		<div class="ev-row">
			<label class="check"><input type="checkbox" bind:checked={editing.allDay} />{t('cal.allDay')}</label>
			<select bind:value={editing.repeat} aria-label={t('cal.repeat')}>
				<option value="">{t('cal.once')}</option>
				{#each ['daily', 'weekly', 'monthly', 'yearly'] as r (r)}<option value={r}>{t(`cal.r.${r}` as MessageKey)}</option>{/each}
				{#if editing.repeat === 'custom'}<option value="custom">{t('cal.r.custom')}</option>{/if}
			</select>
		</div>
		<div class="ev-row">
			<input type="date" bind:value={editing.date} onchange={() => editing && editing.endDate < editing.date && (editing.endDate = editing.date)} aria-label={t('cal.starts')} />
			{#if !editing.allDay}<input type="time" bind:value={editing.start} aria-label={t('cal.startTime')} />{/if}
			<span>–</span>
			<input type="date" bind:value={editing.endDate} min={editing.date} aria-label={t('cal.ends')} />
			{#if !editing.allDay}<input type="time" bind:value={editing.end} aria-label={t('cal.endTime')} />{/if}
		</div>
		{#if editing.repeat && editing.id}<p class="ev-note">{t('cal.seriesNote')}</p>{/if}
		<input bind:value={editing.location} placeholder={t('cal.location')} />
		<textarea bind:value={editing.description} rows="3" placeholder={t('cal.description')}></textarea>
		<select bind:value={editing.calendar} disabled={!!editing.id} aria-label={t('cal.calendar')}>
			{#each store.calendars as c (c.id)}<option value={c.id}>{c.name} · {c.email}</option>{/each}
		</select>
		{#if editing.organizer || editing.attendees?.length}
			<div class="people">
				{#if editing.organizer}<div><b>{t('cal.organizer')}</b> {editing.organizer.name || editing.organizer.email}</div>{/if}
				{#each editing.attendees ?? [] as a (a.email)}<div>{a.name || a.email} <span>{a.status ? t(`cal.ps.${a.status}` as MessageKey) : ''}</span></div>{/each}
			</div>
		{/if}
		<div class="ev-foot">
			{#if editing.id}<button class="btn" onclick={remove}><Icon name="trash" size={16} />{t('cal.delete')}</button>{/if}
			<span class="spacer"></span>
			<button class="btn" onclick={() => (editing = null)}>{t('acc.box.cancel')}</button>
			<button class="btn primary" onclick={save} disabled={saving}>{t('acc.settings.save')}</button>
		</div>
		{#if calOf(editing.calendar)}<span class="ev-cal" style:--c={calOf(editing.calendar)?.color}></span>{/if}
	</div>
{/if}

<style>
	.cal { display: flex; flex-direction: column; min-height: 0; overflow: hidden; }
	.cal-head { display: flex; align-items: center; gap: 6px; padding: 10px 12px; border-bottom: 1px solid var(--border); flex-wrap: wrap; }
	.cal-head h2 { font-size: 17px; margin: 0 6px; font-weight: 650; white-space: nowrap; }
	.flip { transform: scaleX(-1); }
	:global([dir='rtl']) .cal-head .tb:not(.flip) { transform: scaleX(-1); }
	:global([dir='rtl']) .cal-head .flip { transform: none; }
	.spacer { flex: 1; }
	.seg { display: flex; background: var(--surface-2); border-radius: 10px; padding: 2px; }
	.seg button { border: 0; background: none; color: var(--text-2); font: inherit; font-size: 13px; padding: 5px 10px; border-radius: 8px; cursor: pointer; }
	.seg button.on { background: var(--surface); color: var(--text); font-weight: 600; box-shadow: 0 1px 2px rgb(0 0 0 / 0.1); }
	.cal-empty { display: grid; justify-items: center; gap: 6px; padding: 60px 20px; color: var(--text-3); text-align: center; }
	.cal-empty b { color: var(--text); }

	.month { display: grid; grid-template-columns: repeat(7, minmax(0, 1fr)); grid-auto-rows: minmax(0, 1fr); grid-template-rows: auto; flex: 1; min-height: 0; overflow: auto; }
	.dow { padding: 6px 8px; font-size: 11.5px; text-transform: uppercase; letter-spacing: 0.05em; color: var(--text-3); border-bottom: 1px solid var(--border); }
	.mday { min-height: 96px; border-inline-end: 1px solid var(--border); border-bottom: 1px solid var(--border); padding: 4px; display: flex; flex-direction: column; gap: 2px; min-width: 0; }
	.mday.other { background: color-mix(in oklab, var(--surface-2) 55%, transparent); }
	.mday.other .dnum { color: var(--text-3); }
	.dnum { align-self: flex-start; border: 0; background: none; font: inherit; font-size: 12.5px; font-weight: 600; color: var(--text-2); width: 24px; height: 24px; border-radius: 50%; cursor: pointer; }
	.mday.today .dnum, .th.today b { background: var(--accent); color: var(--accent-fg); }
	.chip-ev { display: block; text-align: start; border: 0; background: none; font: inherit; font-size: 12px; color: var(--text); padding: 1px 6px; border-radius: 5px; cursor: pointer; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; border-inline-start: 3px solid var(--c); min-width: 0; }
	.chip-ev.solid { background: color-mix(in oklab, var(--c) 22%, transparent); border-inline-start-color: var(--c); }
	.chip-ev:hover { background: color-mix(in oklab, var(--c) 16%, transparent); }
	.chip-ev i { font-style: normal; color: var(--text-3); margin-inline-end: 4px; font-size: 11px; }
	.more { border: 0; background: none; font: inherit; font-size: 11.5px; color: var(--text-3); text-align: start; padding: 0 6px; cursor: pointer; }

	.tgrid-head { display: grid; grid-template-columns: 56px repeat(var(--n), minmax(0, 1fr)); border-bottom: 1px solid var(--border); }
	.th { border: 0; background: none; font: inherit; display: grid; justify-items: center; padding: 6px 0 4px; color: var(--text-2); cursor: pointer; }
	.th span { font-size: 11.5px; text-transform: uppercase; letter-spacing: 0.05em; color: var(--text-3); }
	.th b { font-size: 18px; width: 32px; height: 32px; display: grid; place-items: center; border-radius: 50%; }
	.allday-label { font-size: 10.5px; color: var(--text-3); padding: 4px; text-align: end; }
	.allday { border-inline-start: 1px solid var(--border); padding: 2px; display: grid; gap: 2px; min-height: 26px; align-content: start; min-width: 0; }
	.tgrid { display: grid; grid-template-columns: 56px repeat(var(--n), minmax(0, 1fr)); flex: 1; min-height: 0; overflow-y: auto; position: relative; }
	.gutter .hl { height: var(--hour); font-size: 10.5px; color: var(--text-3); text-align: end; padding-inline-end: 6px; transform: translateY(-7px); }
	.col { position: relative; height: calc(24 * var(--hour)); border-inline-start: 1px solid var(--border); background-image: repeating-linear-gradient(to bottom, var(--border) 0 1px, transparent 1px var(--hour)); cursor: cell; }
	.col.today { background-color: color-mix(in oklab, var(--accent) 4%, transparent); }
	.ev { position: absolute; border: 0; border-radius: 6px; padding: 3px 6px; text-align: start; font: inherit; font-size: 12px; overflow: hidden; cursor: pointer; background: color-mix(in oklab, var(--c) 24%, var(--surface)); border-inline-start: 3px solid var(--c); color: var(--text); display: flex; flex-direction: column; line-height: 1.25; }
	.ev:hover { background: color-mix(in oklab, var(--c) 34%, var(--surface)); }
	.ev b { font-weight: 600; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.ev span { color: var(--text-2); font-size: 11px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
	.now { position: absolute; inset-inline: 0; height: 2px; background: var(--danger); z-index: 2; pointer-events: none; }
	.now::before { content: ''; position: absolute; inset-inline-start: -5px; top: -4px; width: 10px; height: 10px; border-radius: 50%; background: var(--danger); }

	.agenda { flex: 1; overflow-y: auto; padding: 8px 16px 40px; }
	.aday { display: grid; grid-template-columns: 72px 1fr; gap: 12px; padding: 10px 0; border-bottom: 1px solid var(--border); }
	.adate { display: grid; justify-items: start; }
	.adate b { font-size: 22px; line-height: 1; }
	.adate span { font-size: 12px; color: var(--text-3); text-transform: uppercase; }
	.aday.today .adate b { color: var(--accent-ink); }
	.alist { display: grid; gap: 2px; }
	.aev { display: flex; align-items: baseline; gap: 10px; border: 0; background: none; font: inherit; text-align: start; padding: 5px 8px; border-radius: 8px; cursor: pointer; color: var(--text); }
	.aev:hover { background: var(--surface-2); }
	.aev .dot { width: 9px; height: 9px; border-radius: 50%; background: var(--c); flex: none; align-self: center; }
	.atime { color: var(--text-2); font-size: 13px; min-width: 110px; }
	.aloc { color: var(--text-3); font-size: 13px; }

	.ev-scrim { position: fixed; inset: 0; background: rgb(0 0 0 / 0.25); z-index: 70; }
	.ev-dialog { position: fixed; inset-inline-start: 50%; top: 12vh; transform: translateX(-50%); width: min(440px, 94vw); background: var(--surface); border: 1px solid var(--border); border-radius: 16px; box-shadow: 0 20px 60px rgb(0 0 0 / 0.3); padding: 18px; display: grid; gap: 10px; z-index: 71; }
	:global([dir='rtl']) .ev-dialog { transform: translateX(50%); }
	.ev-dialog input:not([type='checkbox']), .ev-dialog select, .ev-dialog textarea { font: inherit; font-size: 14px; height: 36px; padding-inline: 10px; border-radius: 9px; border: 1px solid var(--border); background: var(--surface-2); color: var(--text); min-width: 0; }
	.ev-dialog textarea { height: auto; padding-block: 8px; resize: vertical; }
	.ev-title { font-size: 18px !important; font-weight: 600; height: 42px !important; background: transparent !important; border-color: transparent !important; border-bottom: 2px solid var(--accent) !important; border-radius: 0 !important; padding-inline: 2px !important; }
	.ev-row { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; }
	.ev-row input[type='date'] { flex: 1 0 8.5rem; }
	.ev-row input[type='time'] { flex: 0 0 6.5rem; }
	.check { display: flex; align-items: center; gap: 6px; font-size: 14px; margin-inline-end: auto; }
	.ev-note { margin: -2px 0 0; font-size: 12.5px; color: var(--text-3); }
	.people { font-size: 13px; display: grid; gap: 2px; color: var(--text-2); }
	.people span { color: var(--text-3); }
	.ev-foot { display: flex; gap: 8px; align-items: center; margin-top: 4px; }
	.ev-cal { position: absolute; top: 18px; inset-inline-end: 18px; width: 10px; height: 10px; border-radius: 50%; background: var(--c); }
	@media (max-width: 720px) {
		.cal-head h2 { font-size: 15px; width: 100%; order: 10; margin: 4px 0 0; }
		.mday { min-height: 64px; }
		.chip-ev i { display: none; }
	}
</style>
