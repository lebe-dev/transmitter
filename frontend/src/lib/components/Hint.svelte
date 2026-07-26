<script lang="ts">
	import type { Snippet } from 'svelte';
	import * as Tooltip from '$lib/components/ui/tooltip/index.js';

	type Side = 'top' | 'right' | 'bottom' | 'left';

	let {
		text,
		side = 'top',
		class: className,
		contentClass,
		children,
		trigger
	}: {
		/** Текст подсказки; пустое значение отключает тултип. */
		text: string | undefined | null;
		side?: Side;
		/** Классы для дефолтного span-триггера. */
		class?: string;
		contentClass?: string;
		children?: Snippet;
		/** Своя разметка триггера: props нужно распылить на корневой элемент. */
		trigger?: Snippet<[{ props: Record<string, unknown> }]>;
	} = $props();
</script>

{#if text}
	<Tooltip.Root>
		<Tooltip.Trigger>
			{#snippet child({ props })}
				{#if trigger}
					{@render trigger({ props })}
				{:else}
					<span {...props} class={className}>{@render children?.()}</span>
				{/if}
			{/snippet}
		</Tooltip.Trigger>
		<Tooltip.Content {side} class={contentClass}>{text}</Tooltip.Content>
	</Tooltip.Root>
{:else if trigger}
	{@render trigger({ props: {} })}
{:else}
	<span class={className}>{@render children?.()}</span>
{/if}
