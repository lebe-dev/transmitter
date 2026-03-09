<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { toast } from 'svelte-sonner';
	import { mode, toggleMode, setTheme, theme } from 'mode-watcher';
	import { getCoreRowModel, getSortedRowModel, type ColumnDef, type SortingState } from '@tanstack/table-core';
	import SunIcon from '@lucide/svelte/icons/sun';
	import MoonIcon from '@lucide/svelte/icons/moon';
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

	import { torrentStore } from '$lib/stores.svelte.js';
	import { addTorrentMagnet, addTorrentFile, startTorrents, stopTorrents, removeTorrents } from '$lib/api.js';
	import type { Torrent, FilterStatus } from '$lib/types.js';
	import { createSvelteTable } from '$lib/components/ui/data-table/index.js';
	import * as AlertDialog from '$lib/components/ui/alert-dialog/index.js';
	import * as Select from '$lib/components/ui/select/index.js';
	import { Spinner } from '$lib/components/ui/spinner/index.js';
	import { Button } from '$lib/components/ui/button/index.js';

	// ── Formatters ────────────────────────────────────────────────────────────

	function formatSize(bytes: number): string {
		if (bytes <= 0) return '0 B';
		const units = ['B', 'KB', 'MB', 'GB', 'TB'];
		const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
		return `${(bytes / 1024 ** i).toFixed(1)} ${units[i]}`;
	}

	function formatSpeed(bps: number): string {
		if (bps <= 0) return '';
		return `${formatSize(bps)}/s`;
	}

	function formatEta(secs: number): string {
		if (secs < 0) return '∞';
		if (secs === 0) return '';
		const h = Math.floor(secs / 3600);
		const m = Math.floor((secs % 3600) / 60);
		const s = secs % 60;
		if (h > 0) return `${h}h ${m}m`;
		if (m > 0) return `${m}m ${s}s`;
		return `${s}s`;
	}

	function formatDate(ts: number): string {
		return new Date(ts * 1000).toLocaleDateString(undefined, {
			month: 'short',
			day: 'numeric',
		});
	}

	// ── Status ────────────────────────────────────────────────────────────────

	const STATUS_LABEL: Record<number, string> = {
		0: 'Stopped',
		1: 'Check Queue',
		2: 'Checking',
		3: 'Queued',
		4: 'Downloading',
		5: 'Seed Queue',
		6: 'Seeding',
	};

	function statusPillClass(status: number): string {
		switch (status) {
			case 4: case 3: return 'bg-blue-500/10 text-blue-600 dark:text-blue-400';
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
		{ value: 'addedDate', label: 'Added' },
		{ value: 'name', label: 'Name' },
		{ value: 'totalSize', label: 'Size' },
		{ value: 'percentDone', label: 'Progress' },
		{ value: 'status', label: 'Status' },
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

	const FILTERS: { key: FilterStatus; label: string }[] = [
		{ key: 'all', label: 'All' },
		{ key: 'downloading', label: 'Downloading' },
		{ key: 'seeding', label: 'Seeding' },
		{ key: 'paused', label: 'Paused' },
		{ key: 'done', label: 'Done' },
	];

	// ── Add torrent dialog ────────────────────────────────────────────────────

	let addOpen = $state(false);
	let addMode = $state<'magnet' | 'file'>('magnet');
	let magnetUrl = $state('');
	let pendingFile = $state<File | null>(null);
	let fileInputEl = $state<HTMLInputElement | null>(null);
	let isAdding = $state(false);
	let dragOver = $state(false);

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
		if (isAdding) return;
		isAdding = true;
		try {
			if (addMode === 'magnet') {
				if (!magnetUrl.trim()) return;
				await addTorrentMagnet(magnetUrl.trim());
			} else {
				if (!pendingFile) return;
				const b64 = await readFileAsBase64(pendingFile);
				await addTorrentFile(b64);
			}
			toast.success('Torrent added');
			addOpen = false;
			magnetUrl = '';
			pendingFile = null;
			await torrentStore.refresh();
		} catch (err) {
			toast.error(err instanceof Error ? err.message : 'Failed to add torrent');
		} finally {
			isAdding = false;
		}
	}

	function resetAddDialog() {
		magnetUrl = '';
		pendingFile = null;
		addMode = 'magnet';
		isAdding = false;
		dragOver = false;
	}

	// ── Delete dialog ─────────────────────────────────────────────────────────

	let deleteOpen = $state(false);
	let deleteTarget = $state<Torrent | null>(null);
	let deleteWithData = $state(false);
	let isDeleting = $state(false);

	function openDeleteDialog(t: Torrent) {
		deleteTarget = t;
		deleteWithData = false;
		deleteOpen = true;
	}

	async function handleDelete() {
		if (!deleteTarget || isDeleting) return;
		isDeleting = true;
		try {
			await removeTorrents([deleteTarget.id], deleteWithData);
			toast.success(`Deleted: ${deleteTarget.name}`);
			deleteOpen = false;
			deleteTarget = null;
			await torrentStore.refresh();
		} catch (err) {
			toast.error(err instanceof Error ? err.message : 'Failed to delete torrent');
		} finally {
			isDeleting = false;
		}
	}

	// ── Torrent actions ───────────────────────────────────────────────────────

	async function handleStart(t: Torrent) {
		try {
			await startTorrents([t.id]);
			toast.success(`Started: ${t.name}`);
			await torrentStore.refresh();
		} catch {
			toast.error('Failed to start torrent');
		}
	}

	async function handleStop(t: Torrent) {
		try {
			await stopTorrents([t.id]);
			toast.success(`Paused: ${t.name}`);
			await torrentStore.refresh();
		} catch {
			toast.error('Failed to pause torrent');
		}
	}

	// ── Color theme ───────────────────────────────────────────────────────────

	const COLOR_THEMES = [
		{ value: 'yellow', label: 'Yellow' },
		{ value: 'blue', label: 'Blue' },
		{ value: 'green', label: 'Green' },
		{ value: 'default', label: 'Default' },
		{ value: 'orange', label: 'Orange' },
		{ value: 'red', label: 'Red' },
		{ value: 'rose', label: 'Rose' },
		{ value: 'violet', label: 'Violet' },
	] as const;

	// mode-watcher manages data-theme attr & localStorage ('mode-watcher-theme')
	// yellow = "" (default, no data-theme attr), others = theme name
	const toMwTheme = (t: string) => (t === 'yellow' ? '' : t);
	const fromMwTheme = (t: string) => (t || 'yellow');

	let colorTheme = $derived(fromMwTheme(theme.current));

	function onColorThemeChange(value: string) {
		if (value) setTheme(toMwTheme(value));
	}

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
		// migrate: old code stored 'yellow' literally, mode-watcher uses '' for no theme
		const stored = localStorage.getItem('transmitter-color-theme');
		if (stored === 'yellow') {
			localStorage.removeItem('transmitter-color-theme');
			setTheme('');
		}
		torrentStore.init();
		window.addEventListener('scroll', onScroll, { passive: true });
	});
	onDestroy(() => {
		torrentStore.destroy();
		window.removeEventListener('scroll', onScroll);
	});
</script>

<!-- ── Layout ──────────────────────────────────────────────────────────────── -->
<div class="min-h-screen bg-background text-foreground">

	<!-- Header -->
	<header class="border-b border-border/50">
		<div class="max-w-3xl mx-auto px-4 sm:px-6 h-14 flex items-center gap-3">
			<div class="flex items-center gap-2.5 mr-auto">
				<div class="size-7 rounded-lg bg-primary flex items-center justify-center flex-shrink-0">
					<span class="text-primary-foreground font-bold text-sm leading-none font-display">T</span>
				</div>
				<span class="font-display font-semibold text-[17px] tracking-tight">Transmitter</span>
			</div>

			<Select.Root type="single" value={colorTheme} onValueChange={onColorThemeChange}>
				<Select.Trigger class="h-8 w-24 text-xs" aria-label="Color theme">
					{COLOR_THEMES.find((t) => t.value === colorTheme)?.label ?? 'Theme'}
				</Select.Trigger>
				<Select.Portal>
					<Select.Content>
						{#each COLOR_THEMES as t}
							<Select.Item value={t.value} label={t.label} />
						{/each}
					</Select.Content>
				</Select.Portal>
			</Select.Root>

			<button
				onclick={toggleMode}
				aria-label="Toggle theme"
				class="size-8 rounded-lg flex items-center justify-center text-muted-foreground hover:text-foreground hover:bg-accent transition-colors"
			>
				{#if mode.current === 'dark'}
					<SunIcon class="size-4" />
				{:else}
					<MoonIcon class="size-4" />
				{/if}
			</button>

			<Button
				size="sm"
				class="font-display font-semibold"
				onclick={() => {
					resetAddDialog();
					addOpen = true;
				}}
			>
				<PlusIcon class="size-4" />
				Add
			</Button>
		</div>
	</header>

	<!-- Content -->
	<div class="max-w-3xl mx-auto px-4 sm:px-6 py-4 flex flex-col gap-4">

		<!-- Search -->
		<div class="relative">
			<SearchIcon class="absolute left-3 top-1/2 -translate-y-1/2 size-4 text-muted-foreground pointer-events-none" />
			<input
				type="search"
				placeholder="Search torrents…"
				class="w-full h-10 rounded-lg border border-input bg-background pl-9 pr-3 text-sm outline-none transition-colors focus:border-primary/40 focus:ring-2 focus:ring-primary/10"
				bind:value={torrentStore.search}
			/>
		</div>

		<!-- Filters + Sort -->
		<div class="flex items-center gap-1 overflow-x-auto -mx-4 px-4 sm:mx-0 sm:px-0">
			<div class="flex items-center gap-0.5">
				{#each FILTERS as f}
					<button
						class="relative px-3 py-1.5 text-sm font-medium transition-colors {torrentStore.filterStatus === f.key
							? 'text-foreground'
							: 'text-muted-foreground hover:text-foreground'}"
						onclick={() => (torrentStore.filterStatus = f.key)}
					>
						{f.label}
						<span class="ml-0.5 text-[11px] opacity-50 tabular-nums">{counts[f.key]}</span>
						{#if torrentStore.filterStatus === f.key}
							<span class="absolute bottom-0 left-3 right-3 h-0.5 bg-primary rounded-full"></span>
						{/if}
					</button>
				{/each}
			</div>

			<div class="ml-auto flex items-center gap-1 flex-shrink-0">
				<span class="text-xs text-muted-foreground">Sort:</span>
				<button
					class="flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground transition-colors px-1.5 py-1 rounded"
					onclick={() => {
						const idx = SORT_OPTIONS.findIndex((o) => o.value === sortField);
						const next = SORT_OPTIONS[(idx + 1) % SORT_OPTIONS.length];
						sortField = next.value;
					}}
				>
					{SORT_OPTIONS.find((o) => o.value === sortField)?.label}
				</button>
				<button
					class="text-xs text-muted-foreground hover:text-foreground transition-colors p-1 rounded"
					onclick={() => (sortDesc = !sortDesc)}
					aria-label="Toggle sort direction"
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
					<p class="font-medium text-destructive">Connection error</p>
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
					<h3 class="font-display font-semibold text-lg mb-1">No torrents yet</h3>
					<p class="text-sm text-muted-foreground mb-5 max-w-xs">Add a magnet link or upload a .torrent file to get started.</p>
					<Button
						size="sm"
						class="font-display font-semibold"
						onclick={() => {
							resetAddDialog();
							addOpen = true;
						}}
					>
						<PlusIcon class="size-4" />
						Add Torrent
					</Button>
				{:else}
					<h3 class="font-display font-semibold text-lg mb-1">No matches</h3>
					<p class="text-sm text-muted-foreground max-w-xs">Try a different search term or filter.</p>
				{/if}
			</div>

		<!-- Torrent cards -->
		{:else}
			<div class="flex flex-col gap-2">
				{#each table.getRowModel().rows as row, i (row.id)}
					{@const t = row.original}
					<div
						class="group rounded-lg border border-border/60 p-4 transition-all hover:shadow-sm hover:border-border {t.error ? 'border-l-2 border-l-destructive' : ''}"
						style="animation: card-enter 0.3s ease-out both; animation-delay: {Math.min(i, 10) * 30}ms"
					>
						<!-- Row 1: Name -->
						<div class="flex items-start justify-between gap-3 mb-2">
							<h3 class="font-display text-[15px] font-semibold leading-snug line-clamp-2 min-w-0">
								{t.name}
							</h3>
						</div>

						<!-- Row 2: Status + Progress + Size -->
						<div class="flex items-center gap-2.5 mb-2">
							<span class="inline-flex items-center px-2 py-0.5 rounded-md text-[11px] font-medium flex-shrink-0 {statusPillClass(t.status)}">
								{STATUS_LABEL[t.status] ?? t.status}
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

							<span class="text-xs text-muted-foreground tabular-nums flex-shrink-0">
								{formatSize(t.totalSize)}
							</span>
						</div>

						<!-- Row 3: Speeds + ETA + Date | Actions -->
						<div class="flex items-center justify-between gap-2">
							<div class="flex items-center gap-2 text-xs text-muted-foreground tabular-nums min-w-0 overflow-hidden">
								{#if formatSpeed(t.rateDownload)}
									<span class="text-blue-500 dark:text-blue-400">↓ {formatSpeed(t.rateDownload)}</span>
								{/if}
								{#if formatSpeed(t.rateUpload)}
									<span class="text-primary">↑ {formatSpeed(t.rateUpload)}</span>
								{/if}
								{#if t.status !== 0 && t.status !== 6 && t.status !== 5 && formatEta(t.eta)}
									<span>ETA {formatEta(t.eta)}</span>
								{/if}
								{#if t.errorString}
									<span class="text-destructive truncate">{t.errorString}</span>
								{:else}
									<span>{formatDate(t.addedDate)}</span>
								{/if}
							</div>

							<!-- Action buttons: visible on hover (desktop), always on touch -->
							<div class="flex items-center gap-0.5 opacity-0 group-hover:opacity-100 transition-opacity touch-device:opacity-100">
								{#if t.status === 0}
									<button
										onclick={() => handleStart(t)}
										aria-label="Resume"
										class="size-7 rounded-md flex items-center justify-center text-muted-foreground hover:text-foreground hover:bg-accent transition-colors"
									>
										<PlayIcon class="size-3.5" />
									</button>
								{:else if t.status === 4 || t.status === 3 || t.status === 6 || t.status === 5}
									<button
										onclick={() => handleStop(t)}
										aria-label="Pause"
										class="size-7 rounded-md flex items-center justify-center text-muted-foreground hover:text-foreground hover:bg-accent transition-colors"
									>
										<PauseIcon class="size-3.5" />
									</button>
								{/if}
								<button
									onclick={() => openDeleteDialog(t)}
									aria-label="Delete"
									class="size-7 rounded-md flex items-center justify-center text-muted-foreground hover:text-destructive hover:bg-destructive/10 transition-colors"
								>
									<Trash2Icon class="size-3.5" />
								</button>
							</div>
						</div>
					</div>
				{/each}
			</div>
		{/if}
	</div>
</div>

<!-- ── Scroll to top button ───────────────────────────────────────────────── -->
<button
	onclick={scrollToTop}
	aria-label="Scroll to top"
	class="fixed bottom-6 right-6 size-10 rounded-full bg-primary text-primary-foreground shadow-lg flex items-center justify-center transition-all duration-300 hover:opacity-90 hover:scale-105 active:scale-95 {showScrollTop ? 'opacity-100 translate-y-0 pointer-events-auto' : 'opacity-0 translate-y-4 pointer-events-none'}"
>
	<ArrowUpIcon class="size-4" />
</button>

<!-- ── Add Torrent Dialog ──────────────────────────────────────────────────── -->
<AlertDialog.Root bind:open={addOpen}>
	<AlertDialog.Content class="sm:max-w-md">
		<AlertDialog.Header class="pb-4">
			<AlertDialog.Title class="font-display text-lg font-semibold">Add Torrent</AlertDialog.Title>
			<AlertDialog.Description class="text-sm text-muted-foreground">Add by magnet link or upload a .torrent file.</AlertDialog.Description>
		</AlertDialog.Header>

		<!-- Mode tabs (underline style) -->
		<div class="flex gap-4 border-b border-border/60 mb-4">
			<button
				class="relative flex items-center gap-1.5 pb-2.5 text-sm font-medium transition-colors {addMode === 'magnet'
					? 'text-foreground'
					: 'text-muted-foreground hover:text-foreground'}"
				onclick={() => (addMode = 'magnet')}
			>
				<LinkIcon class="size-3.5" />
				Magnet / URL
				{#if addMode === 'magnet'}
					<span class="absolute bottom-0 left-0 right-0 h-0.5 bg-primary rounded-full"></span>
				{/if}
			</button>
			<button
				class="relative flex items-center gap-1.5 pb-2.5 text-sm font-medium transition-colors {addMode === 'file'
					? 'text-foreground'
					: 'text-muted-foreground hover:text-foreground'}"
				onclick={() => (addMode = 'file')}
			>
				<UploadIcon class="size-3.5" />
				.torrent File
				{#if addMode === 'file'}
					<span class="absolute bottom-0 left-0 right-0 h-0.5 bg-primary rounded-full"></span>
				{/if}
			</button>
		</div>

		{#if addMode === 'magnet'}
			<input
				type="text"
				placeholder="magnet:?xt=urn:btih:… or http://…"
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
						<span class="text-sm text-muted-foreground">Drop a .torrent file here or click to browse</span>
					{/if}
				</button>
			</div>
		{/if}

		<AlertDialog.Footer class="pt-4">
			<AlertDialog.Cancel disabled={isAdding} onclick={resetAddDialog}>Cancel</AlertDialog.Cancel>
			<Button
				class="font-display font-semibold"
				onclick={handleAdd}
				disabled={isAdding || (addMode === 'magnet' ? !magnetUrl.trim() : !pendingFile)}
			>
				{#if isAdding}
					<Spinner class="size-4" />
				{/if}
				Add
			</Button>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>

<!-- ── Delete Confirmation Dialog ────────────────────────────────────────── -->
<AlertDialog.Root bind:open={deleteOpen}>
	<AlertDialog.Content class="sm:max-w-md">
		<AlertDialog.Header class="pb-4">
			<AlertDialog.Title class="font-display text-lg font-semibold">Delete Torrent</AlertDialog.Title>
			<AlertDialog.Description>
				<span class="font-medium text-foreground">{deleteTarget?.name}</span>
				<br />
				This action cannot be undone.
			</AlertDialog.Description>
		</AlertDialog.Header>

		<label class="flex items-center gap-2.5 text-sm cursor-pointer">
			<input
				type="checkbox"
				class="rounded"
				bind:checked={deleteWithData}
			/>
			Also delete local data
		</label>

		<AlertDialog.Footer class="pt-4">
			<AlertDialog.Cancel disabled={isDeleting}>Cancel</AlertDialog.Cancel>
			<Button variant="destructive" class="font-display font-semibold" onclick={handleDelete} disabled={isDeleting}>
				{#if isDeleting}
					<Spinner class="size-4" />
				{/if}
				Delete
			</Button>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>

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
