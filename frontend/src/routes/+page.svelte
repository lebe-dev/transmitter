<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { get } from 'svelte/store';
	import { toast } from 'svelte-sonner';
	import { setTheme, theme } from 'mode-watcher';
	import { getCoreRowModel, getSortedRowModel, type ColumnDef, type SortingState } from '@tanstack/table-core';
	import { mergeProps } from 'bits-ui';
	import { t as tt, locale } from 'svelte-intl-precompile';
	import SunIcon from '@lucide/svelte/icons/sun';
	import PlusIcon from '@lucide/svelte/icons/plus';
	import PlayIcon from '@lucide/svelte/icons/play';
	import PauseIcon from '@lucide/svelte/icons/pause';
	import Trash2Icon from '@lucide/svelte/icons/trash-2';
	import UploadIcon from '@lucide/svelte/icons/upload';
	import LinkIcon from '@lucide/svelte/icons/link';
	import SearchIcon from '@lucide/svelte/icons/search';
	import InboxIcon from '@lucide/svelte/icons/inbox';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';
	import ChevronDownIcon from '@lucide/svelte/icons/chevron-down';
	import ArrowUpIcon from '@lucide/svelte/icons/arrow-up';
	import PinIcon from '@lucide/svelte/icons/pin';
	import MoonStarIcon from '@lucide/svelte/icons/moon-star';
	import NotebookPenIcon from '@lucide/svelte/icons/notebook-pen';

	import XIcon from '@lucide/svelte/icons/x';
	import FolderIcon from '@lucide/svelte/icons/folder';

	import { Checkbox } from '$lib/components/ui/checkbox/index.js';
	import { torrentStore, pinStore, downloadDirStore, noteStore } from '$lib/stores.svelte.js';
	import { settingsStore } from '$lib/settings.svelte.js';
	import { addTorrentMagnet, addTorrentFile, startTorrents, stopTorrents, removeTorrents, getTorrentFiles, setFilesWanted, setTorrentLabels, getFreeSpace } from '$lib/api.js';
	import { formatSize, formatSpeed, formatEta, formatDate } from '$lib/format.js';
	import { parseTorrentSize } from '$lib/bencode.js';
	import { DAY_SHIFT_LABEL, NIGHT_SHIFT_LABEL } from '$lib/bulk.js';
	import type { Torrent, FilterStatus } from '$lib/types.js';
	import { createSvelteTable } from '$lib/components/ui/data-table/index.js';
	import * as AlertDialog from '$lib/components/ui/alert-dialog/index.js';
	import { Spinner } from '$lib/components/ui/spinner/index.js';
	import { Button } from '$lib/components/ui/button/index.js';
	import AppHeader from '$lib/components/AppHeader.svelte';
	import BulkActionsMenu from '$lib/components/BulkActionsMenu.svelte';
	import Hint from '$lib/components/Hint.svelte';
	import TorrentDetailPanel from '$lib/components/TorrentDetailPanel.svelte';
	import FileSelectDialog from '$lib/components/FileSelectDialog.svelte';

	// ── Status ────────────────────────────────────────────────────────────────

	const STATUS_KEYS: Record<number, string> = {
		0: 'status.stopped',
		1: 'status.checkQueue',
		2: 'status.checking',
		3: 'status.queued',
		4: 'status.downloading',
		5: 'status.seedQueue',
		6: 'status.seeding',
	};

	function statusPillClass(status: number): string {
		switch (status) {
			case 4: return 'bg-primary/10 text-primary';
			case 3: return 'bg-blue-500/10 text-blue-600 dark:text-blue-400';
			case 6: case 5: return 'bg-primary/10 text-primary';
			case 1: case 2: return 'bg-amber-500/10 text-amber-600 dark:text-amber-400';
			case 0: default: return 'bg-muted text-muted-foreground';
		}
	}

	function progressBarClass(t: Torrent): string {
		if (t.status === 0) return 'bg-muted-foreground/40';
		if (colorTheme !== 'default') return 'bg-primary';
		if (t.percentDone >= 1) return 'bg-emerald-500';
		if (t.status === 4 || t.status === 3) return 'bg-primary';
		return 'bg-muted-foreground/60';
	}

	function isDownloading(t: Torrent): boolean {
		return t.status === 4 && t.rateDownload > 0;
	}

	// ── Table (sort logic only) ──────────────────────────────────────────────

	let sorting = $state<SortingState>([{ id: 'addedDate', desc: true }]);

	const SORT_OPTIONS = [
		{ value: 'addedDate', key: 'sort.added' },
		{ value: 'name', key: 'sort.name' },
		{ value: 'totalSize', key: 'sort.size' },
		{ value: 'percentDone', key: 'sort.progress' },
		{ value: 'status', key: 'sort.status' },
	] as const;

	let sortField = $state('addedDate');
	let sortDesc = $state(true);

	$effect(() => {
		sorting = [{ id: sortField, desc: sortDesc }];
	});

	const columns: ColumnDef<Torrent>[] = [
		{ accessorKey: 'name', header: 'Name' },
		{ accessorKey: 'status', header: 'Status' },
		{ accessorKey: 'percentDone', header: 'Progress' },
		{ accessorKey: 'totalSize', header: 'Size' },
		{ accessorKey: 'rateDownload', header: '↓' },
		{ accessorKey: 'rateUpload', header: '↑' },
		{ accessorKey: 'eta', header: 'ETA' },
		{ accessorKey: 'addedDate', header: 'Added' },
	];

	const table = createSvelteTable({
		get data() {
			return torrentStore.filtered;
		},
		columns,
		getCoreRowModel: getCoreRowModel(),
		getSortedRowModel: getSortedRowModel(),
		state: {
			get sorting() {
				return sorting;
			},
		},
		onSortingChange: (updater) => {
			if (typeof updater === 'function') sorting = updater(sorting);
			else sorting = updater;
		},
	});

	// ── Pin-aware row order ──────────────────────────────────────────────────

	const sortedRows = $derived.by(() => {
		const rows = table.getRowModel().rows;
		return rows.toSorted((a, b) => {
			const aPinned = pinStore.isPinned(a.original.hashString);
			const bPinned = pinStore.isPinned(b.original.hashString);
			if (aPinned === bPinned) return 0;
			return aPinned ? -1 : 1;
		});
	});

	// ── Filter counts ─────────────────────────────────────────────────────────

	const counts = $derived({
		all: torrentStore.torrents.length,
		downloading: torrentStore.torrents.filter((t) => t.status === 4 || t.status === 3).length,
		seeding: torrentStore.torrents.filter((t) => t.status === 6 || t.status === 5).length,
		paused: torrentStore.torrents.filter((t) => t.status === 0).length,
		done: torrentStore.torrents.filter(
			(t) => t.percentDone === 1 && (t.status === 0 || t.status === 5 || t.status === 6)
		).length,
	});

	const FILTER_KEYS: { key: FilterStatus; tKey: string }[] = [
		{ key: 'all', tKey: 'filters.all' },
		{ key: 'downloading', tKey: 'filters.downloading' },
		{ key: 'seeding', tKey: 'filters.seeding' },
		{ key: 'paused', tKey: 'filters.paused' },
		{ key: 'done', tKey: 'filters.done' },
	];

	// ── File selection dialog ─────────────────────────────────────────────────

	let fileSelectOpen = $state(false);
	let fileSelectTorrentId = $state(0);
	let fileSelectTorrentName = $state('');

	async function handleFileSelectConfirm(wantedIndices: number[], unwantedIndices: number[]) {
		try {
			if (unwantedIndices.length > 0) {
				await setFilesWanted(fileSelectTorrentId, wantedIndices, unwantedIndices);
			}
			await startTorrents([fileSelectTorrentId]);
			toast.success(get(tt)('toast.added'));
			fileSelectOpen = false;
			await torrentStore.refresh();
		} catch (err) {
			toast.error(err instanceof Error ? err.message : get(tt)('toast.failAdd'));
		}
	}

	async function handleFileSelectCancel() {
		try {
			await removeTorrents([fileSelectTorrentId], false);
		} catch {
			// best effort cleanup
		}
		fileSelectOpen = false;
		await torrentStore.refresh();
	}

	// ── Add torrent dialog ────────────────────────────────────────────────────

	let addOpen = $state(false);
	let addMode = $state<'magnet' | 'file'>('file');
	let magnetUrl = $state('');
	let pendingFile = $state<File | null>(null);
	let fileInputEl = $state<HTMLInputElement | null>(null);
	let isAdding = $state(false);
	let dragOver = $state(false);
	let downloadDir = $state('');
	let showDirDropdown = $state(false);

	// Free-space guard for the destination folder
	let pendingFileSize = $state<number | null>(null);
	let destFreeSpace = $state<number | null>(null);
	let freeSpaceTimer: ReturnType<typeof setTimeout> | null = null;

	const isDirValid = $derived(!downloadDir.trim() || downloadDir.trim().startsWith('/'));

	// Not enough space only when we know both the torrent size and free space
	const notEnoughSpace = $derived(
		pendingFileSize !== null && destFreeSpace !== null && pendingFileSize > destFreeSpace,
	);

	// Parse the selected .torrent client-side to learn its total size
	$effect(() => {
		const file = pendingFile;
		if (!file) {
			pendingFileSize = null;
			return;
		}
		let cancelled = false;
		file
			.arrayBuffer()
			.then((buf) => {
				if (!cancelled) pendingFileSize = parseTorrentSize(buf);
			})
			.catch(() => {
				if (!cancelled) pendingFileSize = null;
			});
		return () => {
			cancelled = true;
		};
	});

	// Fetch free space for the chosen destination (debounced while typing)
	$effect(() => {
		if (!addOpen) return;
		const path = downloadDir.trim() || downloadDirStore.defaultDir;
		if (freeSpaceTimer) clearTimeout(freeSpaceTimer);
		if (!path || !path.startsWith('/')) {
			destFreeSpace = null;
			return;
		}
		freeSpaceTimer = setTimeout(() => {
			getFreeSpace(path)
				.then((bytes) => {
					destFreeSpace = bytes;
				})
				.catch(() => {
					destFreeSpace = null;
				});
		}, 350);
		return () => {
			if (freeSpaceTimer) clearTimeout(freeSpaceTimer);
		};
	});

	function readFileAsBase64(file: File): Promise<string> {
		return new Promise((resolve, reject) => {
			const reader = new FileReader();
			reader.onload = (e) => {
				const result = e.target?.result as string;
				resolve(result.split(',')[1]);
			};
			reader.onerror = reject;
			reader.readAsDataURL(file);
		});
	}

	function onFileChange(e: Event) {
		const input = e.target as HTMLInputElement;
		pendingFile = input.files?.[0] ?? null;
	}

	function onDrop(e: DragEvent) {
		e.preventDefault();
		dragOver = false;
		const file = e.dataTransfer?.files[0];
		if (file && (file.name.endsWith('.torrent') || file.type === 'application/x-bittorrent')) {
			pendingFile = file;
			addMode = 'file';
		}
	}

	async function handleAdd() {
		if (isAdding || notEnoughSpace) return;
		isAdding = true;
		try {
			const dir = downloadDir.trim() || undefined;
			if (addMode === 'magnet') {
				if (!magnetUrl.trim()) return;
				await addTorrentMagnet(magnetUrl.trim(), dir);
			} else {
				if (!pendingFile) return;
				const b64 = await readFileAsBase64(pendingFile);
				const added = await addTorrentFile(b64, dir, true);

				if (added.duplicate) {
					toast.success(get(tt)('toast.added'));
					if (dir) downloadDirStore.addDir(dir);
					addOpen = false;
					magnetUrl = '';
					pendingFile = null;
					await torrentStore.refresh();
					return;
				}

				const files = await getTorrentFiles(added.id);

				if (files.length <= 1) {
					await startTorrents([added.id]);
				} else {
					if (dir) downloadDirStore.addDir(dir);
					addOpen = false;
					magnetUrl = '';
					pendingFile = null;
					fileSelectTorrentId = added.id;
					fileSelectTorrentName = added.name;
					fileSelectOpen = true;
					return;
				}
			}
			if (dir) downloadDirStore.addDir(dir);
			toast.success(get(tt)('toast.added'));
			addOpen = false;
			magnetUrl = '';
			pendingFile = null;
			await torrentStore.refresh();
			void downloadDirStore.refreshFreeSpace();
		} catch (err) {
			toast.error(err instanceof Error ? err.message : get(tt)('toast.failAdd'));
		} finally {
			isAdding = false;
		}
	}

	function resetAddDialog() {
		magnetUrl = '';
		pendingFile = null;
		pendingFileSize = null;
		addMode = 'file';
		isAdding = false;
		dragOver = false;
		downloadDir = downloadDirStore.selectedDir || downloadDirStore.defaultDir;
		// Seed with the cached free space for the default dir so the info row is
		// present on the first frame — the debounced fetch then refreshes it in
		// place instead of appearing late and shifting the dialog.
		destFreeSpace =
			downloadDir === downloadDirStore.defaultDir ? downloadDirStore.defaultFreeSpace : null;
		showDirDropdown = false;
	}

	// ── Delete dialog ─────────────────────────────────────────────────────────

	let deleteOpen = $state(false);
	let deleteTarget = $state<Torrent | null>(null);
	let deleteWithData = $state(false);
	let isDeleting = $state(false);

	function openDeleteDialog(t: Torrent) {
		deleteTarget = t;
		deleteWithData = settingsStore.deleteWithData;
		deleteOpen = true;
	}

	async function handleDelete() {
		if (!deleteTarget || isDeleting) return;
		isDeleting = true;
		try {
			await removeTorrents([deleteTarget.id], deleteWithData);
			toast.success(get(tt)('toast.deleted', { values: { name: deleteTarget.name } }));
			deleteOpen = false;
			deleteTarget = null;
			await torrentStore.refresh();
			void downloadDirStore.refreshFreeSpace();
		} catch (err) {
			toast.error(err instanceof Error ? err.message : get(tt)('toast.failDelete'));
		} finally {
			isDeleting = false;
		}
	}

	// ── Torrent actions ───────────────────────────────────────────────────────

	async function handleStart(torrent: Torrent) {
		try {
			await startTorrents([torrent.id]);
			toast.success(get(tt)('toast.started', { values: { name: torrent.name } }));
			await torrentStore.refresh();
		} catch {
			toast.error(get(tt)('toast.failStart'));
		}
	}

	async function handleStop(torrent: Torrent) {
		try {
			await stopTorrents([torrent.id]);
			toast.success(get(tt)('toast.paused', { values: { name: torrent.name } }));
			await torrentStore.refresh();
		} catch {
			toast.error(get(tt)('toast.failPause'));
		}
	}

	// ── Shifts (night / day) ─────────────────────────────────────────────────

	const nightShift = $derived(settingsStore.night);
	const dayShift = $derived(settingsStore.day);

	function hasShiftLabel(t: Torrent, label: string): boolean {
		return Array.isArray(t.labels) && t.labels.includes(label);
	}

	/** Toggles a shift label on the torrent, leaving its other labels untouched. */
	async function toggleShiftLabel(
		t: Torrent,
		label: string,
		toastKeys: { on: string; off: string; fail: string },
	) {
		const enabled = hasShiftLabel(t, label);
		const current = t.labels ?? [];
		const next = enabled ? current.filter((l) => l !== label) : [...current, label];
		try {
			await setTorrentLabels(t.id, next);
			toast.success(get(tt)(enabled ? toastKeys.off : toastKeys.on, { values: { name: t.name } }));
			await torrentStore.refresh();
		} catch {
			toast.error(get(tt)(toastKeys.fail));
		}
	}

	const NIGHT_SHIFT_TOASTS = {
		on: 'toast.nightShiftOn',
		off: 'toast.nightShiftOff',
		fail: 'toast.failNightShift',
	};
	const DAY_SHIFT_TOASTS = {
		on: 'toast.dayShiftOn',
		off: 'toast.dayShiftOff',
		fail: 'toast.failDayShift',
	};

	// ── Compact view ─────────────────────────────────────────────────────────

	const compactView = $derived(settingsStore.compactView);

	// ── Detail panel ─────────────────────────────────────────────────────────

	let detailOpen = $state(false);
	let detailTorrent = $state<Torrent | null>(null);

	function openDetail(t: Torrent) {
		detailTorrent = t;
		detailOpen = true;
	}

	// ── Color theme ───────────────────────────────────────────────────────────

	// mode-watcher manages data-theme attr & localStorage ('mode-watcher-theme')
	// green = "" (default, no data-theme attr), others = theme name
	const colorTheme = $derived(theme.current || 'green');

	// ── Scroll to top ─────────────────────────────────────────────────────────

	let showScrollTop = $state(false);

	function onScroll() {
		showScrollTop = window.scrollY > 300;
	}

	function scrollToTop() {
		window.scrollTo({ top: 0, behavior: 'smooth' });
	}

	// ── Lifecycle ─────────────────────────────────────────────────────────────

	onMount(() => {
		torrentStore.init();
		void downloadDirStore.init();
		settingsStore
			.load()
			.then((s) => noteStore.init(s.noteMaxLength))
			.catch(() => {});
		window.addEventListener('scroll', onScroll, { passive: true });
	});
	onDestroy(() => {
		torrentStore.destroy();
		window.removeEventListener('scroll', onScroll);
	});

	// Кнопка «Добавить» в шапке страницы настроек уводит сюда с этим флагом.
	$effect(() => {
		if (!settingsStore.addRequested) return;
		settingsStore.addRequested = false;
		resetAddDialog();
		addOpen = true;
	});
</script>

<!--
	Кнопки смен в шапке карточки торрента (обычный и компактный вид).
	Смена без окна в переменных окружения не показывается вовсе; выключенная
	смена показывается серой, отжатой и не реагирует на клик.
-->
{#snippet shiftToggles(t: Torrent)}
	{#if dayShift.configured}
		{@const onDS = dayShift.enabled && hasShiftLabel(t, DAY_SHIFT_LABEL)}
		<Hint
			text={!dayShift.enabled
				? $tt('actions.dayShiftDisabled')
				: dayShift.start && dayShift.end
					? $tt('actions.dayShiftWindow', { values: { start: dayShift.start, end: dayShift.end } })
					: onDS ? $tt('actions.dayShiftOff') : $tt('actions.dayShiftOn')}
		>
			{#snippet trigger({ props })}
				<button
					{...mergeProps(props, {
						onclick: (e: MouseEvent) => {
							e.stopPropagation();
							if (dayShift.enabled) toggleShiftLabel(t, DAY_SHIFT_LABEL, DAY_SHIFT_TOASTS);
						}
					})}
					aria-disabled={!dayShift.enabled}
					aria-pressed={onDS}
					aria-label={!dayShift.enabled
						? $tt('actions.dayShiftDisabled')
						: onDS ? $tt('actions.dayShiftOff') : $tt('actions.dayShiftOn')}
					class="size-7 rounded-md flex items-center justify-center transition-colors {!dayShift.enabled
						? 'text-muted-foreground/25 cursor-not-allowed'
						: onDS
							? 'bg-amber-500/10 text-amber-600 dark:text-amber-300'
							: 'text-muted-foreground/40 hover:text-muted-foreground'}"
				>
					<SunIcon class="size-3.5" />
				</button>
			{/snippet}
		</Hint>
	{/if}
	{#if nightShift.configured}
		{@const onNS = nightShift.enabled && hasShiftLabel(t, NIGHT_SHIFT_LABEL)}
		<Hint
			text={!nightShift.enabled
				? $tt('actions.nightShiftDisabled')
				: nightShift.start && nightShift.end
					? $tt('actions.nightShiftWindow', { values: { start: nightShift.start, end: nightShift.end } })
					: onNS ? $tt('actions.nightShiftOff') : $tt('actions.nightShiftOn')}
		>
			{#snippet trigger({ props })}
				<button
					{...mergeProps(props, {
						onclick: (e: MouseEvent) => {
							e.stopPropagation();
							if (nightShift.enabled) toggleShiftLabel(t, NIGHT_SHIFT_LABEL, NIGHT_SHIFT_TOASTS);
						}
					})}
					aria-disabled={!nightShift.enabled}
					aria-pressed={onNS}
					aria-label={!nightShift.enabled
						? $tt('actions.nightShiftDisabled')
						: onNS ? $tt('actions.nightShiftOff') : $tt('actions.nightShiftOn')}
					class="size-7 rounded-md flex items-center justify-center transition-colors {!nightShift.enabled
						? 'text-muted-foreground/25 cursor-not-allowed'
						: onNS
							? 'bg-indigo-600/10 text-indigo-600 dark:text-indigo-300'
							: 'text-muted-foreground/40 hover:text-muted-foreground'}"
				>
					<MoonStarIcon class="size-3.5" />
				</button>
			{/snippet}
		</Hint>
	{/if}
{/snippet}

<svelte:head><title>Transmitter</title></svelte:head>

<!-- ── Layout ──────────────────────────────────────────────────────────────── -->
<div class="min-h-screen page-shell text-foreground flex flex-col">

	<AppHeader
		onAdd={() => {
			resetAddDialog();
			addOpen = true;
		}}
	/>

	<!-- Content -->
	<div class="flex-1 w-full max-w-3xl mx-auto page-surface border-x border-border/50 px-4 sm:px-6 py-4 flex flex-col gap-4">

		<!-- Search -->
		<div class="relative">
			<SearchIcon class="absolute left-3 top-1/2 -translate-y-1/2 size-4 text-muted-foreground pointer-events-none" />
			<input
				type="search"
				placeholder={$tt('search.placeholder')}
				class="w-full h-10 rounded-lg border border-input bg-background pl-9 pr-3 text-sm outline-none transition-colors focus:border-primary/40 focus:ring-2 focus:ring-primary/10"
				bind:value={torrentStore.search}
			/>
		</div>

		<!-- Filters + Sort -->
		<div class="flex items-center gap-1 overflow-x-auto -mx-4 px-4 sm:mx-0 sm:px-0">
			<div class="flex items-center gap-0.5">
				{#each FILTER_KEYS as f}
					<button
						class="relative px-3 py-1.5 text-sm font-medium transition-colors {torrentStore.filterStatus === f.key
							? 'text-foreground'
							: 'text-muted-foreground hover:text-foreground'}"
						onclick={() => (torrentStore.filterStatus = f.key)}
					>
						{$tt(f.tKey)}
						<span class="ml-0.5 text-[11px] opacity-50 tabular-nums">{counts[f.key]}</span>
						{#if torrentStore.filterStatus === f.key}
							<span class="absolute bottom-0 left-3 right-3 h-0.5 bg-primary rounded-full"></span>
						{/if}
					</button>
				{/each}
			</div>

			<div class="ml-auto flex items-center gap-1 flex-shrink-0">
				<BulkActionsMenu />
				<span class="w-px h-4 bg-border/70 mx-1.5"></span>
				<span class="text-xs text-muted-foreground">{$tt('sort.label')}</span>
				<button
					class="flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground transition-colors px-1.5 py-1 rounded"
					onclick={() => {
						const idx = SORT_OPTIONS.findIndex((o) => o.value === sortField);
						const next = SORT_OPTIONS[(idx + 1) % SORT_OPTIONS.length];
						sortField = next.value;
					}}
				>
					{$tt(SORT_OPTIONS.find((o) => o.value === sortField)?.key ?? 'sort.added')}
				</button>
				<button
					class="text-xs text-muted-foreground hover:text-foreground transition-colors p-1 rounded"
					onclick={() => (sortDesc = !sortDesc)}
					aria-label={$tt('sort.toggleDirection')}
				>
					{sortDesc ? '↓' : '↑'}
				</button>
			</div>
		</div>

		<!-- Error banner -->
		{#if torrentStore.error}
			<div class="flex items-start gap-3 rounded-lg border border-destructive/20 bg-destructive/5 p-4">
				<AlertCircleIcon class="size-4 text-destructive flex-shrink-0 mt-0.5" />
				<div class="text-sm">
					<p class="font-medium text-destructive">{$tt('error.connection')}</p>
					<p class="text-muted-foreground mt-0.5">{torrentStore.error}</p>
				</div>
			</div>
		{/if}

		<!-- Loading state -->
		{#if torrentStore.loading}
			<div class="flex flex-col gap-3">
				{#each [0, 1, 2] as i}
					<div
						class="rounded-lg border border-border/60 p-4 space-y-3 animate-pulse"
						style="animation-delay: {i * 100}ms"
					>
						<div class="h-4 bg-muted rounded w-3/4"></div>
						<div class="flex items-center gap-3">
							<div class="h-3 bg-muted rounded w-16"></div>
							<div class="h-1.5 bg-muted rounded flex-1"></div>
							<div class="h-3 bg-muted rounded w-12"></div>
						</div>
						<div class="h-3 bg-muted rounded w-1/2"></div>
					</div>
				{/each}
			</div>

		<!-- Empty state -->
		{:else if torrentStore.filtered.length === 0}
			<div class="flex flex-col items-center justify-center py-16 text-center">
				<div class="size-16 rounded-2xl bg-muted flex items-center justify-center mb-4">
					<InboxIcon class="size-7 text-muted-foreground" />
				</div>
				{#if torrentStore.torrents.length === 0}
					<h3 class="font-display font-semibold text-lg mb-1">{$tt('empty.title')}</h3>
					<p class="text-sm text-muted-foreground mb-5 max-w-xs">{$tt('empty.description')}</p>
					<Button
						size="sm"
						class="font-display font-semibold"
						onclick={() => {
							resetAddDialog();
							addOpen = true;
						}}
					>
						<PlusIcon class="size-4" />
						{$tt('empty.addButton')}
					</Button>
				{:else}
					<h3 class="font-display font-semibold text-lg mb-1">{$tt('empty.noMatchTitle')}</h3>
					<p class="text-sm text-muted-foreground max-w-xs">{$tt('empty.noMatchDescription')}</p>
				{/if}
			</div>

		<!-- Torrent cards -->
		{:else}
			<div class="flex flex-col {compactView ? 'gap-1' : 'gap-2'}">
				{#each sortedRows as row, i (row.id)}
					{@const t = row.original}
					{@const pinned = pinStore.isPinned(t.hashString)}
				{@const note = noteStore.get(t.hashString)}
					{#if compactView}
						<!-- Compact card: same structure, reduced padding, no date -->
						<div
							class="group rounded-lg border px-4 py-2.5 transition-all hover:shadow-sm cursor-pointer {pinned
								? 'border-primary/40 hover:border-primary/60'
								: 'border-border/60 hover:border-border'} {t.error ? 'border-l-2 border-l-destructive' : ''}"
							style="animation: card-enter 0.3s ease-out both; animation-delay: {Math.min(i, 10) * 30}ms"
							onclick={() => openDetail(t)}
							onkeydown={(e) => e.key === 'Enter' && openDetail(t)}
							role="button"
							tabindex="0"
						>
							<!-- Row 1: Name + Shift toggles + Pin -->
							<div class="flex items-start justify-between gap-3 mb-1.5">
								<h3 class="font-display text-[15px] font-semibold leading-snug line-clamp-2 min-w-0">
									{t.name}
								</h3>
								<div class="flex items-center gap-0.5 flex-shrink-0">
									{@render shiftToggles(t)}
									<button
										onclick={(e) => { e.stopPropagation(); pinStore.toggle(t.hashString); }}
										aria-label={pinned ? $tt('actions.unpin') : $tt('actions.pin')}
										class="size-7 rounded-md flex items-center justify-center transition-colors {pinned
											? 'bg-primary/10 text-primary'
											: 'text-muted-foreground/40 hover:text-muted-foreground'}"
									>
										<PinIcon class="size-3.5" />
									</button>
								</div>
							</div>

							<!-- Note -->
						{#if note}
							<div class="flex items-start gap-1.5 mb-1.5 text-xs text-muted-foreground">
								<NotebookPenIcon class="size-3 mt-0.5 flex-shrink-0" />
								<Hint text={note} class="line-clamp-1 italic">{note}</Hint>
							</div>
						{/if}

						<!-- Row 2: Status + Progress + Size -->
							<div class="flex items-center gap-2.5 mb-1.5">
								<span class="inline-flex items-center px-2 py-0.5 rounded-md text-[11px] font-medium flex-shrink-0 {statusPillClass(t.status)}">
									{$tt(STATUS_KEYS[t.status] ?? 'status.stopped')}
								</span>

								<div class="flex-1 flex items-center gap-2 min-w-0">
									<div class="flex-1 h-1.5 rounded-full bg-muted overflow-hidden">
										<div
											class="h-full rounded-full transition-[width] duration-700 ease-out {progressBarClass(t)}"
											class:animate-[progress-pulse_2s_ease-in-out_infinite]={isDownloading(t)}
											style="width: {t.percentDone * 100}%"
										></div>
									</div>
									<span class="text-xs text-muted-foreground tabular-nums w-8 text-right flex-shrink-0">
										{(t.percentDone * 100).toFixed(0)}%
									</span>
								</div>

								<div class="flex items-center gap-2 flex-shrink-0">
								<span class="text-xs text-muted-foreground tabular-nums">{formatSize(t.totalSize, $tt)}</span>
								{#if t.uploadedEver > 0}
									<Hint text={$tt('card.uploadedTotal')} class="text-xs text-primary tabular-nums">⬆ {formatSize(t.uploadedEver, $tt)}</Hint>
								{/if}
							</div>
							</div>

							<!-- Row 3: Speeds + ETA + Error | Actions (no date) -->
							<div class="flex items-center justify-between gap-2">
								<div class="flex items-center gap-2 text-xs text-muted-foreground tabular-nums min-w-0 overflow-hidden">
									{#if formatSpeed(t.rateDownload, $tt)}
										<Hint text={$tt('card.downloadSpeed')} class="text-primary">↓ {formatSpeed(t.rateDownload, $tt)}</Hint>
									{/if}
									{#if formatSpeed(t.rateUpload, $tt)}
										<Hint text={$tt('card.uploadSpeed')} class="text-primary">↑ {formatSpeed(t.rateUpload, $tt)}</Hint>
									{/if}
									{#if t.status !== 0 && t.status !== 6 && t.status !== 5 && formatEta(t.eta, $tt)}
										<span>{$tt('eta.prefix')} {formatEta(t.eta, $tt)}</span>
									{/if}
									{#if t.errorString}
										<span class="text-destructive truncate">{t.errorString}</span>
									{/if}
								</div>

								<!-- Action buttons -->
								<div class="flex items-center gap-0.5 opacity-0 group-hover:opacity-100 transition-opacity touch-device:opacity-100">
									{#if t.status === 0}
										<button
											onclick={(e) => { e.stopPropagation(); handleStart(t); }}
											aria-label={$tt('actions.resume')}
											class="size-7 rounded-md flex items-center justify-center text-muted-foreground hover:text-foreground hover:bg-accent transition-colors"
										>
											<PlayIcon class="size-3.5" />
										</button>
									{:else if t.status === 4 || t.status === 3 || t.status === 6 || t.status === 5}
										<button
											onclick={(e) => { e.stopPropagation(); handleStop(t); }}
											aria-label={$tt('actions.pause')}
											class="size-7 rounded-md flex items-center justify-center text-muted-foreground hover:text-foreground hover:bg-accent transition-colors"
										>
											<PauseIcon class="size-3.5" />
										</button>
									{/if}
									<button
										onclick={(e) => { e.stopPropagation(); openDeleteDialog(t); }}
										aria-label={$tt('actions.delete')}
										class="size-7 rounded-md flex items-center justify-center text-muted-foreground hover:text-destructive hover:bg-destructive/10 transition-colors"
									>
										<Trash2Icon class="size-3.5" />
									</button>
								</div>
							</div>
						</div>
					{:else}
						<!-- Normal card -->
						<div
							class="group rounded-lg border p-4 transition-all hover:shadow-sm cursor-pointer {pinned
								? 'border-primary/40 hover:border-primary/60'
								: 'border-border/60 hover:border-border'} {t.error ? 'border-l-2 border-l-destructive' : ''}"
							style="animation: card-enter 0.3s ease-out both; animation-delay: {Math.min(i, 10) * 30}ms"
							onclick={() => openDetail(t)}
							onkeydown={(e) => e.key === 'Enter' && openDetail(t)}
							role="button"
							tabindex="0"
						>
							<!-- Row 1: Name + Shift toggles + Pin -->
							<div class="flex items-start justify-between gap-3 mb-2">
								<h3 class="font-display text-[15px] font-semibold leading-snug line-clamp-2 min-w-0">
									{t.name}
								</h3>
								<div class="flex items-center gap-0.5 flex-shrink-0">
									{@render shiftToggles(t)}
									<button
										onclick={(e) => { e.stopPropagation(); pinStore.toggle(t.hashString); }}
										aria-label={pinned ? $tt('actions.unpin') : $tt('actions.pin')}
										class="size-7 rounded-md flex items-center justify-center transition-colors {pinned
											? 'bg-primary/10 text-primary'
											: 'text-muted-foreground/40 hover:text-muted-foreground'}"
									>
										<PinIcon class="size-3.5" />
									</button>
								</div>
							</div>

							<!-- Note -->
						{#if note}
							<div class="flex items-start gap-1.5 mb-1.5 text-xs text-muted-foreground">
								<NotebookPenIcon class="size-3 mt-0.5 flex-shrink-0" />
								<Hint text={note} class="line-clamp-1 italic">{note}</Hint>
							</div>
						{/if}

						<!-- Row 2: Status + Progress + Size -->
							<div class="flex items-center gap-2.5 mb-2">
								<span class="inline-flex items-center px-2 py-0.5 rounded-md text-[11px] font-medium flex-shrink-0 {statusPillClass(t.status)}">
									{$tt(STATUS_KEYS[t.status] ?? 'status.stopped')}
								</span>

								<div class="flex-1 flex items-center gap-2 min-w-0">
									<div class="flex-1 h-1.5 rounded-full bg-muted overflow-hidden">
										<div
											class="h-full rounded-full transition-[width] duration-700 ease-out {progressBarClass(t)}"
											class:animate-[progress-pulse_2s_ease-in-out_infinite]={isDownloading(t)}
											style="width: {t.percentDone * 100}%"
										></div>
									</div>
									<span class="text-xs text-muted-foreground tabular-nums w-8 text-right flex-shrink-0">
										{(t.percentDone * 100).toFixed(0)}%
									</span>
								</div>

								<div class="flex items-center gap-2 flex-shrink-0">
								<span class="text-xs text-muted-foreground tabular-nums">{formatSize(t.totalSize, $tt)}</span>
								{#if t.uploadedEver > 0}
									<Hint text={$tt('card.uploadedTotal')} class="text-xs text-primary tabular-nums">⬆ {formatSize(t.uploadedEver, $tt)}</Hint>
								{/if}
							</div>
							</div>

							<!-- Row 3: Speeds + ETA + Date | Actions -->
							<div class="flex items-center justify-between gap-2">
								<div class="flex items-center gap-2 text-xs text-muted-foreground tabular-nums min-w-0 overflow-hidden">
									{#if formatSpeed(t.rateDownload, $tt)}
										<Hint text={$tt('card.downloadSpeed')} class="text-primary">↓ {formatSpeed(t.rateDownload, $tt)}</Hint>
									{/if}
									{#if formatSpeed(t.rateUpload, $tt)}
										<Hint text={$tt('card.uploadSpeed')} class="text-primary">↑ {formatSpeed(t.rateUpload, $tt)}</Hint>
									{/if}
									{#if t.status !== 0 && t.status !== 6 && t.status !== 5 && formatEta(t.eta, $tt)}
										<span>{$tt('eta.prefix')} {formatEta(t.eta, $tt)}</span>
									{/if}
									{#if t.errorString}
										<span class="text-destructive truncate">{t.errorString}</span>
									{:else}
										<span>{formatDate(t.addedDate, $locale)}</span>
									{/if}
								</div>

								<!-- Action buttons: visible on hover (desktop), always on touch -->
								<div class="flex items-center gap-0.5 opacity-0 group-hover:opacity-100 transition-opacity touch-device:opacity-100">
									{#if t.status === 0}
										<button
											onclick={(e) => { e.stopPropagation(); handleStart(t); }}
											aria-label={$tt('actions.resume')}
											class="size-7 rounded-md flex items-center justify-center text-muted-foreground hover:text-foreground hover:bg-accent transition-colors"
										>
											<PlayIcon class="size-3.5" />
										</button>
									{:else if t.status === 4 || t.status === 3 || t.status === 6 || t.status === 5}
										<button
											onclick={(e) => { e.stopPropagation(); handleStop(t); }}
											aria-label={$tt('actions.pause')}
											class="size-7 rounded-md flex items-center justify-center text-muted-foreground hover:text-foreground hover:bg-accent transition-colors"
										>
											<PauseIcon class="size-3.5" />
										</button>
									{/if}
									<button
										onclick={(e) => { e.stopPropagation(); openDeleteDialog(t); }}
										aria-label={$tt('actions.delete')}
										class="size-7 rounded-md flex items-center justify-center text-muted-foreground hover:text-destructive hover:bg-destructive/10 transition-colors"
									>
										<Trash2Icon class="size-3.5" />
									</button>
								</div>
							</div>
						</div>
					{/if}
				{/each}
			</div>
		{/if}
	</div>
</div>

<!-- ── Scroll to top button ───────────────────────────────────────────────── -->
<button
	onclick={scrollToTop}
	aria-label={$tt('actions.scrollToTop')}
	class="fixed bottom-6 right-6 size-10 rounded-full bg-primary text-primary-foreground shadow-lg flex items-center justify-center transition-all duration-300 hover:opacity-90 hover:scale-105 active:scale-95 {showScrollTop ? 'opacity-100 translate-y-0 pointer-events-auto' : 'opacity-0 translate-y-4 pointer-events-none'}"
>
	<ArrowUpIcon class="size-4" />
</button>

<!-- ── Add Torrent Dialog ──────────────────────────────────────────────────── -->
<AlertDialog.Root bind:open={addOpen}>
	<AlertDialog.Content class="sm:max-w-md">
		<AlertDialog.Header class="pb-4">
			<AlertDialog.Title class="font-display text-lg font-semibold">{$tt('addDialog.title')}</AlertDialog.Title>
			<AlertDialog.Description class="text-sm text-muted-foreground">{$tt('addDialog.description')}</AlertDialog.Description>
		</AlertDialog.Header>

		<!-- Mode tabs (underline style) -->
		<div class="flex gap-4 border-b border-border/60 mb-4">
			<button
				class="relative flex items-center gap-1.5 pb-2.5 text-sm font-medium transition-colors {addMode === 'file'
					? 'text-foreground'
					: 'text-muted-foreground hover:text-foreground'}"
				onclick={() => (addMode = 'file')}
			>
				<UploadIcon class="size-3.5" />
				{$tt('addDialog.fileTab')}
				{#if addMode === 'file'}
					<span class="absolute bottom-0 left-0 right-0 h-0.5 bg-primary rounded-full"></span>
				{/if}
			</button>
			<button
				class="relative flex items-center gap-1.5 pb-2.5 text-sm font-medium transition-colors {addMode === 'magnet'
					? 'text-foreground'
					: 'text-muted-foreground hover:text-foreground'}"
				onclick={() => (addMode = 'magnet')}
			>
				<LinkIcon class="size-3.5" />
				{$tt('addDialog.magnetTab')}
				{#if addMode === 'magnet'}
					<span class="absolute bottom-0 left-0 right-0 h-0.5 bg-primary rounded-full"></span>
				{/if}
			</button>
		</div>

		{#if addMode === 'magnet'}
			<input
				type="text"
				placeholder={$tt('addDialog.magnetPlaceholder')}
				class="w-full h-10 rounded-lg border border-input bg-background px-3 text-sm outline-none transition-colors focus:border-primary/40 focus:ring-2 focus:ring-primary/10"
				bind:value={magnetUrl}
				onkeydown={(e) => e.key === 'Enter' && handleAdd()}
			/>
		{:else}
			<div class="flex flex-col gap-3">
				<input
					bind:this={fileInputEl}
					type="file"
					accept=".torrent,application/x-bittorrent"
					class="hidden"
					onchange={onFileChange}
				/>
					<button
					type="button"
					class="flex flex-col items-center justify-center gap-2 rounded-lg border-2 border-dashed p-6 cursor-pointer transition-colors w-full {dragOver
						? 'border-primary bg-primary/5'
						: 'border-border/60 hover:border-border hover:bg-accent/50'}"
					onclick={() => fileInputEl?.click()}
					ondragover={(e) => { e.preventDefault(); dragOver = true; }}
					ondragleave={() => (dragOver = false)}
					ondrop={onDrop}
				>
					<UploadIcon class="size-6 text-muted-foreground" />
					{#if pendingFile}
						<span class="text-sm font-medium">{pendingFile.name}</span>
					{:else}
						<span class="text-sm text-muted-foreground">{$tt('addDialog.dropHint')}</span>
					{/if}
				</button>
			</div>
		{/if}

		<!-- Destination folder -->
		<div class="flex flex-col gap-1.5 mt-4">
			<label class="text-sm font-medium flex items-center gap-1.5">
				<FolderIcon class="size-3.5 text-muted-foreground" />
				{$tt('addDialog.destinationFolder')}
			</label>
			<div class="relative">
				<div class="flex">
					<input
						type="text"
						class="flex-1 h-9 rounded-lg rounded-r-none border border-r-0 bg-background px-3 text-sm outline-none transition-colors font-mono {isDirValid
							? 'border-input focus:border-primary/40 focus:ring-2 focus:ring-primary/10'
							: 'border-destructive focus:border-destructive focus:ring-2 focus:ring-destructive/20'}"
						bind:value={downloadDir}
					/>
					<button
						type="button"
						class="h-9 px-2 rounded-lg rounded-l-none border border-input bg-background text-muted-foreground hover:text-foreground hover:bg-accent transition-colors"
						onclick={() => (showDirDropdown = !showDirDropdown)}
					>
						<ChevronDownIcon class="size-4" />
					</button>
				</div>
				{#if showDirDropdown && downloadDirStore.allDirs.length > 0}
					<div class="absolute z-50 top-full left-0 right-0 mt-1 rounded-lg border border-border bg-popover shadow-md py-1 max-h-40 overflow-y-auto">
						{#each downloadDirStore.allDirs as dir}
							<div class="flex items-center group/dir">
								<button
									type="button"
									class="flex-1 text-left px-3 py-1.5 text-sm font-mono truncate hover:bg-accent transition-colors {dir === downloadDir ? 'text-foreground font-medium' : 'text-muted-foreground'}"
									onclick={() => { downloadDir = dir; showDirDropdown = false; downloadDirStore.selectDir(dir); }}
								>
									{dir}
									{#if dir === downloadDirStore.defaultDir}
										<span class="ml-1.5 text-[10px] font-sans font-medium text-muted-foreground/60 uppercase">{$tt('addDialog.defaultPath')}</span>
									{/if}
								</button>
								{#if dir !== downloadDirStore.defaultDir}
									<button
										type="button"
										aria-label={$tt('addDialog.removePath')}
										class="size-6 flex items-center justify-center text-muted-foreground/40 hover:text-destructive transition-colors opacity-0 group-hover/dir:opacity-100 mr-1"
										onclick={() => downloadDirStore.removeDir(dir)}
									>
										<XIcon class="size-3" />
									</button>
								{/if}
							</div>
						{/each}
					</div>
				{/if}
			</div>
			{#if !isDirValid}
				<p class="text-xs text-destructive">{$tt('addDialog.invalidPath')}</p>
			{/if}
		</div>

		<!-- Size / free space -->
		{#if pendingFileSize !== null || destFreeSpace !== null}
			<div class="mt-3 flex flex-col gap-1.5 rounded-lg border border-border/60 bg-accent/30 px-3 py-2.5 text-sm">
				{#if pendingFileSize !== null}
					<div class="flex items-center justify-between gap-2">
						<span class="text-muted-foreground">{$tt('addDialog.torrentSize')}</span>
						<span class="font-medium tabular-nums">{formatSize(pendingFileSize, $tt)}</span>
					</div>
				{/if}
				{#if destFreeSpace !== null}
					<div class="flex items-center justify-between gap-2">
						<span class="text-muted-foreground">{$tt('addDialog.freeSpace')}</span>
						<span class="font-medium tabular-nums {notEnoughSpace ? 'text-destructive' : ''}">{formatSize(destFreeSpace, $tt)}</span>
					</div>
				{/if}
				{#if notEnoughSpace}
					<p class="flex items-center gap-1.5 text-xs text-destructive mt-0.5">
						<AlertCircleIcon class="size-3.5 flex-shrink-0" />
						{$tt('addDialog.notEnoughSpace')}
					</p>
				{/if}
			</div>
		{/if}

		<AlertDialog.Footer class="pt-4">
			<AlertDialog.Cancel disabled={isAdding} onclick={resetAddDialog}>{$tt('addDialog.cancel')}</AlertDialog.Cancel>
			<Button
				class="font-display font-semibold"
				onclick={handleAdd}
				disabled={isAdding || !isDirValid || notEnoughSpace || (addMode === 'magnet' ? !magnetUrl.trim() : !pendingFile)}
			>
				{#if isAdding}
					<Spinner class="size-4" />
				{/if}
				{$tt('addDialog.addButton')}
			</Button>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>

<!-- ── Delete Confirmation Dialog ────────────────────────────────────────── -->
<AlertDialog.Root bind:open={deleteOpen}>
	<AlertDialog.Content class="sm:max-w-md">
		<AlertDialog.Header class="pb-4">
			<AlertDialog.Title class="font-display text-lg font-semibold">{$tt('deleteDialog.title')}</AlertDialog.Title>
			<AlertDialog.Description>
				<span class="font-medium text-foreground">{deleteTarget?.name}</span>
				<br />
				{$tt('deleteDialog.cannotUndo')}
			</AlertDialog.Description>
		</AlertDialog.Header>

		<label class="flex items-center gap-2.5 text-sm cursor-pointer">
			<Checkbox bind:checked={deleteWithData} disabled={isDeleting} />
			{$tt('deleteDialog.deleteLocal')}
		</label>

		<AlertDialog.Footer class="pt-4">
			<Button variant="outline" disabled={isDeleting} onclick={() => { deleteOpen = false; }}>{$tt('deleteDialog.cancel')}</Button>
			<Button variant="destructive" class="font-display font-semibold" onclick={handleDelete} disabled={isDeleting}>
				{#if isDeleting}
					<Spinner class="size-4" />
				{/if}
				{$tt('deleteDialog.deleteButton')}
			</Button>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>

<!-- ── File Selection Dialog ────────────────────────────────────────────── -->
<FileSelectDialog
	bind:open={fileSelectOpen}
	torrentId={fileSelectTorrentId}
	torrentName={fileSelectTorrentName}
	onConfirm={handleFileSelectConfirm}
	onCancel={handleFileSelectCancel}
/>

<!-- ── Torrent Detail Panel ─────────────────────────────────────────────── -->
<TorrentDetailPanel bind:open={detailOpen} torrent={detailTorrent} />

<style>
	@media (hover: none) {
		.touch-device\:opacity-100 {
			opacity: 1 !important;
		}
		/* Always show action buttons on touch devices */
		:global(.group) .opacity-0 {
			opacity: 1 !important;
		}
	}
</style>
