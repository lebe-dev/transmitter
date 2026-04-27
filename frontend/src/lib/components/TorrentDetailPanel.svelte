<script lang="ts">
	import { t as tt } from 'svelte-intl-precompile';
	import XIcon from '@lucide/svelte/icons/x';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import LockIcon from '@lucide/svelte/icons/lock';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';
	import ChevronRightIcon from '@lucide/svelte/icons/chevron-right';
	import ChevronDownIcon from '@lucide/svelte/icons/chevron-down';
	import FolderIcon from '@lucide/svelte/icons/folder';
	import FolderOpenIcon from '@lucide/svelte/icons/folder-open';
	import FileIcon from '@lucide/svelte/icons/file';

	import * as Sheet from '$lib/components/ui/sheet/index.js';
	import * as Tabs from '$lib/components/ui/tabs/index.js';
	import { Button } from '$lib/components/ui/button/index.js';
	import { Spinner } from '$lib/components/ui/spinner/index.js';
	import { getTorrentDetails, setFilesWanted, setFilePriority } from '$lib/api.js';
	import type { Torrent, TorrentDetail, TorrentFile, FilePriority } from '$lib/types.js';

	let {
		torrent,
		open = $bindable(false),
	}: {
		torrent: Torrent | null;
		open: boolean;
	} = $props();

	type FileLeaf = { kind: 'file'; index: number; name: string; path: string };
	type DirNode = { kind: 'dir'; name: string; path: string; children: TreeNode[] };
	type TreeNode = FileLeaf | DirNode;
	type DirAgg = {
		size: number;
		done: number;
		wanted: 'all' | 'none' | 'mixed';
		fileIndices: number[];
	};

	let detail = $state<TorrentDetail | null>(null);
	let loading = $state(false);
	let error = $state<string | null>(null);
	let activeTab = $state('files');
	let pendingKey = $state<string | null>(null);
	let expanded = $state<Set<string>>(new Set());

	const isBusy = $derived(pendingKey !== null);

	async function fetchDetails() {
		if (!torrent) return;
		loading = true;
		error = null;
		detail = null;
		try {
			detail = await getTorrentDetails(torrent.id);
		} catch (err) {
			error = err instanceof Error ? err.message : 'Unknown error';
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		if (open && torrent) {
			fetchDetails();
		}
		if (!open) {
			detail = null;
			error = null;
			activeTab = 'files';
			expanded = new Set();
		}
	});

	const tree = $derived.by<TreeNode[]>(() => (detail ? buildTree(detail.files) : []));

	$effect(() => {
		if (!detail) return;
		const next = new Set<string>();
		collectDirPaths(tree, next);
		expanded = next;
	});

	const dirAggs = $derived.by(() => {
		const map = new Map<string, DirAgg>();
		if (!detail) return map;
		const files = detail.files;
		const stats = detail.fileStats;
		const visit = (node: TreeNode): {
			size: number;
			done: number;
			allWanted: boolean;
			noneWanted: boolean;
			indices: number[];
		} => {
			if (node.kind === 'file') {
				const file = files[node.index];
				const wanted = stats[node.index]?.wanted !== false;
				return {
					size: file.length,
					done: file.bytesCompleted,
					allWanted: wanted,
					noneWanted: !wanted,
					indices: [node.index],
				};
			}
			let size = 0;
			let done = 0;
			let allWanted = true;
			let noneWanted = true;
			const indices: number[] = [];
			for (const child of node.children) {
				const r = visit(child);
				size += r.size;
				done += r.done;
				allWanted = allWanted && r.allWanted;
				noneWanted = noneWanted && r.noneWanted;
				indices.push(...r.indices);
			}
			map.set(node.path, {
				size,
				done,
				wanted: allWanted ? 'all' : noneWanted ? 'none' : 'mixed',
				fileIndices: indices,
			});
			return { size, done, allWanted, noneWanted, indices };
		};
		for (const root of tree) visit(root);
		return map;
	});

	function buildTree(files: TorrentFile[]): TreeNode[] {
		const root: DirNode = { kind: 'dir', name: '', path: '', children: [] };
		const dirMap = new Map<string, DirNode>();
		dirMap.set('', root);

		files.forEach((file, index) => {
			const parts = file.name.split('/').filter((p) => p.length > 0);
			if (parts.length === 0) return;
			const fileName = parts[parts.length - 1];
			let parent = root;
			let currentPath = '';
			for (let i = 0; i < parts.length - 1; i++) {
				const part = parts[i];
				currentPath = currentPath ? `${currentPath}/${part}` : part;
				let dir = dirMap.get(currentPath);
				if (!dir) {
					dir = { kind: 'dir', name: part, path: currentPath, children: [] };
					dirMap.set(currentPath, dir);
					parent.children.push(dir);
				}
				parent = dir;
			}
			parent.children.push({
				kind: 'file',
				index,
				name: fileName,
				path: file.name,
			});
		});

		const sortRec = (node: DirNode) => {
			node.children.sort((a, b) => {
				if (a.kind !== b.kind) return a.kind === 'dir' ? -1 : 1;
				return a.name.localeCompare(b.name);
			});
			for (const c of node.children) if (c.kind === 'dir') sortRec(c);
		};
		sortRec(root);
		return root.children;
	}

	function collectDirPaths(nodes: TreeNode[], acc: Set<string>) {
		for (const n of nodes) {
			if (n.kind === 'dir') {
				acc.add(n.path);
				collectDirPaths(n.children, acc);
			}
		}
	}

	function toggleExpand(path: string) {
		const next = new Set(expanded);
		if (next.has(path)) next.delete(path);
		else next.add(path);
		expanded = next;
	}

	function formatSize(bytes: number): string {
		if (bytes <= 0) return '0 B';
		const units = ['B', 'KB', 'MB', 'GB', 'TB'];
		const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
		return `${(bytes / 1024 ** i).toFixed(1)} ${units[i]}`;
	}

	function formatSpeed(bps: number): string {
		if (bps <= 0) return '—';
		return `${formatSize(bps)}/s`;
	}

	function priorityLabel(p: FilePriority): string {
		switch (p) {
			case -1: return $tt('detail.priorityLow');
			case 0: return $tt('detail.priorityNormal');
			case 1: return $tt('detail.priorityHigh');
			default: return '';
		}
	}

	function nextPriority(p: FilePriority): { value: FilePriority; level: 'low' | 'normal' | 'high' } {
		switch (p) {
			case -1: return { value: 0, level: 'normal' };
			case 0: return { value: 1, level: 'high' };
			case 1: return { value: -1, level: 'low' };
			default: return { value: 0, level: 'normal' };
		}
	}

	async function toggleFileWanted(fileIndex: number, currentlyWanted: boolean) {
		if (!torrent || !detail || isBusy) return;
		const key = `f:${fileIndex}`;
		pendingKey = key;
		const prev = detail.fileStats[fileIndex].wanted;
		detail.fileStats[fileIndex].wanted = !currentlyWanted;
		try {
			if (currentlyWanted) await setFilesWanted(torrent.id, [], [fileIndex]);
			else await setFilesWanted(torrent.id, [fileIndex], []);
		} catch {
			detail.fileStats[fileIndex].wanted = prev;
		} finally {
			pendingKey = null;
		}
	}

	async function toggleDirWanted(path: string) {
		if (!torrent || !detail || isBusy) return;
		const agg = dirAggs.get(path);
		if (!agg) return;
		const targetWanted = agg.wanted !== 'all';
		const indices = agg.fileIndices;
		const prev = indices.map((i) => detail!.fileStats[i].wanted);
		pendingKey = `d:${path}`;
		for (const i of indices) detail.fileStats[i].wanted = targetWanted;
		try {
			await setFilesWanted(
				torrent.id,
				targetWanted ? indices : [],
				targetWanted ? [] : indices,
			);
		} catch {
			indices.forEach((i, k) => (detail!.fileStats[i].wanted = prev[k]));
		} finally {
			pendingKey = null;
		}
	}

	async function setAllWanted(wanted: boolean) {
		if (!torrent || !detail || isBusy) return;
		const indices = detail.files.map((_, i) => i);
		const prev = detail.fileStats.map((s) => s.wanted);
		pendingKey = 'all';
		detail.fileStats.forEach((s) => (s.wanted = wanted));
		try {
			await setFilesWanted(torrent.id, wanted ? indices : [], wanted ? [] : indices);
		} catch {
			detail.fileStats.forEach((s, i) => (s.wanted = prev[i]));
		} finally {
			pendingKey = null;
		}
	}

	async function cyclePriority(fileIndex: number, current: FilePriority) {
		if (!torrent || !detail || isBusy) return;
		pendingKey = `p:${fileIndex}`;
		const next = nextPriority(current);
		const prev = detail.fileStats[fileIndex].priority;
		detail.fileStats[fileIndex].priority = next.value;
		try {
			await setFilePriority(torrent.id, fileIndex, next.level);
		} catch {
			detail.fileStats[fileIndex].priority = prev;
		} finally {
			pendingKey = null;
		}
	}

	function trackerStatus(state: number): string {
		switch (state) {
			case 0: return 'Inactive';
			case 1: return 'Waiting';
			case 2: return 'Queued';
			case 3: return 'Active';
			default: return `${state}`;
		}
	}

	function indeterminateAttach(value: boolean) {
		return (el: HTMLInputElement) => {
			el.indeterminate = value;
		};
	}
</script>

{#snippet renderNode(node: TreeNode, depth: number)}
	{#if node.kind === 'dir'}
		{@const agg = dirAggs.get(node.path)}
		{@const isExpanded = expanded.has(node.path)}
		{@const wantedAll = agg?.wanted === 'all'}
		{@const wantedMixed = agg?.wanted === 'mixed'}
		{@const progress = agg && agg.size > 0 ? agg.done / agg.size : 0}
		<div
			class="flex items-center gap-2 rounded-md px-2 py-1.5 hover:bg-accent/50 {wantedAll || wantedMixed ? '' : 'opacity-50'}"
			style="padding-left: {depth * 14 + 8}px"
		>
			<input
				type="checkbox"
				checked={wantedAll}
				{@attach indeterminateAttach(wantedMixed)}
				disabled={isBusy}
				onchange={() => toggleDirWanted(node.path)}
				class="size-3.5 accent-primary flex-shrink-0 cursor-pointer"
			/>
			<button
				type="button"
				class="flex flex-1 min-w-0 items-center gap-1.5 cursor-pointer text-left"
				onclick={() => toggleExpand(node.path)}
			>
				{#if isExpanded}
					<ChevronDownIcon class="size-3.5 text-muted-foreground flex-shrink-0" />
				{:else}
					<ChevronRightIcon class="size-3.5 text-muted-foreground flex-shrink-0" />
				{/if}
				{#if isExpanded}
					<FolderOpenIcon class="size-3.5 text-muted-foreground flex-shrink-0" />
				{:else}
					<FolderIcon class="size-3.5 text-muted-foreground flex-shrink-0" />
				{/if}
				<span class="text-sm truncate">{node.name}</span>
				<span class="text-[11px] text-muted-foreground tabular-nums flex-shrink-0 ml-auto">
					{(progress * 100).toFixed(0)}%
				</span>
				<span class="text-xs text-muted-foreground tabular-nums flex-shrink-0">
					{agg ? formatSize(agg.size) : ''}
				</span>
			</button>
		</div>
		{#if isExpanded}
			{#each node.children as child (child.kind === 'dir' ? `d:${child.path}` : `f:${child.index}`)}
				{@render renderNode(child, depth + 1)}
			{/each}
		{/if}
	{:else}
		{@const file = detail!.files[node.index]}
		{@const stat = detail!.fileStats[node.index]}
		{@const progress = file.length > 0 ? file.bytesCompleted / file.length : 0}
		<div
			class="rounded-md px-2 py-1.5 hover:bg-accent/30 transition-opacity {stat?.wanted === false ? 'opacity-50' : ''}"
			style="padding-left: {depth * 14 + 8}px"
		>
			<div class="flex items-center gap-2 mb-1">
				<input
					type="checkbox"
					checked={stat?.wanted !== false}
					disabled={isBusy}
					onchange={() => toggleFileWanted(node.index, stat?.wanted !== false)}
					class="size-3.5 accent-primary flex-shrink-0 cursor-pointer"
				/>
				<FileIcon class="size-3.5 text-muted-foreground flex-shrink-0" />
				<span class="text-sm break-all leading-snug min-w-0 flex-1" title={file.name}>
					{node.name}
				</span>
				{#if stat}
					<button
						onclick={() => cyclePriority(node.index, stat.priority)}
						disabled={isBusy}
						title={$tt('detail.cyclePriority')}
						class="text-[10px] font-medium px-1.5 py-0.5 rounded cursor-pointer transition-colors flex-shrink-0
							{stat.priority === 1 ? 'bg-primary/15 text-primary' : ''}
							{stat.priority === 0 ? 'bg-muted text-muted-foreground' : ''}
							{stat.priority === -1 ? 'bg-muted/50 text-muted-foreground/60' : ''}"
					>
						{priorityLabel(stat.priority)}
					</button>
				{/if}
				<span class="text-xs text-muted-foreground tabular-nums flex-shrink-0">
					{formatSize(file.length)}
				</span>
			</div>
			<div class="flex items-center gap-2 pl-[22px]">
				<div class="flex-1 h-1 rounded-full bg-muted overflow-hidden">
					<div
						class="h-full rounded-full bg-primary transition-[width] duration-300"
						style="width: {progress * 100}%"
					></div>
				</div>
				<span class="text-[11px] text-muted-foreground tabular-nums w-8 text-right">
					{(progress * 100).toFixed(0)}%
				</span>
			</div>
		</div>
	{/if}
{/snippet}

<Sheet.Root bind:open>
	<Sheet.Content>
		<Sheet.Header>
			<div class="flex items-start justify-between gap-3">
				<Sheet.Title class="font-display line-clamp-2 min-w-0 pr-2">
					{torrent?.name ?? ''}
				</Sheet.Title>
				<Sheet.Close>
					<XIcon class="size-4" />
				</Sheet.Close>
			</div>
			<Sheet.Description class="sr-only">
				{$tt('detail.files')}, {$tt('detail.peers')}, {$tt('detail.trackers')}
			</Sheet.Description>
		</Sheet.Header>

		<div class="flex-1 overflow-y-auto p-4">
			{#if loading}
				<div class="flex items-center justify-center py-12">
					<Spinner class="size-6" />
				</div>
			{:else if error}
				<div class="flex flex-col items-center justify-center py-12 gap-3 text-center">
					<AlertCircleIcon class="size-8 text-destructive" />
					<p class="text-sm text-muted-foreground">{error}</p>
					<Button variant="outline" size="sm" onclick={fetchDetails}>
						<RefreshCwIcon class="size-3.5" />
						{$tt('detail.retry')}
					</Button>
				</div>
			{:else if detail}
				<Tabs.Root bind:value={activeTab}>
					<Tabs.List>
						<Tabs.Trigger value="files">
							{$tt('detail.files')}
							<span class="ml-1 text-[11px] opacity-50 tabular-nums">{detail.files.length}</span>
						</Tabs.Trigger>
						<Tabs.Trigger value="peers">
							{$tt('detail.peers')}
							<span class="ml-1 text-[11px] opacity-50 tabular-nums">{detail.peers.length}</span>
						</Tabs.Trigger>
						<Tabs.Trigger value="trackers">
							{$tt('detail.trackers')}
							<span class="ml-1 text-[11px] opacity-50 tabular-nums">{detail.trackerStats.length}</span>
						</Tabs.Trigger>
					</Tabs.List>

					<!-- Files tab -->
					<Tabs.Content value="files">
						{#if detail.files.length === 0}
							<p class="text-sm text-muted-foreground py-6 text-center">{$tt('detail.noFiles')}</p>
						{:else}
							<div class="flex items-center gap-3 mb-2 text-xs">
								<button
									type="button"
									class="text-primary hover:underline disabled:opacity-50 cursor-pointer"
									disabled={isBusy}
									onclick={() => setAllWanted(true)}
								>
									{$tt('detail.selectAll')}
								</button>
								<span class="text-muted-foreground/40">|</span>
								<button
									type="button"
									class="text-primary hover:underline disabled:opacity-50 cursor-pointer"
									disabled={isBusy}
									onclick={() => setAllWanted(false)}
								>
									{$tt('detail.selectNone')}
								</button>
							</div>
							<div class="flex flex-col gap-0.5">
								{#each tree as node (node.kind === 'dir' ? `d:${node.path}` : `f:${node.index}`)}
									{@render renderNode(node, 0)}
								{/each}
							</div>
						{/if}
					</Tabs.Content>

					<!-- Peers tab -->
					<Tabs.Content value="peers">
						{#if detail.peers.length === 0}
							<p class="text-sm text-muted-foreground py-6 text-center">{$tt('detail.noPeers')}</p>
						{:else}
							<div class="flex flex-col gap-1.5">
								{#each detail.peers as peer}
									<div class="rounded-md border border-border/60 px-3 py-2">
										<div class="flex items-center justify-between gap-2 mb-1">
											<div class="flex items-center gap-1.5 min-w-0">
												{#if peer.isEncrypted}
													<LockIcon class="size-3 text-muted-foreground flex-shrink-0" />
												{/if}
												<span class="text-sm truncate">{peer.address}</span>
											</div>
											<span class="text-[11px] text-muted-foreground tabular-nums flex-shrink-0">
												{(peer.progress * 100).toFixed(0)}%
											</span>
										</div>
										<div class="flex items-center justify-between gap-2">
											<span class="text-xs text-muted-foreground truncate">{peer.clientName}</span>
											<div class="flex items-center gap-2 text-xs tabular-nums flex-shrink-0">
												<span class="text-blue-500 dark:text-blue-400">↓ {formatSpeed(peer.rateToClient)}</span>
												<span class="text-primary">↑ {formatSpeed(peer.rateToPeer)}</span>
											</div>
										</div>
									</div>
								{/each}
							</div>
						{/if}
					</Tabs.Content>

					<!-- Trackers tab -->
					<Tabs.Content value="trackers">
						{#if detail.trackerStats.length === 0}
							<p class="text-sm text-muted-foreground py-6 text-center">{$tt('detail.noTrackers')}</p>
						{:else}
							<div class="flex flex-col gap-1.5">
								{#each detail.trackerStats as tracker}
									<div class="rounded-md border border-border/60 px-3 py-2">
										<div class="flex items-center justify-between gap-2 mb-1">
											<span class="text-sm truncate min-w-0" title={tracker.announce}>
												{tracker.host}
											</span>
											<span class="text-[11px] text-muted-foreground flex-shrink-0">
												T{tracker.tier}
											</span>
										</div>
										<div class="flex items-center justify-between gap-2 text-xs text-muted-foreground">
											<span class={tracker.lastAnnounceSucceeded ? 'text-emerald-500' : 'text-destructive'}>
												{tracker.lastAnnounceResult || trackerStatus(tracker.announceState)}
											</span>
											<div class="flex items-center gap-2 tabular-nums flex-shrink-0">
												<span>{$tt('detail.trackerSeeders')}: {tracker.seederCount >= 0 ? tracker.seederCount : '—'}</span>
												<span>{$tt('detail.trackerLeechers')}: {tracker.leecherCount >= 0 ? tracker.leecherCount : '—'}</span>
											</div>
										</div>
									</div>
								{/each}
							</div>
						{/if}
					</Tabs.Content>
				</Tabs.Root>
			{/if}
		</div>

		{#if detail && !loading}
			<div class="border-t border-border/60 p-4">
				<Button variant="outline" size="sm" onclick={fetchDetails} class="w-full">
					<RefreshCwIcon class="size-3.5" />
					{$tt('detail.refresh')}
				</Button>
			</div>
		{/if}
	</Sheet.Content>
</Sheet.Root>
