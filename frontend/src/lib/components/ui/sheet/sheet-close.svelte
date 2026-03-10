<script lang="ts">
	import { Dialog as DialogPrimitive } from "bits-ui";
	import { cn } from "$lib/utils.js";

	let {
		ref = $bindable(null),
		class: className,
		...restProps
	}: DialogPrimitive.CloseProps = $props();
</script>

<DialogPrimitive.Close
	bind:ref
	data-slot="sheet-close"
	class={cn("size-7 rounded-md flex items-center justify-center text-muted-foreground hover:text-foreground hover:bg-accent transition-colors", className)}
	{...restProps}
/>
