<script lang="ts">
	import { t, type MessageKey } from '@ternkit/tern';
	import Icon from './Icon.svelte';
	import type { MailStore } from './store.svelte';
	import { isMac } from './util';

	let { store }: { store: MailStore } = $props();
	const mod = isMac() ? '⌘' : 'Ctrl';
	const nav: [string, string[]][] = [['next', ['J']], ['prev', ['K']], ['open', ['Enter']], ['back', ['U']], ['search', ['/']], ['palette', [mod, 'K']], ['help', ['?']], ['close', ['Esc']]];
	const acts: [string, string[]][] = [['compose', ['C']], ['reply', ['R']], ['replyAll', ['A']], ['forward', ['F']], ['archive', ['E']], ['delete', ['#']], ['star', ['S']], ['select', ['X']], ['read', ['Shift', 'I']], ['unread', ['Shift', 'U']], ['send', [mod, 'Enter']]];
	const close = () => (store.ui.help = false);
</script>

<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
<div class="modal-wrap" onclick={(e) => e.target === e.currentTarget && close()}>
	<div class="help" role="dialog" aria-modal="true" aria-label={t('m.h.title')}>
		<div class="help-head"><h2>{t('m.h.title')}</h2><button class="icon-btn" onclick={close} aria-label="Close"><Icon name="x" /></button></div>
		<div class="help-grid">
			{#each [['nav', nav], ['actions', acts]] as const as [title, rows] (title)}
				<div>
					<h3>{t(`m.h.${title}` as MessageKey)}</h3>
					{#each rows as [k, keys] (k)}
						<div class="hk"><span>{t(`m.h.${k}` as MessageKey)}</span><span>{#each keys as key (key)}<span class="kbd">{key}</span>{/each}</span></div>
					{/each}
				</div>
			{/each}
		</div>
	</div>
</div>
