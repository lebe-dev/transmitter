<script lang="ts">
	import './fonts.css';
	import './layout.css';
	import favicon from '$lib/assets/favicon.svg';
	import { ModeWatcher } from 'mode-watcher';
	import { Toaster } from '$lib/components/ui/sonner/index.js';
	import * as Tooltip from '$lib/components/ui/tooltip/index.js';
	import { waitLocale } from 'svelte-intl-precompile';

	let { children } = $props();
</script>

<svelte:head><link rel="icon" href={favicon} /></svelte:head>

<ModeWatcher defaultMode="system" themeStorageKey="transmitter-color-theme" />
<Toaster richColors position="top-right" />
<Tooltip.Provider delayDuration={200}>
	{#await waitLocale() then}
		{@render children()}
	{/await}
</Tooltip.Provider>
