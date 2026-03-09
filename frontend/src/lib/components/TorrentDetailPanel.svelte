<script lang="ts">
	import { t as tt } from 'svelte-intl-precompile';
	import XIcon from '@lucide/svelte/icons/x';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import LockIcon from '@lucide/svelte/icons/lock';
	import AlertCircleIcon from '@lucide/svelte/icons/alert-circle';

	import * as Sheet from '$lib/components/ui/sheet/index.js';
	import * as Tabs from '$lib/components/ui/tabs/index.js';
	import { Button } from '$lib/components/ui/button/index.js';
	import { Spinner } from '$lib/components/ui/spinner/index.js';
	import { getTorrentDetails, setFilesWanted, setFilePriority } from '$lib/api.js';
	import type { Torrent, TorrentDetail, FilePriority } from '$lib/types.js';

	let {
		torrent,
		open = $bindable(false),
	}: {
		torrent: Torrent | null;
		open: boolean;
	} = $props();

	let detail = $state<TorrentDetail | null>(null);
	let loading = $state(false);
	let error = $state<string | null>(null);
	let activeTab = $state('files');
	let updatingFile = $state<number | null>(null);

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
		}
	});

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

	function basename(path: string): string {
		const parts = path.split('/');
		return parts[parts.length - 1];
	}

	function priorityLabel(p: FilePriority): string {
		switch (p) {
			case -1: return $tt('detail.priorityLow');
			case 0: return $tt('detail.priorityNormal');
			case 1: return $tt('detail.priorityHigh');
			default: return '';
		}
	}

	function priorityLevel(p: FilePriority): 'low' | 'normal' | 'high' {
		switch (p) {
			case -1: return 'low';
			case 0: return 'normal';
			case 1: return 'high';
			default: return 'normal';
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

	async function toggleWanted(fileIndex: number, currentlyWanted: boolean) {
		if (!torrent || !detail || updatingFile !== null) return;
		updatingFile = fileIndex;
		const prevWanted = detail.fileStats[fileIndex].wanted;
		detail.fileStats[fileIndex].wanted = !currentlyWanted;
		try {
			if (currentlyWanted) {
				await setFilesWanted(torrent.id, [], [fileIndex]);
			} else {
				await setFilesWanted(torrent.id, [fileIndex], []);
			}
		} catch {
			detail.fileStats[fileIndex].wanted = prevWanted;
		} finally {
			updatingFile = null;
		}
	}

	async function cyclePriority(fileIndex: number, current: FilePriority) {
		if (!torrent || !detail || updatingFile !== null) return;
		updatingFile = fileIndex;
		const next = nextPriority(current);
		const prevPriority = detail.fileStats[fileIndex].priority;
		detail.fileStats[fileIndex].priority = next.value;
		try {
			await setFilePriority(torrent.id, fileIndex, next.level);
		} catch {
			detail.fileStats[fileIndex].priority = prevPriority;
		} finally {
			updatingFile = null;
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
</script>

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
							<div class="flex flex-col gap-1.5">
								{#each detail.files as file, i}
									{@const stat = detail.fileStats[i]}
									{@const progress = file.length > 0 ? file.bytesCompleted / file.length : 0}
									<div class="rounded-md border border-border/60 px-3 py-2 transition-opacity {stat?.wanted === false ? 'opacity-50' : ''}">
										<div class="flex items-center justify-between gap-2 mb-1">
											<div class="flex items-center gap-2 min-w-0">
												<input
													type="checkbox"
													checked={stat?.wanted !== false}
													disabled={updatingFile !== null}
													onchange={() => toggleWanted(i, stat?.wanted !== false)}
													class="size-3.5 accent-primary flex-shrink-0 cursor-pointer"
												/>
												<span class="text-sm truncate min-w-0" title={file.name}>
													{basename(file.name)}
												</span>
											</div>
											<div class="flex items-center gap-2 flex-shrink-0">
												{#if stat}
													<button
														onclick={() => cyclePriority(i, stat.priority)}
														disabled={updatingFile !== null}
														title={$tt('detail.cyclePriority')}
														class="text-[10px] font-medium px-1.5 py-0.5 rounded cursor-pointer transition-colors
															{stat.priority === 1 ? 'bg-primary/15 text-primary' : ''}
															{stat.priority === 0 ? 'bg-muted text-muted-foreground' : ''}
															{stat.priority === -1 ? 'bg-muted/50 text-muted-foreground/60' : ''}"
													>
														{priorityLabel(stat.priority)}
													</button>
												{/if}
												<span class="text-xs text-muted-foreground tabular-nums">
													{formatSize(file.length)}
												</span>
											</div>
										</div>
										<div class="flex items-center gap-2">
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
