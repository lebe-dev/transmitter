<script lang="ts">
	import type { Component } from 'svelte';
	import { get } from 'svelte/store';
	import { toast } from 'svelte-sonner';
	import { t as tt } from 'svelte-intl-precompile';
	import ChevronDownIcon from '@lucide/svelte/icons/chevron-down';
	import ListChecksIcon from '@lucide/svelte/icons/list-checks';
	import PauseIcon from '@lucide/svelte/icons/pause';
	import SunIcon from '@lucide/svelte/icons/sun';
	import MoonStarIcon from '@lucide/svelte/icons/moon-star';

	import * as AlertDialog from '$lib/components/ui/alert-dialog/index.js';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu/index.js';
	import { Button } from '$lib/components/ui/button/index.js';
	import { Spinner } from '$lib/components/ui/spinner/index.js';
	import { setTorrentsLabels, stopTorrents } from '$lib/api.js';
	import {
		DAY_SHIFT_LABEL,
		NIGHT_SHIFT_LABEL,
		countIds,
		labelUpdates,
		pausableIds,
		type LabelUpdate,
	} from '$lib/bulk.js';
	import { settingsStore, type ShiftState } from '$lib/settings.svelte.js';
	import { torrentStore } from '$lib/stores.svelte.js';

	type BulkAction = {
		/** Ключ действия: из него собираются переводы пункта меню и тоста. */
		key: string;
		icon: Component;
		/** Сколько раздач реально изменится; 0 — пункт неактивен. */
		count: number;
		/** Смена выключена в настройках: пункт показываем, но не даём нажать. */
		disabled: boolean;
		run: () => Promise<void>;
	};

	/**
	 * Массовые действия применяются к тому, что сейчас на экране — то есть с
	 * учётом фильтра и поиска. Без фильтра это и есть «все раздачи».
	 */
	const targets = $derived(torrentStore.filtered);
	const filtered = $derived(targets.length !== torrentStore.torrents.length);

	/** torrent-set по группам: раздачи с разными метками нельзя обновить одним запросом. */
	async function applyLabelUpdates(updates: LabelUpdate[]) {
		// Последовательно: Transmission на Raspberry Pi не любит пачку параллельных RPC.
		for (const update of updates) {
			await setTorrentsLabels(update.ids, update.labels);
		}
	}

	function shiftActions(
		shift: ShiftState,
		label: string,
		icon: Component,
		keys: { add: string; remove: string },
	): BulkAction[] {
		if (!shift.configured) return [];
		return [
			{ key: keys.add, add: true },
			{ key: keys.remove, add: false },
		].map(({ key, add }) => {
			const updates = labelUpdates(targets, label, add);
			return {
				key,
				icon,
				count: countIds(updates),
				disabled: !shift.enabled,
				run: () => applyLabelUpdates(updates),
			};
		});
	}

	const actions = $derived.by<BulkAction[]>(() => {
		const ids = pausableIds(targets);
		return [
			{
				key: 'pause',
				icon: PauseIcon,
				count: ids.length,
				disabled: false,
				run: () => stopTorrents(ids),
			},
			...shiftActions(settingsStore.day, DAY_SHIFT_LABEL, SunIcon, {
				add: 'dayShiftAdd',
				remove: 'dayShiftRemove',
			}),
			...shiftActions(settingsStore.night, NIGHT_SHIFT_LABEL, MoonStarIcon, {
				add: 'nightShiftAdd',
				remove: 'nightShiftRemove',
			}),
		];
	});

	// ── Подтверждение ────────────────────────────────────────────────────────

	// Храним ключ, а не само действие: список раздач обновляется каждые несколько
	// секунд, и к моменту подтверждения id и метки должны быть свежими.
	let pendingKey = $state<string | null>(null);
	let running = $state(false);

	const pending = $derived(actions.find((a) => a.key === pendingKey) ?? null);

	async function confirm() {
		if (!pending || running) return;
		const { key, count, run } = pending;
		running = true;
		try {
			await run();
			toast.success(get(tt)(`bulk.done.${key}`, { values: { count } }));
			pendingKey = null;
			await torrentStore.refresh();
		} catch {
			toast.error(get(tt)('bulk.fail'));
		} finally {
			running = false;
		}
	}
</script>

<DropdownMenu.Root>
	<DropdownMenu.Trigger
		class="flex items-center gap-1 rounded-md px-1.5 py-1 text-xs text-muted-foreground transition-colors hover:text-foreground data-[state=open]:text-foreground"
		aria-label={$tt('bulk.menu')}
	>
		<ListChecksIcon class="size-3.5" />
		<span>{$tt('bulk.trigger')}</span>
		<ChevronDownIcon class="size-3" />
	</DropdownMenu.Trigger>

	<DropdownMenu.Content class="min-w-56" align="end">
		<DropdownMenu.Group>
			<DropdownMenu.GroupHeading>
				{$tt('bulk.menu')}
				{#if filtered}
					<span class="block font-normal opacity-70">{$tt('bulk.filteredNotice')}</span>
				{/if}
			</DropdownMenu.GroupHeading>
			<DropdownMenu.Separator />

			{#each actions as action (action.key)}
				{@const Icon = action.icon}
				<DropdownMenu.Item
					disabled={action.disabled || action.count === 0}
					onSelect={() => (pendingKey = action.key)}
				>
					<Icon class="size-3.5" />
					<span class="flex-1">{$tt(`bulk.${action.key}`)}</span>
					<span class="text-xs text-muted-foreground tabular-nums">{action.count}</span>
				</DropdownMenu.Item>
			{/each}
		</DropdownMenu.Group>
	</DropdownMenu.Content>
</DropdownMenu.Root>

<AlertDialog.Root
	open={pending !== null}
	onOpenChange={(open) => {
		if (!open && !running) pendingKey = null;
	}}
>
	<AlertDialog.Content class="sm:max-w-md">
		<AlertDialog.Header class="pb-4">
			<AlertDialog.Title class="font-display text-lg font-semibold">
				{$tt('bulk.confirmTitle')}
			</AlertDialog.Title>
			<AlertDialog.Description>
				<span class="font-medium text-foreground">{$tt(`bulk.${pending?.key ?? 'pause'}`)}</span>
				<br />
				{$tt('bulk.confirmDescription', { values: { count: pending?.count ?? 0 } })}
				{#if filtered}
					<br />
					{$tt('bulk.filteredNotice')}
				{/if}
			</AlertDialog.Description>
		</AlertDialog.Header>

		<AlertDialog.Footer class="pt-4">
			<Button variant="outline" disabled={running} onclick={() => (pendingKey = null)}>
				{$tt('bulk.cancel')}
			</Button>
			<Button class="font-display font-semibold" disabled={running} onclick={confirm}>
				{#if running}
					<Spinner class="size-4" />
				{/if}
				{$tt('bulk.confirmButton')}
			</Button>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>
