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
	import SunriseIcon from '@lucide/svelte/icons/sunrise';
	import ArrowDownIcon from '@lucide/svelte/icons/arrow-down';
	import ArrowUpIcon from '@lucide/svelte/icons/arrow-up';

	import { Button } from '$lib/components/ui/button/index.js';
	import Hint from '$lib/components/Hint.svelte';
	import { downloadDirStore, torrentStore } from '$lib/stores.svelte.js';
	import { settingsStore } from '$lib/settings.svelte.js';
	import { formatSize } from '$lib/format.js';

	const FREE_SPACE_INTERVAL = 20_000;

	let { onAdd }: { onAdd: () => void } = $props();

	const onSettingsPage = $derived(page.url.pathname.startsWith('/settings'));
	const totalDownloadSpeed = $derived(
		torrentStore.torrents.reduce((sum, t) => sum + t.rateDownload, 0)
	);
	const totalUploadSpeed = $derived(
		torrentStore.torrents.reduce((sum, t) => sum + t.rateUpload, 0)
	);

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
	<!--
		На узком экране название, бейджи и кнопки в одну строку не помещаются и
		налезают друг на друга, поэтому бейджи переносятся во вторую строку
		(`order-last w-full`), а с sm возвращаются в общий ряд.
	-->
	<div class="max-w-3xl mx-auto px-4 sm:px-6 py-2 sm:py-0 sm:h-14 flex flex-wrap items-center gap-x-3 gap-y-2">
		<a href="/" class="flex items-center gap-2.5 min-w-0">
			<div class="size-7 rounded-lg bg-primary flex items-center justify-center flex-shrink-0">
				<span class="text-primary-foreground font-bold text-sm leading-none font-display">T</span>
			</div>
			<span class="font-display font-semibold text-[17px] tracking-tight truncate">Transmitter</span>
		</a>

		<div
			class="order-last w-full sm:order-none sm:w-auto sm:mr-auto flex items-center gap-2.5 min-w-0 overflow-x-auto no-scrollbar"
		>
			{#if settingsStore.showFreeSpace && downloadDirStore.defaultFreeSpace !== null}
				<Hint
					text={$tt('header.freeSpace')}
					class="inline-flex items-center gap-1.5 rounded-md bg-muted/60 px-2 py-1 text-xs font-medium text-muted-foreground tabular-nums min-w-0"
				>
					<HardDriveIcon class="size-3.5 flex-shrink-0" />
					<span class="truncate">{formatSize(downloadDirStore.defaultFreeSpace, $tt)}</span>
				</Hint>
			{/if}

			{#if settingsStore.activeShift}
				<Hint
					text={$tt(
						settingsStore.activeShift === 'night' ? 'header.nightShiftActive' : 'header.dayShiftActive'
					)}
					class="inline-flex items-center gap-1.5 rounded-md bg-primary/10 px-2 py-1 text-xs font-medium text-primary flex-shrink-0"
				>
					{#if settingsStore.activeShift === 'night'}
						<MoonIcon class="size-3.5" />
					{:else}
						<SunriseIcon class="size-3.5" />
					{/if}
				</Hint>
			{/if}

			<Hint
				text={$tt('header.totalDownloadSpeed')}
				class="inline-flex items-center gap-1.5 rounded-md bg-muted/60 px-2 py-1 text-xs font-medium text-muted-foreground tabular-nums flex-shrink-0"
			>
				<ArrowDownIcon class="size-3.5 flex-shrink-0" />
				<span>{formatSize(totalDownloadSpeed, $tt)}/{$tt('units.perSec')}</span>
			</Hint>

			<Hint
				text={$tt('header.totalUploadSpeed')}
				class="inline-flex items-center gap-1.5 rounded-md bg-muted/60 px-2 py-1 text-xs font-medium text-muted-foreground tabular-nums flex-shrink-0"
			>
				<ArrowUpIcon class="size-3.5 flex-shrink-0" />
				<span>{formatSize(totalUploadSpeed, $tt)}/{$tt('units.perSec')}</span>
			</Hint>
		</div>

		<button
			onclick={toggleMode}
			aria-label={$tt('header.toggleTheme')}
			class="ml-auto sm:ml-0 size-8 rounded-lg flex flex-shrink-0 items-center justify-center text-muted-foreground hover:text-foreground hover:bg-accent transition-colors"
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
			class="size-8 rounded-lg flex flex-shrink-0 items-center justify-center transition-colors {onSettingsPage
				? 'bg-primary/10 text-primary ring-1 ring-primary/30'
				: 'text-muted-foreground hover:text-foreground hover:bg-accent'}"
		>
			<SettingsIcon class="size-4" />
		</a>

		<Button size="sm" class="font-display font-semibold flex-shrink-0" onclick={onAdd}>
			<PlusIcon class="size-4" />
			{$tt('header.add')}
		</Button>
	</div>
</header>
