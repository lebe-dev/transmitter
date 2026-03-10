<script lang="ts">
	import { Tabs as TabsPrimitive } from "bits-ui";
	import { cn } from "$lib/utils.js";

	let {
		ref = $bindable(null),
		class: className,
		...restProps
	}: TabsPrimitive.TriggerProps = $props();
</script>

<TabsPrimitive.Trigger
	bind:ref
	data-slot="tabs-trigger"
	class={cn(
		"relative pb-2.5 text-sm font-medium transition-colors text-muted-foreground hover:text-foreground data-[state=active]:text-foreground cursor-pointer",
		className
	)}
	{...restProps}
/>
