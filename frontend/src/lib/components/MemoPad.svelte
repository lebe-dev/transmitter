<script lang="ts">
	import { onDestroy, onMount } from 'svelte';
	import { get } from 'svelte/store';
	import { toast } from 'svelte-sonner';
	import { t as tt } from 'svelte-intl-precompile';
	import NotebookPenIcon from '@lucide/svelte/icons/notebook-pen';

	import { memoStore } from '$lib/stores.svelte.js';

	/** Сколько строк показываем, пока текста меньше. */
	const MIN_ROWS = 3;
	const SAVE_DELAY = 800;

	let draft = $state('');
	let status = $state<'idle' | 'saving' | 'saved'>('idle');
	let timer: ReturnType<typeof setTimeout> | null = null;
	/** Пока пользователь не начал печатать, подтягиваем текст с сервера. */
	let dirty = false;

	// Высота растёт вместе с текстом: не меньше MIN_ROWS строк, дальше — по числу строк.
	const rows = $derived(Math.max(MIN_ROWS, draft.split('\n').length));

	$effect(() => {
		if (!dirty) draft = memoStore.text;
	});

	async function save() {
		if (timer) {
			clearTimeout(timer);
			timer = null;
		}
		if (draft === memoStore.text) {
			dirty = false;
			return;
		}
		status = 'saving';
		const text = draft;
		try {
			await memoStore.save(text);
			// Пока шёл запрос, могли допечатать — тогда остаёмся «грязными».
			dirty = draft !== text;
			status = 'saved';
		} catch (err) {
			status = 'idle';
			const msg = err instanceof Error && err.message === 'HTTP 422' ? 'memo.tooLong' : 'memo.failSave';
			toast.error(get(tt)(msg));
		}
	}

	function onInput() {
		dirty = true;
		status = 'idle';
		if (timer) clearTimeout(timer);
		timer = setTimeout(save, SAVE_DELAY);
	}

	onMount(() => {
		void memoStore.init();
	});

	onDestroy(() => {
		// Не теряем последние правки при уходе со страницы.
		if (timer) void save();
	});
</script>

<div class="flex flex-col gap-1.5">
	<div class="flex items-center gap-1.5 text-xs font-medium text-muted-foreground">
		<NotebookPenIcon class="size-3.5" />
		<label for="memo-pad">{$tt('memo.label')}</label>
		<span class="ml-auto tabular-nums" aria-live="polite">
			{#if status === 'saving'}
				{$tt('memo.saving')}
			{:else if status === 'saved'}
				{$tt('memo.saved')}
			{/if}
		</span>
	</div>
	<textarea
		id="memo-pad"
		{rows}
		bind:value={draft}
		oninput={onInput}
		onblur={save}
		placeholder={$tt('memo.placeholder')}
		class="w-full resize-none rounded-lg border border-input bg-background px-3 py-2 text-sm leading-5 outline-none transition-colors focus:border-primary/40 focus:ring-2 focus:ring-primary/10"
	></textarea>
</div>
