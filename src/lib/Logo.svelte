<script lang="ts">
	// The Petrel mark ("Glide"). icon draws it as the app icon: white on the accent.
	let { size = 32, icon = false }: { size?: number; icon?: boolean } = $props();
</script>

<svg width={size} height={size} viewBox="0 0 64 64" aria-hidden="true" class="logo">
	{#if icon}
		<rect width="64" height="64" rx="17" fill="var(--tern-accent)" />
		<g transform="translate(9 4.6) scale(.72)">
			<path d="M6 31c8.5-7.5 17.5-7 26 3.5C40.5 24 49.5 23.5 58 31c-8.6-2.4-17 .8-26 10.8C23 31.8 14.6 28.6 6 31z" fill="var(--tern-accent-fg)" />
			<path d="M14 50c3-2.6 6-2.6 9 0s6 2.6 9 0 6-2.6 9 0 6 2.6 9 0" fill="none" stroke="var(--tern-accent-fg)" stroke-width="4" stroke-linecap="round" opacity=".6" />
		</g>
	{:else}
		<path d="M6 31c8.5-7.5 17.5-7 26 3.5C40.5 24 49.5 23.5 58 31c-8.6-2.4-17 .8-26 10.8C23 31.8 14.6 28.6 6 31z" fill="var(--tern-accent)" />
		<path d="M14 50c3-2.6 6-2.6 9 0s6 2.6 9 0 6-2.6 9 0 6 2.6 9 0" fill="none" stroke="var(--tern-accent)" stroke-width="3.6" stroke-linecap="round" opacity=".55" />
	{/if}
</svg>

<style>
	.logo {
		display: block;
		flex: none;
	}
</style>
