<script lang="ts">
	import { t as tt, locale, locales } from 'svelte-intl-precompile';
	import { setTheme, theme } from 'mode-watcher';

	import { Checkbox } from '$lib/components/ui/checkbox/index.js';
	import { settingsStore } from '$lib/settings.svelte.js';

	const LOCALE_STORAGE_KEY = 'transmitter-locale';

	const COLOR_THEME_KEYS = [
		{ value: 'green', tKey: 'themes.green' },
		{ value: 'blue', tKey: 'themes.blue' },
		{ value: 'yellow', tKey: 'themes.yellow' },
		{ value: 'default', tKey: 'themes.default' },
		{ value: 'orange', tKey: 'themes.orange' },
		{ value: 'red', tKey: 'themes.red' },
		{ value: 'rose', tKey: 'themes.rose' },
		{ value: 'violet', tKey: 'themes.violet' },
	] as const;

	// mode-watcher manages data-theme attr & localStorage ('mode-watcher-theme')
	// green = "" (default, no data-theme attr), others = theme name
	const toMwTheme = (t: string) => (t === 'green' ? '' : t);
	const fromMwTheme = (t: string) => t || 'green';

	const colorTheme = $derived(fromMwTheme(theme.current ?? ''));

	function onColorThemeChange(value: string) {
		if (value) setTheme(toMwTheme(value));
	}

	function onLocaleChange(loc: string) {
		locale.set(loc);
		localStorage.setItem(LOCALE_STORAGE_KEY, loc);
	}
</script>

<div class="flex flex-col gap-5">
	<div class="flex flex-col gap-3">
		<span class="text-sm font-medium">{$tt('settings.colorTheme')}</span>
		<div class="grid grid-cols-2 sm:grid-cols-4 gap-2">
			{#each COLOR_THEME_KEYS as ct (ct.value)}
				<button
					class="h-9 rounded-lg border text-xs font-medium transition-colors {colorTheme === ct.value
						? 'border-primary bg-primary/10 text-foreground'
						: 'border-border/60 text-muted-foreground hover:border-border hover:bg-accent/50'}"
					onclick={() => onColorThemeChange(ct.value)}
				>
					{$tt(ct.tKey)}
				</button>
			{/each}
		</div>
	</div>

	<div class="flex flex-col gap-3">
		<span class="text-sm font-medium">{$tt('settings.language')}</span>
		<div class="grid grid-cols-2 sm:grid-cols-4 gap-2">
			{#each [...$locales] as loc (loc)}
				<button
					class="h-9 rounded-lg border text-xs font-medium transition-colors {$locale === loc
						? 'border-primary bg-primary/10 text-foreground'
						: 'border-border/60 text-muted-foreground hover:border-border hover:bg-accent/50'}"
					onclick={() => onLocaleChange(loc)}
				>
					{$tt(`languages.${loc}`)}
				</button>
			{/each}
		</div>
	</div>

	<div class="flex flex-col gap-3">
		<div class="flex items-center gap-3">
			<Checkbox
				id="compact-view"
				checked={settingsStore.compactView}
				onCheckedChange={(v) => settingsStore.setCompactView(v)}
			/>
			<label for="compact-view" class="text-sm font-medium cursor-pointer select-none">
				{$tt('settings.compactView')}
			</label>
		</div>

		<div class="flex items-center gap-3">
			<Checkbox
				id="show-free-space"
				checked={settingsStore.showFreeSpace}
				onCheckedChange={(v) => settingsStore.setShowFreeSpace(v)}
			/>
			<label for="show-free-space" class="text-sm font-medium cursor-pointer select-none">
				{$tt('settings.showFreeSpace')}
			</label>
		</div>
	</div>
</div>
