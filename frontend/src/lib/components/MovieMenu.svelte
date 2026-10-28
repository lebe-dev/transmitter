<script lang="ts">
	import { get } from 'svelte/store';
	import { toast } from 'svelte-sonner';
	import { t as tt } from 'svelte-intl-precompile';
	import ClapperboardIcon from '@lucide/svelte/icons/clapperboard';
	import ImageIcon from '@lucide/svelte/icons/image';
	import EyeIcon from '@lucide/svelte/icons/eye';
	import EyeOffIcon from '@lucide/svelte/icons/eye-off';
	import CaptionsIcon from '@lucide/svelte/icons/captions';
	import CaptionsOffIcon from '@lucide/svelte/icons/captions-off';
	import PencilLineIcon from '@lucide/svelte/icons/pencil-line';
	import RefreshCwIcon from '@lucide/svelte/icons/refresh-cw';
	import Link2Icon from '@lucide/svelte/icons/link-2';
	import CircleXIcon from '@lucide/svelte/icons/circle-x';

	import * as DropdownMenu from '$lib/components/ui/dropdown-menu/index.js';
	import { movieLabel, type MovieDialogKind } from '$lib/movie.js';
	import { movieStore } from '$lib/movies.svelte.js';
	import { errorText } from '$lib/offline.js';
	import type { Torrent } from '$lib/types.js';

	let {
		torrent,
		onOpen,
	}: {
		torrent: Torrent;
		onOpen: (kind: MovieDialogKind) => void;
	} = $props();

	const rec = $derived(movieStore.get(torrent.hashString));
	const matched = $derived(rec?.state === 'matched');

	/** Подпись над пунктами: что плагин знает о раздаче. */
	const heading = $derived.by(() => {
		if (matched) return movieLabel(rec);
		switch (rec?.state) {
			case 'unmatched':
				return $tt('movie.state.unmatched');
			case 'skipped':
				return $tt('movie.state.skipped');
			case 'failed':
				return $tt('movie.state.failed');
			default:
				return $tt('movie.state.pending');
		}
	});

	async function run(action: () => Promise<void>, failKey: string) {
		try {
			await action();
		} catch (err) {
			toast.error(errorText(err, get(tt), failKey));
		}
	}

	function toggleWatched() {
		const watched = !rec?.watched;
		void run(async () => {
			await movieStore.setFlags(torrent.hashString, { watched });
			toast.success(get(tt)(watched ? 'movie.toast.watched' : 'movie.toast.unwatched'));
		}, 'movie.toast.failFlags');
	}

	function toggleMissingSubs() {
		const missingSubs = !rec?.missingSubs;
		void run(async () => {
			await movieStore.setFlags(torrent.hashString, { missingSubs });
			toast.success(get(tt)(missingSubs ? 'movie.toast.missingSubs' : 'movie.toast.hasSubs'));
		}, 'movie.toast.failFlags');
	}

	function detect() {
		void run(async () => {
			await movieStore.detect(torrent.hashString);
			const next = movieStore.get(torrent.hashString);
			if (next?.state === 'matched') {
				toast.success(get(tt)('movie.toast.detected', { values: { title: movieLabel(next) } }));
			} else {
				toast.info(get(tt)('movie.toast.notDetected', { values: { query: next?.query || torrent.name } }));
			}
		}, 'movie.toast.failDetect');
	}

	function clear() {
		void run(async () => {
			await movieStore.clear(torrent.hashString);
			toast.success(get(tt)('movie.toast.cleared'));
		}, 'movie.toast.failClear');
	}

	const stop = (e: Event) => e.stopPropagation();
</script>

<DropdownMenu.Root>
	<DropdownMenu.Trigger
		onclick={stop}
		onkeydown={stop}
		aria-label={$tt('movie.menu')}
		title={$tt('movie.menu')}
		class="size-7 rounded-md flex items-center justify-center transition-colors data-[state=open]:bg-accent {matched
			? 'text-primary hover:bg-primary/10'
			: 'text-muted-foreground/40 hover:text-muted-foreground'}"
	>
		<ClapperboardIcon class="size-3.5" />
	</DropdownMenu.Trigger>

	<DropdownMenu.Content class="min-w-60 max-w-72" align="end">
		<DropdownMenu.Group>
			<DropdownMenu.GroupHeading class="truncate">{heading}</DropdownMenu.GroupHeading>
			<DropdownMenu.Separator />

			<DropdownMenu.Item disabled={!matched} onSelect={() => onOpen('info')}>
				<ImageIcon class="size-3.5" />
				{$tt('movie.actions.info')}
			</DropdownMenu.Item>
			<DropdownMenu.Item onSelect={toggleWatched}>
				{#if rec?.watched}
					<EyeOffIcon class="size-3.5" />
					{$tt('movie.actions.markUnwatched')}
				{:else}
					<EyeIcon class="size-3.5" />
					{$tt('movie.actions.markWatched')}
				{/if}
			</DropdownMenu.Item>
			<DropdownMenu.Item onSelect={toggleMissingSubs}>
				{#if rec?.missingSubs}
					<CaptionsIcon class="size-3.5" />
					{$tt('movie.actions.hasSubs')}
				{:else}
					<CaptionsOffIcon class="size-3.5" />
					{$tt('movie.actions.missingSubs')}
				{/if}
			</DropdownMenu.Item>

			<DropdownMenu.Separator />

			<DropdownMenu.Item disabled={!matched} onSelect={() => onOpen('rename')}>
				<PencilLineIcon class="size-3.5" />
				{$tt('movie.actions.rename')}
			</DropdownMenu.Item>
			<DropdownMenu.Item onSelect={detect}>
				<RefreshCwIcon class="size-3.5" />
				{$tt('movie.actions.detect')}
			</DropdownMenu.Item>
			<DropdownMenu.Item onSelect={() => onOpen('link')}>
				<Link2Icon class="size-3.5" />
				{$tt('movie.actions.link')}
			</DropdownMenu.Item>
			<DropdownMenu.Item disabled={!matched} variant="destructive" onSelect={clear}>
				<CircleXIcon class="size-3.5" />
				{$tt('movie.actions.clear')}
			</DropdownMenu.Item>
		</DropdownMenu.Group>
	</DropdownMenu.Content>
</DropdownMenu.Root>
