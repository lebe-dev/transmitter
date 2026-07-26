<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { page } from '$app/state';
	import { mode, toggleMode } from 'mode-watcher';
	import { t as tt } from 'svelte-intl-precompile';
	import SunIcon from '@lucide/svelte/icons/sun';
	import MoonIcon from '@lucide/svelte/icons/moon';
	import PlusIcon from '@lucide/svelte/icons/plus';
	import SettingsIcon from '@lucide/svelte/icons/settings';
	import HardDriveIcon from '@lucide/svelte/icons/hard-drive';

	import { Button } from '$lib/components/ui/button/index.js';
	import Hint from '$lib/components/Hint.svelte';
	import { downloadDirStore } from '$lib/stores.svelte.js';
	import { settingsStore } from '$lib/settings.svelte.js';
	import { formatSize } from '$lib/format.js';

	const FREE_SPACE_INTERVAL = 20_000;

	let { onAdd }: { onAdd: () => void } = $props();

	const onSettingsPage = $derived(page.url.pathname.startsWith('/settings'));

	let freeSpaceTimer: ReturnType<typeof setInterval> | null = null;

	onMount(() => {
		void downloadDirStore.init();
		freeSpaceTimer = setInterval(() => {
			if (!document.hidden) void downloadDirStore.refreshFreeSpace();
		}, FREE_SPACE_INTERVAL);
	});

	onDestroy(() => {
		if (freeSpaceTimer) clearInterval(freeSpaceTimer);
	});
</script>

<header class="border-b border-border/50 bg-background">
	<div class="max-w-3xl mx-auto px-4 sm:px-6 h-14 flex items-center gap-3">
		<div class="flex items-center gap-2.5 mr-auto min-w-0">
			<a href="/" class="flex items-center gap-2.5 min-w-0">
				<div class="size-7 rounded-lg bg-primary flex items-center justify-center flex-shrink-0">
					<span class="text-primary-foreground font-bold text-sm leading-none font-display">T</span>
				</div>
				<span class="font-display font-semibold text-[17px] tracking-tight">Transmitter</span>
			</a>

			{#if settingsStore.showFreeSpace && downloadDirStore.defaultFreeSpace !== null}
				<Hint
					text={$tt('header.freeSpace')}
					class="inline-flex items-center gap-1.5 rounded-md bg-muted/60 px-2 py-1 text-xs font-medium text-muted-foreground tabular-nums min-w-0"
				>
					<HardDriveIcon class="size-3.5 flex-shrink-0" />
					<span class="truncate">{formatSize(downloadDirStore.defaultFreeSpace, $tt)}</span>
				</Hint>
			{/if}
		</div>

		<button
			onclick={toggleMode}
			aria-label={$tt('header.toggleTheme')}
			class="size-8 rounded-lg flex items-center justify-center text-muted-foreground hover:text-foreground hover:bg-accent transition-colors"
		>
			{#if mode.current === 'dark'}
				<SunIcon class="size-4" />
			{:else}
				<MoonIcon class="size-4" />
			{/if}
		</button>

		<a
			href={onSettingsPage ? '/' : '/settings/general'}
			aria-label={$tt('header.settings')}
			aria-current={onSettingsPage ? 'page' : undefined}
			class="size-8 rounded-lg flex items-center justify-center transition-colors {onSettingsPage
				? 'bg-primary/10 text-primary ring-1 ring-primary/30'
				: 'text-muted-foreground hover:text-foreground hover:bg-accent'}"
		>
			<SettingsIcon class="size-4" />
		</a>

		<Button size="sm" class="font-display font-semibold" onclick={onAdd}>
			<PlusIcon class="size-4" />
			{$tt('header.add')}
		</Button>
	</div>
</header>
