<script lang="ts">
	import './fonts.css';
	import './layout.css';
	import { onMount } from 'svelte';
	import { get } from 'svelte/store';
	import favicon from '$lib/assets/favicon.svg';
	import { ModeWatcher, setTheme } from 'mode-watcher';
	import { Toaster } from '$lib/components/ui/sonner/index.js';
	import * as Tooltip from '$lib/components/ui/tooltip/index.js';
	import { waitLocale, locale, locales } from 'svelte-intl-precompile';

	const LOCALE_STORAGE_KEY = 'transmitter-locale';

	let { children } = $props();

	onMount(() => {
		// migrate: old code stored 'yellow' literally, mode-watcher uses '' for no theme (now green)
		if (localStorage.getItem('transmitter-color-theme') === 'yellow') {
			localStorage.removeItem('transmitter-color-theme');
			setTheme('yellow');
		}

		// Restore saved locale, or detect from browser language
		const supported = get(locales);
		const saved = localStorage.getItem(LOCALE_STORAGE_KEY);
		if (saved && supported.includes(saved)) {
			locale.set(saved);
			return;
		}
		const browserLang = navigator.language.split('-')[0];
		locale.set(supported.includes(browserLang) ? browserLang : 'en');
	});
</script>

<svelte:head><link rel="icon" href={favicon} /></svelte:head>

<ModeWatcher defaultMode="system" themeStorageKey="transmitter-color-theme" />
<Toaster richColors position="top-right" />
<Tooltip.Provider delayDuration={200}>
	{#await waitLocale() then}
		{@render children()}
	{/await}
</Tooltip.Provider>
