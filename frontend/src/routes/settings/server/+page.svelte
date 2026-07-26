<script lang="ts">
	import { onMount } from 'svelte';
	import { t as tt } from 'svelte-intl-precompile';
	import InfoIcon from '@lucide/svelte/icons/info';

	import * as Table from '$lib/components/ui/table/index.js';
	import * as HoverCard from '$lib/components/ui/hover-card/index.js';
	import { Spinner } from '$lib/components/ui/spinner/index.js';
	import { settingsStore } from '$lib/settings.svelte.js';
	import type { ServerConfig } from '$lib/types.js';

	const CONFIG_ROWS_BASE: { envVar: string; key: keyof ServerConfig }[] = [
		{ envVar: 'TRANSMISSION_URL',         key: 'transmissionUrl' },
		{ envVar: 'LISTEN_ADDR',              key: 'listenAddr' },
		{ envVar: 'CORS_ORIGIN',              key: 'corsOrigin' },
		{ envVar: 'LOG_LEVEL',                key: 'logLevel' },
		{ envVar: 'WEBUI_ENABLED',            key: 'webUiEnabled' },
		{ envVar: 'TELEGRAM_BOT_ENABLED',     key: 'telegramBotEnabled' },
		{ envVar: 'TELEGRAM_USERS',           key: 'telegramUsers' },
		{ envVar: 'FILE_PRIORITY_ENABLED',    key: 'filePriorityEnabled' },
		{ envVar: 'FILE_PRIORITY_HIGH_COUNT', key: 'filePriorityHighCount' },
		{ envVar: 'DELETE_WITH_DATA',         key: 'deleteWithData' },
		{ envVar: 'MONITOR_INTERVAL',         key: 'monitorInterval' },
		{ envVar: 'FILE_SELECT_TIMEOUT',      key: 'fileSelectTimeout' },
		{ envVar: 'MAX_REQUEST_BODY_BYTES',   key: 'maxRequestBodyBytes' },
		{ envVar: 'DB_PATH',                  key: 'dbPath' },
		{ envVar: 'TORRENT_NOTE_MAX_LENGTH',  key: 'noteMaxLength' },
		{ envVar: 'TORRENT_NOTE_CLEANUP_INTERVAL', key: 'noteCleanupInterval' },
	];

	const NIGHT_SHIFT_CONFIG_ROWS: { envVar: string; key: keyof ServerConfig }[] = [
		{ envVar: 'NIGHT_SHIFT_START',        key: 'nightShiftStart' },
		{ envVar: 'NIGHT_SHIFT_END',          key: 'nightShiftEnd' },
	];

	const DAY_SHIFT_CONFIG_ROWS: { envVar: string; key: keyof ServerConfig }[] = [
		{ envVar: 'DAY_SHIFT_START',          key: 'dayShiftStart' },
		{ envVar: 'DAY_SHIFT_END',            key: 'dayShiftEnd' },
	];

	const config = $derived(settingsStore.serverConfig);

	const configRows = $derived([
		...CONFIG_ROWS_BASE,
		...(config?.nightShiftConfigured ? NIGHT_SHIFT_CONFIG_ROWS : []),
		...(config?.dayShiftConfigured ? DAY_SHIFT_CONFIG_ROWS : []),
	]);

	function formatConfigValue(v: unknown): string {
		if (Array.isArray(v)) return v.length === 0 ? '—' : v.join(', ');
		if (typeof v === 'boolean') return v ? 'true' : 'false';
		return String(v ?? '—');
	}

	onMount(() => {
		void settingsStore.loadServerConfig();
	});
</script>

{#if settingsStore.serverConfigLoading}
	<div class="flex justify-center py-6">
		<Spinner class="size-5" />
	</div>
{:else if settingsStore.serverConfigError}
	<p class="text-sm text-destructive py-4 text-center">{$tt('settings.configError')}</p>
{:else if config}
	<p class="text-xs text-muted-foreground mb-3">{$tt('settings.configNote')}</p>
	<div class="overflow-hidden rounded-lg border border-border/60">
		<Table.Root class="table-fixed w-full">
			<Table.Header>
				<Table.Row>
					<Table.Head class="text-xs w-[45%]">{$tt('settings.configEnvVar')}</Table.Head>
					<Table.Head class="text-xs">{$tt('settings.configValue')}</Table.Head>
				</Table.Row>
			</Table.Header>
			<Table.Body>
				{#each configRows as row (row.envVar)}
					<Table.Row>
						<Table.Cell class="font-mono text-xs py-2 text-muted-foreground break-all">
							<HoverCard.Root>
								<HoverCard.Trigger class="flex items-center gap-1.5 cursor-default">
									{row.envVar}
									<InfoIcon class="size-3 shrink-0 text-muted-foreground/50" />
								</HoverCard.Trigger>
								<HoverCard.Portal>
									<HoverCard.Content class="w-64 text-xs" side="right">
										{$tt(`settings.configHint.${row.key}`)}
									</HoverCard.Content>
								</HoverCard.Portal>
							</HoverCard.Root>
						</Table.Cell>
						<Table.Cell class="font-mono text-xs py-2 break-all">
							{formatConfigValue(config[row.key])}
						</Table.Cell>
					</Table.Row>
				{/each}
			</Table.Body>
		</Table.Root>
	</div>
{/if}
