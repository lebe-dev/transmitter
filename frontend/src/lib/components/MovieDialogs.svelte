<script lang="ts">
	import { get } from 'svelte/store';
	import { toast } from 'svelte-sonner';
	import { t as tt } from 'svelte-intl-precompile';
	import ImageOffIcon from '@lucide/svelte/icons/image-off';
	import ExternalLinkIcon from '@lucide/svelte/icons/external-link';
	import StarIcon from '@lucide/svelte/icons/star';
	import EyeIcon from '@lucide/svelte/icons/eye';
	import CaptionsOffIcon from '@lucide/svelte/icons/captions-off';
	import ArrowDownIcon from '@lucide/svelte/icons/arrow-down';

	import * as AlertDialog from '$lib/components/ui/alert-dialog/index.js';
	import { Button } from '$lib/components/ui/button/index.js';
	import { Spinner } from '$lib/components/ui/spinner/index.js';
	import { getRenamePreview, moviePosterUrl } from '$lib/api.js';
	import { imdbUrl, movieLabel, tmdbUrl, type MovieDialogKind } from '$lib/movie.js';
	import { movieStore } from '$lib/movies.svelte.js';
	import { torrentStore } from '$lib/stores.svelte.js';
	import { errorText } from '$lib/offline.js';
	import type { Torrent } from '$lib/types.js';

	let {
		kind = $bindable(null),
		torrent,
	}: {
		kind: MovieDialogKind | null;
		torrent: Torrent | null;
	} = $props();

	const rec = $derived(torrent ? movieStore.get(torrent.hashString) : undefined);
	const busyKinds = $state({ link: false, rename: false });

	function close() {
		if (busyKinds.link || busyKinds.rename) return;
		kind = null;
	}

	/** Сообщения сервера — английские; переводим те, что знаем. */
	function movieError(err: unknown, fallbackKey: string): string {
		const t = get(tt);
		const message = err instanceof Error ? err.message : '';
		if (message === 'title not found') return t('movie.toast.linkNotFound');
		if (message.startsWith('unsupported link')) return t('movie.toast.linkUnsupported');
		return errorText(err, t, fallbackKey);
	}

	// ── Info ──────────────────────────────────────────────────────────────

	let posterFailed = $state(false);
	$effect(() => {
		// A new title gets a fresh chance to show its poster.
		void rec?.tmdbId;
		posterFailed = false;
	});

	// ── Link ──────────────────────────────────────────────────────────────

	let linkUrl = $state('');
	$effect(() => {
		if (kind === 'link') linkUrl = '';
	});

	async function submitLink() {
		if (!torrent || !linkUrl.trim() || busyKinds.link) return;
		busyKinds.link = true;
		try {
			await movieStore.link(torrent.hashString, linkUrl.trim());
			const next = movieStore.get(torrent.hashString);
			toast.success(get(tt)('movie.toast.linked', { values: { title: movieLabel(next) } }));
			busyKinds.link = false;
			kind = 'info';
		} catch (err) {
			toast.error(movieError(err, 'movie.toast.failLink'));
		} finally {
			busyKinds.link = false;
		}
	}

	// ── Rename ────────────────────────────────────────────────────────────

	let preview = $state<{ current: string; target: string } | null>(null);
	let previewError = $state<string | null>(null);

	$effect(() => {
		if (kind !== 'rename' || !torrent) return;
		const hash = torrent.hashString;
		preview = null;
		previewError = null;
		let cancelled = false;
		getRenamePreview(hash)
			.then((p) => {
				if (!cancelled) preview = p;
			})
			.catch((err) => {
				if (!cancelled) previewError = movieError(err, 'movie.toast.failRename');
			});
		return () => {
			cancelled = true;
		};
	});

	const nothingToRename = $derived(preview !== null && preview.current === preview.target);

	async function submitRename() {
		if (!torrent || !preview || busyKinds.rename) return;
		busyKinds.rename = true;
		try {
			const name = await movieStore.rename(torrent.hashString);
			toast.success(get(tt)('movie.toast.renamed', { values: { name } }));
			busyKinds.rename = false;
			kind = null;
			await torrentStore.refresh();
		} catch (err) {
			toast.error(movieError(err, 'movie.toast.failRename'));
		} finally {
			busyKinds.rename = false;
		}
	}
</script>

<!-- ── Poster and details ─────────────────────────────────────────────── -->
<AlertDialog.Root open={kind === 'info'} onOpenChange={(open) => !open && close()}>
	<AlertDialog.Content class="sm:max-w-xl max-h-[90vh] overflow-y-auto">
		{#if torrent && rec?.state === 'matched'}
			<div class="flex flex-col sm:flex-row gap-5">
				<div class="flex-shrink-0 self-center sm:self-start">
					{#if rec.hasPoster && !posterFailed}
						<img
							src={moviePosterUrl(torrent.hashString, rec.tmdbId)}
							alt={$tt('movie.info.posterAlt', { values: { title: rec.title ?? '' } })}
							class="w-32 sm:w-44 aspect-[2/3] rounded-md object-cover bg-muted shadow-sm"
							onerror={() => (posterFailed = true)}
						/>
					{:else}
						<div class="w-32 sm:w-44 aspect-[2/3] rounded-md bg-muted flex flex-col items-center justify-center gap-2 text-muted-foreground">
							<ImageOffIcon class="size-6" />
							<span class="text-xs">{$tt('movie.info.noPoster')}</span>
						</div>
					{/if}
				</div>

				<div class="flex flex-col gap-3 min-w-0">
					<AlertDialog.Header class="text-left">
						<AlertDialog.Title class="font-display text-lg font-semibold leading-snug">
							{rec.title}
						</AlertDialog.Title>
						<AlertDialog.Description class="text-sm text-muted-foreground flex flex-wrap items-center gap-x-2 gap-y-0.5">
							{#if rec.originalTitle && rec.originalTitle !== rec.title}
								<span class="italic">{rec.originalTitle}</span>
							{/if}
							{#if rec.year}<span class="tabular-nums">{rec.year}</span>{/if}
							<span>{$tt(rec.mediaType === 'tv' ? 'movie.info.tv' : 'movie.info.movie')}</span>
							{#if rec.rating}
								<span class="flex items-center gap-0.5 tabular-nums">
									<StarIcon class="size-3 text-amber-500" />
									{rec.rating.toFixed(1)}
								</span>
							{/if}
						</AlertDialog.Description>
					</AlertDialog.Header>

					{#if rec.watched || rec.missingSubs}
						<div class="flex flex-wrap gap-1.5">
							{#if rec.watched}
								<span class="inline-flex items-center gap-1 rounded-md bg-primary/10 text-primary px-2 py-0.5 text-[11px] font-medium">
									<EyeIcon class="size-3" />
									{$tt('movie.info.watched')}
								</span>
							{/if}
							{#if rec.missingSubs}
								<span class="inline-flex items-center gap-1 rounded-md bg-amber-500/10 text-amber-600 dark:text-amber-400 px-2 py-0.5 text-[11px] font-medium">
									<CaptionsOffIcon class="size-3" />
									{$tt('movie.info.missingSubs')}
								</span>
							{/if}
						</div>
					{/if}

					{#if rec.overview}
						<p class="text-sm leading-relaxed text-foreground/90">{rec.overview}</p>
					{:else}
						<p class="text-sm text-muted-foreground">{$tt('movie.info.noOverview')}</p>
					{/if}

					<div class="flex flex-wrap gap-x-4 gap-y-1 text-sm">
						{#if tmdbUrl(rec)}
							<a href={tmdbUrl(rec)} target="_blank" rel="noopener noreferrer" class="inline-flex items-center gap-1 text-primary hover:underline">
								TMDB <ExternalLinkIcon class="size-3" />
							</a>
						{/if}
						{#if imdbUrl(rec)}
							<a href={imdbUrl(rec)} target="_blank" rel="noopener noreferrer" class="inline-flex items-center gap-1 text-primary hover:underline">
								IMDb <ExternalLinkIcon class="size-3" />
							</a>
						{/if}
					</div>

					<div class="text-xs text-muted-foreground space-y-0.5 [overflow-wrap:anywhere]">
						{#if rec.manual}
							<p>{$tt('movie.info.manual')}</p>
						{:else if rec.query}
							<p>{$tt('movie.info.detectedFrom', { values: { query: rec.query } })}</p>
						{/if}
						{#if !rec.manual && !rec.confident}
							<p class="text-amber-600 dark:text-amber-400">{$tt('movie.info.notConfident')}</p>
						{/if}
						{#if rec.renamedTo}
							<p>{$tt('movie.info.renamedTo', { values: { name: rec.renamedTo } })}</p>
						{/if}
					</div>
				</div>
			</div>
		{/if}

		<AlertDialog.Footer class="pt-2 gap-2">
			<Button variant="outline" onclick={() => (kind = 'link')}>{$tt('movie.actions.link')}</Button>
			<Button variant="outline" onclick={() => (kind = 'rename')}>{$tt('movie.actions.rename')}</Button>
			<Button class="font-display font-semibold" onclick={close}>{$tt('movie.close')}</Button>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>

<!-- ── Link to TMDB / IMDb ─────────────────────────────────────────────── -->
<AlertDialog.Root open={kind === 'link'} onOpenChange={(open) => !open && close()}>
	<AlertDialog.Content class="sm:max-w-md">
		<AlertDialog.Header class="min-w-0 pb-2">
			<AlertDialog.Title class="font-display text-lg font-semibold">{$tt('movie.link.title')}</AlertDialog.Title>
			<AlertDialog.Description class="min-w-0">
				<span class="font-medium text-foreground [overflow-wrap:anywhere]">{torrent?.name}</span>
				<br />
				{$tt('movie.link.description')}
			</AlertDialog.Description>
		</AlertDialog.Header>

		<input
			type="url"
			inputmode="url"
			placeholder="https://www.themoviedb.org/movie/157336"
			class="w-full h-10 rounded-lg border border-input bg-background px-3 text-sm outline-none transition-colors focus:border-primary/40 focus:ring-2 focus:ring-primary/10"
			bind:value={linkUrl}
			disabled={busyKinds.link}
			onkeydown={(e) => e.key === 'Enter' && submitLink()}
		/>
		<p class="text-xs text-muted-foreground">{$tt('movie.link.hint')}</p>

		<AlertDialog.Footer class="pt-2">
			<Button variant="outline" disabled={busyKinds.link} onclick={close}>{$tt('movie.cancel')}</Button>
			<Button class="font-display font-semibold" disabled={busyKinds.link || !linkUrl.trim()} onclick={submitLink}>
				{#if busyKinds.link}
					<Spinner class="size-4" />
				{/if}
				{$tt('movie.link.submit')}
			</Button>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>

<!-- ── Rename on disk ─────────────────────────────────────────────────── -->
<AlertDialog.Root open={kind === 'rename'} onOpenChange={(open) => !open && close()}>
	<AlertDialog.Content class="sm:max-w-md">
		<AlertDialog.Header class="min-w-0 pb-2">
			<AlertDialog.Title class="font-display text-lg font-semibold">{$tt('movie.rename.title')}</AlertDialog.Title>
			<AlertDialog.Description>{$tt('movie.rename.description')}</AlertDialog.Description>
		</AlertDialog.Header>

		{#if previewError}
			<p class="text-sm text-destructive">{previewError}</p>
		{:else if !preview}
			<div class="flex justify-center py-4"><Spinner class="size-5" /></div>
		{:else}
			<div class="flex flex-col gap-1.5 rounded-lg border border-border/60 bg-accent/30 px-3 py-2.5 text-sm font-mono [overflow-wrap:anywhere]">
				<span class="text-muted-foreground">{preview.current}</span>
				<ArrowDownIcon class="size-3.5 text-muted-foreground" />
				<span class="font-medium">{preview.target}</span>
			</div>
			{#if nothingToRename}
				<p class="text-xs text-muted-foreground">{$tt('movie.rename.same')}</p>
			{/if}
		{/if}

		<AlertDialog.Footer class="pt-2">
			<Button variant="outline" disabled={busyKinds.rename} onclick={close}>{$tt('movie.cancel')}</Button>
			<Button
				class="font-display font-semibold"
				disabled={busyKinds.rename || !preview || nothingToRename}
				onclick={submitRename}
			>
				{#if busyKinds.rename}
					<Spinner class="size-4" />
				{/if}
				{$tt('movie.rename.submit')}
			</Button>
		</AlertDialog.Footer>
	</AlertDialog.Content>
</AlertDialog.Root>
