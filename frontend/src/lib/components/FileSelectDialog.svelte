<script lang="ts">
	import { t as tt } from 'svelte-intl-precompile';
	import { get } from 'svelte/store';

	import * as AlertDialog from '$lib/components/ui/alert-dialog/index.js';
	import { Button } from '$lib/components/ui/button/index.js';
	import { Spinner } from '$lib/components/ui/spinner/index.js';
	import { getTorrentFiles } from '$lib/api.js';
	import { formatSize } from '$lib/format.js';
	import type { TorrentFile } from '$lib/types.js';

	let {
		open = $bindable(false),
		torrentId,
		torrentName,
		onConfirm,
		onCancel,
	}: {
		open: boolean;
		torrentId: number;
		torrentName: string;
		onConfirm: (wantedIndices: number[], unwantedIndices: number[]) => void;
		onCancel: () => void;
	} = $props();

	let files = $state<TorrentFile[]>([]);
	let selected = $state<boolean[]>([]);
	let loading = $state(false);
	let error = $state<string | null>(null);

	function basename(path: string): string {
		return path.split('/').pop() ?? path;
	}

	async function loadFiles() {
		loading = true;
		error = null;
		try {
			files = await getTorrentFiles(torrentId);
			selected = files.map(() => true);
		} catch (err) {
			error = err instanceof Error ? err.message : get(tt)('fileSelectDialog.errorLoad');
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		if (open && torrentId) {
			loadFiles();
		}
	});

	let selectedCount = $derived(selected.filter(Boolean).length);
	let selectedSize = $derived(
		files.reduce((sum, file, i) => (selected[i] ? sum + file.length : sum), 0),
	);
	let canConfirm = $derived(selectedCount > 0 && !loading);

	function selectAll() {
		selected = files.map(() => true);
	}

	function deselectAll() {
		selected = files.map(() => false);
	}

	function toggle(index: number) {
		selected = selected.map((v, i) => (i === index ? !v : v));
	}

	function handleConfirm() {
		const wanted: number[] = [];
		const unwanted: number[] = [];
		selected.forEach((sel, i) => {
			if (sel) wanted.push(i);
			else unwanted.push(i);
		});
		onConfirm(wanted, unwanted);
	}

	function handleCancel() {
		onCancel();
	}
</script>

<AlertDialog.Root bind:open onOpenChange={(v) => { if (!v) handleCancel(); }}>
	<AlertDialog.Content class="sm:max-w-lg">
		<AlertDialog.Header>
			<AlertDialog.Title>{get(tt)('fileSelectDialog.title')}</AlertDialog.Title>
			<AlertDialog.Description>
				<span class="block truncate" title={torrentName}>{torrentName}</span>
			</AlertDialog.Description>
		</AlertDialog.Header>

		{#if loading}
			<div class="flex items-center justify-center py-8">
				<Spinner class="size-6" />
			</div>
		{:else if error}
			<p class="text-sm text-destructive py-4">{error}</p>
		{:else}
			<div class="flex items-center gap-2 mb-2">
				<Button variant="outline" size="sm" onclick={selectAll}>
					{get(tt)('fileSelectDialog.selectAll')}
				</Button>
				<Button variant="outline" size="sm" onclick={deselectAll}>
					{get(tt)('fileSelectDialog.deselectAll')}
				</Button>
			</div>

			<p class="text-sm text-muted-foreground mb-2">
				{$tt('fileSelectDialog.selectedInfo', { values: { count: selectedCount, total: files.length, size: formatSize(selectedSize, $tt) } })}
			</p>

			<div class="max-h-64 overflow-y-auto flex flex-col gap-0.5 rounded-md border border-border/60 p-1">
				{#each files as file, i}
					<label
						class="flex items-center gap-2 px-2 py-1.5 rounded hover:bg-accent/50 cursor-pointer"
					>
						<input
							type="checkbox"
							checked={selected[i]}
							onchange={() => toggle(i)}
							class="size-3.5 accent-primary flex-shrink-0 cursor-pointer"
						/>
						<span class="text-sm truncate min-w-0" title={file.name}>
							{basename(file.name)}
						</span>
						<span class="text-xs text-muted-foreground ml-auto flex-shrink-0">
							{formatSize(file.length, $tt)}
						</span>
					</label>
				{/each}
			</div>
		{/if}

		<AlertDialog.Footer>
			<AlertDialog.Cancel onclick={handleCancel}>
				{get(tt)('fileSelectDialog.cancel')}
			</AlertDialog.Cancel>
			<AlertDialog.Action disabled={!canConfirm} onclick={handleConfirm}>
				{get(tt)('fileSelectDialog.downloadButton')}
			</AlertDialog.Action>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>
