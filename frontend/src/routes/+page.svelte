<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { toast } from 'svelte-sonner';
	import { mode, toggleMode } from 'mode-watcher';
	import { getCoreRowModel, getSortedRowModel, type ColumnDef, type SortingState } from '@tanstack/table-core';
	import SunIcon from '@lucide/svelte/icons/sun';
	import MoonIcon from '@lucide/svelte/icons/moon';
	import PlusIcon from '@lucide/svelte/icons/plus';
	import PlayIcon from '@lucide/svelte/icons/play';
	import PauseIcon from '@lucide/svelte/icons/pause';
	import Trash2Icon from '@lucide/svelte/icons/trash-2';
	import ArrowUpIcon from '@lucide/svelte/icons/arrow-up';
	import ArrowDownIcon from '@lucide/svelte/icons/arrow-down';
	import ChevronsUpDownIcon from '@lucide/svelte/icons/chevrons-up-down';
	import UploadIcon from '@lucide/svelte/icons/upload';
	import LinkIcon from '@lucide/svelte/icons/link';

	import { torrentStore } from '$lib/stores.svelte.js';
	import { addTorrentMagnet, addTorrentFile, startTorrents, stopTorrents, removeTorrents } from '$lib/api.js';
	import type { Torrent, FilterStatus } from '$lib/types.js';
	import { createSvelteTable, FlexRender } from '$lib/components/ui/data-table/index.js';
	import * as Table from '$lib/components/ui/table/index.js';
	import * as AlertDialog from '$lib/components/ui/alert-dialog/index.js';
	import * as Tooltip from '$lib/components/ui/tooltip/index.js';
	import { Progress } from '$lib/components/ui/progress/index.js';
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
			year: 'numeric',
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

	const STATUS_CLASS: Record<number, string> = {
		0: 'text-muted-foreground',
		1: 'text-yellow-500',
		2: 'text-yellow-500',
		3: 'text-blue-500',
		4: 'text-blue-500',
		5: 'text-green-500',
		6: 'text-green-500',
	};

	// ── Table ─────────────────────────────────────────────────────────────────

	let sorting = $state<SortingState>([{ id: 'addedDate', desc: true }]);

	const columns: ColumnDef<Torrent>[] = [
		{ accessorKey: 'name', header: 'Name' },
		{ accessorKey: 'status', header: 'Status' },
		{ accessorKey: 'percentDone', header: 'Progress' },
		{ accessorKey: 'totalSize', header: 'Size' },
		{ accessorKey: 'rateDownload', header: '↓' },
		{ accessorKey: 'rateUpload', header: '↑' },
		{ accessorKey: 'eta', header: 'ETA' },
		{ accessorKey: 'addedDate', header: 'Added' },
		{ id: 'actions', header: '', enableSorting: false },
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

	// ── Lifecycle ─────────────────────────────────────────────────────────────

	onMount(() => torrentStore.init());
	onDestroy(() => torrentStore.destroy());
</script>

<!-- ── Layout ──────────────────────────────────────────────────────────────── -->
<div class="min-h-screen bg-background text-foreground flex flex-col">

	<!-- Header -->
	<header class="border-b border-border px-4 py-3 flex items-center justify-between gap-4">
		<h1 class="text-lg font-semibold tracking-tight">Transmitter</h1>
		<Button variant="ghost" size="icon" onclick={toggleMode} aria-label="Toggle theme">
			{#if mode.current === 'dark'}
				<SunIcon class="size-4" />
			{:else}
				<MoonIcon class="size-4" />
			{/if}
		</Button>
	</header>

	<!-- Error banner -->
	{#if torrentStore.error}
		<div class="bg-destructive/10 border-b border-destructive/20 px-4 py-2 text-sm text-destructive">
			Connection error: {torrentStore.error}
		</div>
	{/if}

	<!-- Toolbar -->
	<div class="border-b border-border px-4 py-2 flex flex-wrap items-center gap-3">
		<!-- Search -->
		<input
			type="search"
			placeholder="Search torrents…"
			class="h-8 rounded-md border border-input bg-background px-3 text-sm outline-none focus:ring-2 focus:ring-ring/50 w-48"
			bind:value={torrentStore.search}
		/>

		<!-- Filter tabs -->
		<div class="flex items-center gap-1 flex-wrap">
			{#each FILTERS as f}
				<button
					class="h-8 rounded-md px-3 text-sm font-medium transition-colors {torrentStore.filterStatus === f.key
						? 'bg-primary text-primary-foreground'
						: 'hover:bg-accent hover:text-accent-foreground text-muted-foreground'}"
					onclick={() => (torrentStore.filterStatus = f.key)}
				>
					{f.label}
					<span class="ml-1 text-xs opacity-70">{counts[f.key]}</span>
				</button>
			{/each}
		</div>

		<div class="ml-auto">
			<Button
				size="sm"
				onclick={() => {
					resetAddDialog();
					addOpen = true;
				}}
			>
				<PlusIcon class="size-4" />
				Add Torrent
			</Button>
		</div>
	</div>

	<!-- Table -->
	<div class="flex-1 overflow-auto">
		{#if torrentStore.loading}
			<div class="flex items-center justify-center h-48 gap-2 text-muted-foreground">
				<Spinner class="size-5" />
				<span class="text-sm">Connecting…</span>
			</div>
		{:else if torrentStore.filtered.length === 0}
			<div class="flex items-center justify-center h-48 text-muted-foreground text-sm">
				{torrentStore.torrents.length === 0 ? 'No torrents' : 'No matching torrents'}
			</div>
		{:else}
			<Table.Root>
				<Table.Header>
					{#each table.getHeaderGroups() as headerGroup}
						<Table.Row>
							{#each headerGroup.headers as header}
								<Table.Head
									class="{header.column.id === 'name'
										? 'min-w-[200px]'
										: header.column.id === 'actions'
											? 'w-24'
											: header.column.id === 'percentDone'
												? 'w-32'
												: 'w-24'} {header.column.getCanSort() ? 'cursor-pointer select-none' : ''}"
									onclick={header.column.getCanSort()
										? () => header.column.toggleSorting()
										: undefined}
								>
									<div class="flex items-center gap-1">
										<FlexRender
											content={header.column.columnDef.header}
											context={header.getContext()}
										/>
										{#if header.column.getCanSort()}
											{#if header.column.getIsSorted() === 'asc'}
												<ArrowUpIcon class="size-3 opacity-70" />
											{:else if header.column.getIsSorted() === 'desc'}
												<ArrowDownIcon class="size-3 opacity-70" />
											{:else}
												<ChevronsUpDownIcon class="size-3 opacity-30" />
											{/if}
										{/if}
									</div>
								</Table.Head>
							{/each}
						</Table.Row>
					{/each}
				</Table.Header>

				<Table.Body>
					{#each table.getRowModel().rows as row (row.id)}
						{@const t = row.original}
						<Table.Row class={t.error ? 'bg-destructive/5' : ''}>
							{#each row.getVisibleCells() as cell (cell.id)}
								<Table.Cell>
									{#if cell.column.id === 'name'}
										<Tooltip.Provider>
											<Tooltip.Root>
												<Tooltip.Trigger class="text-left max-w-[280px] truncate block">
													{t.name}
												</Tooltip.Trigger>
												<Tooltip.Portal>
													<Tooltip.Content class="max-w-xs break-all text-xs">
														{t.name}
														{#if t.errorString}
															<div class="mt-1 text-destructive">{t.errorString}</div>
														{/if}
													</Tooltip.Content>
												</Tooltip.Portal>
											</Tooltip.Root>
										</Tooltip.Provider>

									{:else if cell.column.id === 'status'}
										<span class="text-xs font-medium {STATUS_CLASS[t.status] ?? ''}">
											{STATUS_LABEL[t.status] ?? t.status}
										</span>

									{:else if cell.column.id === 'percentDone'}
										<div class="flex items-center gap-2">
											<Progress value={t.percentDone * 100} class="h-1.5 w-20" />
											<span class="text-xs text-muted-foreground w-9 text-right">
												{(t.percentDone * 100).toFixed(0)}%
											</span>
										</div>

									{:else if cell.column.id === 'totalSize'}
										<span class="text-xs text-muted-foreground">{formatSize(t.totalSize)}</span>

									{:else if cell.column.id === 'rateDownload'}
										<span class="text-xs text-blue-500">{formatSpeed(t.rateDownload)}</span>

									{:else if cell.column.id === 'rateUpload'}
										<span class="text-xs text-green-500">{formatSpeed(t.rateUpload)}</span>

									{:else if cell.column.id === 'eta'}
										<span class="text-xs text-muted-foreground">{formatEta(t.eta)}</span>

									{:else if cell.column.id === 'addedDate'}
										<span class="text-xs text-muted-foreground">{formatDate(t.addedDate)}</span>

									{:else if cell.column.id === 'actions'}
										<div class="flex items-center gap-1">
											{#if t.status === 0}
												<Button
													variant="ghost"
													size="icon-sm"
													onclick={() => handleStart(t)}
													aria-label="Resume"
												>
													<PlayIcon class="size-3.5" />
												</Button>
											{:else if t.status === 4 || t.status === 3 || t.status === 6 || t.status === 5}
												<Button
													variant="ghost"
													size="icon-sm"
													onclick={() => handleStop(t)}
													aria-label="Pause"
												>
													<PauseIcon class="size-3.5" />
												</Button>
											{/if}
											<Button
												variant="ghost"
												size="icon-sm"
												class="text-destructive hover:text-destructive"
												onclick={() => openDeleteDialog(t)}
												aria-label="Delete"
											>
												<Trash2Icon class="size-3.5" />
											</Button>
										</div>
									{/if}
								</Table.Cell>
							{/each}
						</Table.Row>
					{/each}
				</Table.Body>
			</Table.Root>
		{/if}
	</div>
</div>

<!-- ── Add Torrent Dialog ──────────────────────────────────────────────────── -->
<AlertDialog.Root bind:open={addOpen}>
	<AlertDialog.Content>
		<AlertDialog.Header>
			<AlertDialog.Title>Add Torrent</AlertDialog.Title>
			<AlertDialog.Description>Add by magnet link or upload a .torrent file.</AlertDialog.Description>
		</AlertDialog.Header>

		<!-- Mode tabs -->
		<div class="flex gap-2 mt-2">
			<button
				class="flex items-center gap-1.5 px-3 py-1.5 rounded-md text-sm font-medium transition-colors {addMode === 'magnet'
					? 'bg-primary text-primary-foreground'
					: 'hover:bg-accent text-muted-foreground'}"
				onclick={() => (addMode = 'magnet')}
			>
				<LinkIcon class="size-3.5" />
				Magnet / URL
			</button>
			<button
				class="flex items-center gap-1.5 px-3 py-1.5 rounded-md text-sm font-medium transition-colors {addMode === 'file'
					? 'bg-primary text-primary-foreground'
					: 'hover:bg-accent text-muted-foreground'}"
				onclick={() => (addMode = 'file')}
			>
				<UploadIcon class="size-3.5" />
				.torrent File
			</button>
		</div>

		{#if addMode === 'magnet'}
			<input
				type="text"
				placeholder="magnet:?xt=urn:btih:… or http://…"
				class="w-full h-9 rounded-md border border-input bg-background px-3 text-sm outline-none focus:ring-2 focus:ring-ring/50"
				bind:value={magnetUrl}
				onkeydown={(e) => e.key === 'Enter' && handleAdd()}
			/>
		{:else}
			<div class="flex flex-col gap-2">
				<input
					bind:this={fileInputEl}
					type="file"
					accept=".torrent,application/x-bittorrent"
					class="hidden"
					onchange={onFileChange}
				/>
				<Button variant="outline" onclick={() => fileInputEl?.click()}>
					<UploadIcon class="size-4" />
					{pendingFile ? pendingFile.name : 'Choose .torrent file'}
				</Button>
			</div>
		{/if}

		<AlertDialog.Footer>
			<AlertDialog.Cancel disabled={isAdding} onclick={resetAddDialog}>Cancel</AlertDialog.Cancel>
			<Button
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
	<AlertDialog.Content>
		<AlertDialog.Header>
			<AlertDialog.Title>Delete Torrent</AlertDialog.Title>
			<AlertDialog.Description>
				<span class="font-medium text-foreground">{deleteTarget?.name}</span>
				<br />
				This action cannot be undone.
			</AlertDialog.Description>
		</AlertDialog.Header>

		<label class="flex items-center gap-2 text-sm cursor-pointer">
			<input
				type="checkbox"
				class="rounded"
				bind:checked={deleteWithData}
			/>
			Also delete local data
		</label>

		<AlertDialog.Footer>
			<AlertDialog.Cancel disabled={isDeleting}>Cancel</AlertDialog.Cancel>
			<Button variant="destructive" onclick={handleDelete} disabled={isDeleting}>
				{#if isDeleting}
					<Spinner class="size-4" />
				{/if}
				Delete
			</Button>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>
