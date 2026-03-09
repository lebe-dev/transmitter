<script lang="ts">
	import { Dialog as DialogPrimitive } from "bits-ui";
	import SheetPortal from "./sheet-portal.svelte";
	import SheetOverlay from "./sheet-overlay.svelte";
	import { cn, type WithoutChild, type WithoutChildrenOrChild } from "$lib/utils.js";
	import type { ComponentProps } from "svelte";

	let {
		ref = $bindable(null),
		class: className,
		portalProps,
		...restProps
	}: WithoutChild<DialogPrimitive.ContentProps> & {
		portalProps?: WithoutChildrenOrChild<ComponentProps<typeof SheetPortal>>;
	} = $props();
</script>

<SheetPortal {...portalProps}>
	<SheetOverlay />
	<DialogPrimitive.Content
		bind:ref
		data-slot="sheet-content"
		class={cn(
			"bg-background data-[state=open]:animate-in data-[state=closed]:animate-out data-[state=closed]:slide-out-to-right data-[state=open]:slide-in-from-right fixed inset-y-0 right-0 z-50 flex w-full flex-col border-l shadow-lg duration-200 sm:w-[440px]",
			className
		)}
		{...restProps}
	/>
</SheetPortal>
