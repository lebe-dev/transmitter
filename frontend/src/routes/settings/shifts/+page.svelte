<script lang="ts">
	import { get } from 'svelte/store';
	import { toast } from 'svelte-sonner';
	import { t as tt } from 'svelte-intl-precompile';

	import { Switch } from '$lib/components/ui/switch/index.js';
	import { Separator } from '$lib/components/ui/separator/index.js';
	import { settingsStore } from '$lib/settings.svelte.js';
	import type { ShiftName } from '$lib/types.js';

	const day = $derived(settingsStore.day);
	const night = $derived(settingsStore.night);
	const bothConfigured = $derived(day.configured && night.configured);

	async function onShiftEnabledChange(shift: ShiftName, enabled: boolean) {
		try {
			await settingsStore.setShiftEnabled(shift, enabled);
		} catch {
			toast.error(get(tt)('toast.failShiftToggle'));
		}
	}
</script>

<div class="flex flex-col gap-4">
	{#if day.configured}
		<div class="flex flex-col gap-3">
			<div class="flex items-center justify-between gap-3">
				<label for="day-shift-enabled" class="text-sm font-medium cursor-pointer select-none">
					{$tt('settings.dayShiftLabel')}
				</label>
				<Switch
					id="day-shift-enabled"
					checked={day.enabled}
					onCheckedChange={(v) => onShiftEnabledChange('day', v)}
				/>
			</div>

			<div class="rounded-lg border border-border/60 bg-accent/30 p-4 {day.enabled ? '' : 'opacity-50'}">
				<p class="text-sm text-muted-foreground leading-relaxed">{$tt('settings.dayShiftDescription')}</p>
			</div>

			<div class="flex flex-col gap-2 rounded-lg border border-border/60 p-4 {day.enabled ? '' : 'opacity-50'}">
				<span class="text-xs uppercase tracking-wide text-muted-foreground">{$tt('settings.dayShiftWindowLabel')}</span>
				<span class="font-mono text-base tabular-nums">{day.start ?? '—'} – {day.end ?? '—'}</span>
				<span class="text-xs text-muted-foreground">{$tt('settings.dayShiftWindowHint')}</span>
			</div>
		</div>
	{/if}

	{#if bothConfigured}
		<Separator />
	{/if}

	{#if night.configured}
		<div class="flex flex-col gap-3">
			<div class="flex items-center justify-between gap-3">
				<label for="night-shift-enabled" class="text-sm font-medium cursor-pointer select-none">
					{$tt('settings.nightShiftLabel')}
				</label>
				<Switch
					id="night-shift-enabled"
					checked={night.enabled}
					onCheckedChange={(v) => onShiftEnabledChange('night', v)}
				/>
			</div>

			<div class="rounded-lg border border-border/60 bg-accent/30 p-4 {night.enabled ? '' : 'opacity-50'}">
				<p class="text-sm text-muted-foreground leading-relaxed">{$tt('settings.nightShiftDescription')}</p>
			</div>

			<div class="flex flex-col gap-2 rounded-lg border border-border/60 p-4 {night.enabled ? '' : 'opacity-50'}">
				<span class="text-xs uppercase tracking-wide text-muted-foreground">{$tt('settings.nightShiftWindowLabel')}</span>
				<span class="font-mono text-base tabular-nums">{night.start ?? '—'} – {night.end ?? '—'}</span>
				<span class="text-xs text-muted-foreground">{$tt('settings.nightShiftWindowHint')}</span>
			</div>
		</div>
	{/if}

	{#if day.configured || night.configured}
		<p class="text-xs text-muted-foreground leading-relaxed">{$tt('settings.shiftToggleHint')}</p>
	{:else}
		<p class="text-sm text-muted-foreground py-4 text-center">{$tt('settings.shiftsNotConfigured')}</p>
	{/if}

	{#if bothConfigured}
		<p class="text-xs text-muted-foreground leading-relaxed">{$tt('settings.shiftConflictWarning')}</p>
	{/if}
</div>
