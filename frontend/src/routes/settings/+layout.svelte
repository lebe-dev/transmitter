<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { t as tt, locale } from 'svelte-intl-precompile';
	import ArrowLeftIcon from '@lucide/svelte/icons/arrow-left';

	import AppHeader from '$lib/components/AppHeader.svelte';
	import { settingsStore } from '$lib/settings.svelte.js';

	let { children } = $props();

	const TABS = [
		{ href: '/settings/general', tKey: 'settings.tabGeneral', shiftsOnly: false },
		{ href: '/settings/server', tKey: 'settings.tabServer', shiftsOnly: false },
		{ href: '/settings/shifts', tKey: 'settings.tabShifts', shiftsOnly: true },
	];

	const shiftsConfigured = $derived(
		settingsStore.day.configured || settingsStore.night.configured,
	);
	const tabs = $derived(TABS.filter((tab) => !tab.shiftsOnly || shiftsConfigured));

	onMount(() => {
		void settingsStore.load();
	});

	function onAdd() {
		settingsStore.requestAdd();
		void goto('/');
	}
</script>

<svelte:head><title>{$tt('settings.title')} — Transmitter</title></svelte:head>

<div class="min-h-screen page-shell text-foreground flex flex-col">
	<AppHeader {onAdd} />

	<div class="flex-1 w-full max-w-3xl mx-auto page-surface border-x border-border/50">
		<div class="px-4 sm:px-6 py-4 flex flex-col gap-4">
			<div class="flex items-center gap-3">
				<a
					href="/"
					class="size-8 rounded-lg flex items-center justify-center text-muted-foreground hover:text-foreground hover:bg-accent transition-colors flex-shrink-0"
					aria-label={$tt('settings.backToTorrents')}
				>
					<ArrowLeftIcon class="size-4" />
				</a>
				<h1 class="font-display text-xl font-semibold tracking-tight">{$tt('settings.title')}</h1>
			</div>

			<!-- Tabs: каждый — отдельный роут со своей ссылкой -->
			<nav class="flex items-center gap-0.5 border-b border-border/60 overflow-x-auto">
				{#each tabs as tab (tab.href)}
					{@const active = page.url.pathname === tab.href}
					<a
						href={tab.href}
						aria-current={active ? 'page' : undefined}
						class="relative px-3 pb-2.5 pt-1 text-sm font-medium transition-colors whitespace-nowrap {active
							? 'text-foreground'
							: 'text-muted-foreground hover:text-foreground'}"
					>
						{$tt(tab.tKey)}
						{#if active}
							<span class="absolute bottom-0 left-3 right-3 h-0.5 bg-primary rounded-full"></span>
						{/if}
					</a>
				{/each}
			</nav>

			{@render children()}

			<div class="mt-2 pt-4 border-t border-border/60 flex items-center gap-2 text-xs text-muted-foreground">
				v{settingsStore.version}
				<span class="opacity-40">|</span>
				<a
					href="https://github.com/lebe-dev/transmitter/blob/main/{$locale === 'en'
						? 'README.md'
						: `README.${$locale}.md`}"
					target="_blank"
					rel="noopener noreferrer"
					class="hover:underline"
				>{$tt('settings.docs')}</a>
			</div>
		</div>
	</div>
</div>
