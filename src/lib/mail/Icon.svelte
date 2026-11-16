<script lang="ts">
	import { FLIP, ICONS } from './icons';

	let { name, size = 18, filled = false }: { name: string; size?: number; filled?: boolean } = $props();
</script>

<svg
	class="ic"
	class:flip={FLIP.has(name)}
	class:filled
	width={size}
	height={size}
	viewBox="0 0 24 24"
	fill="none"
	stroke="currentColor"
	stroke-width="1.8"
	stroke-linecap="round"
	stroke-linejoin="round"
	aria-hidden="true">{@html ICONS[name] ?? ''}</svg
>
