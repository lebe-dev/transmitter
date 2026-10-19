<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { get } from 'svelte/store';
	import { toast } from 'svelte-sonner';
	import { page } from '$app/state';
	import { mode, toggleMode } from 'mode-watcher';
	import { t as tt, locale } from 'svelte-intl-precompile';
	import SunIcon from '@lucide/svelte/icons/sun';
	import MoonIcon from '@lucide/svelte/icons/moon';
	import PlusIcon from '@lucide/svelte/icons/plus';
	import SettingsIcon from '@lucide/svelte/icons/settings';
	import HardDriveIcon from '@lucide/svelte/icons/hard-drive';
	import SunriseIcon from '@lucide/svelte/icons/sunrise';
	import ArrowDownIcon from '@lucide/svelte/icons/arrow-down';
	import ArrowUpIcon from '@lucide/svelte/icons/arrow-up';
	import WifiOffIcon from '@lucide/svelte/icons/wifi-off';
	import PauseIcon from '@lucide/svelte/icons/pause';
	import PlayIcon from '@lucide/svelte/icons/play';

	import { Button } from '$lib/components/ui/button/index.js';
	import Hint from '$lib/components/Hint.svelte';
	import { downloadDirStore, pauseStore, torrentStore } from '$lib/stores.svelte.js';
	import { settingsStore } from '$lib/settings.svelte.js';
	import { connectivity } from '$lib/connectivity.svelte.js';
	import { formatSize, formatDateTime } from '$lib/format.js';
	import { failText } from '$lib/offline.js';

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

	async function toggleGlobalPause() {
		const t = get(tt);
		const pausing = !pauseStore.paused;
		try {
			const count = await pauseStore.toggle();
			toast.success(t(pausing ? 'globalPause.paused' : 'globalPause.resumed', { values: { count } }));
			await torrentStore.refresh();
		} catch (err) {
			toast.error(failText(err, t, pausing ? 'globalPause.failPause' : 'globalPause.failResume'));
		}
	}

	onMount(() => {
		void downloadDirStore.init();
		void pauseStore.init();
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
	<div class="max-w-3xl mx-auto px-4 sm:px-6 py-2 sm:py-0 sm:h-14 flex flex-wrap items-center gap-x-2 sm:gap-x-3 gap-y-2">
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

		<!-- Глобальная пауза: сервер запоминает, что работало, и при снятии запускает только это. -->
		<Hint text={$tt(pauseStore.paused ? 'globalPause.resume' : 'globalPause.pause')} side="bottom">
			{#snippet trigger({ props })}
				<button
					{...props}
					onclick={toggleGlobalPause}
					disabled={pauseStore.busy || connectivity.offline}
					aria-label={$tt(pauseStore.paused ? 'globalPause.resume' : 'globalPause.pause')}
					aria-pressed={pauseStore.paused}
					class="ml-auto sm:ml-0 h-8 rounded-lg flex flex-shrink-0 items-center justify-center transition-colors disabled:opacity-50 {pauseStore.paused
						? 'w-8 bg-amber-500/10 text-amber-600 dark:text-amber-400 ring-1 ring-amber-500/30 hover:bg-amber-500/15'
						: 'w-8 text-muted-foreground hover:text-foreground hover:bg-accent'}"
				>
					{#if pauseStore.paused}
						<PlayIcon class="size-4" />
					{:else}
						<PauseIcon class="size-4" />
					{/if}
				</button>
			{/snippet}
		</Hint>

		<button
			onclick={toggleMode}
			aria-label={$tt('header.toggleTheme')}
			class="size-8 rounded-lg flex flex-shrink-0 items-center justify-center text-muted-foreground hover:text-foreground hover:bg-accent transition-colors"
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

		<Button
			size="sm"
			class="font-display font-semibold flex-shrink-0"
			onclick={onAdd}
			disabled={connectivity.offline}
			title={connectivity.offline ? $tt('offline.unavailable') : undefined}
		>
			<PlusIcon class="size-4" />
			{$tt('header.add')}
		</Button>
	</div>

	{#if connectivity.offline}
		<div class="border-t border-amber-500/20 bg-amber-500/10 text-amber-700 dark:text-amber-300">
			<div
				class="max-w-3xl mx-auto px-4 sm:px-6 py-1.5 flex items-center gap-2 text-xs font-medium"
				role="status"
			>
				<WifiOffIcon class="size-3.5 flex-shrink-0" />
				<span class="min-w-0">
					{torrentStore.syncedAt !== null
						? $tt('offline.banner', {
								values: { time: formatDateTime(torrentStore.syncedAt, $locale) },
							})
						: $tt('offline.bannerNoData')}
				</span>
			</div>
		</div>
	{/if}
</header>
