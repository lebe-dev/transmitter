<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';

	// /settings — не отдельный экран, а вход в первую вкладку.
	onMount(() => {
		void goto('/settings/general', { replaceState: true });
	});
</script>
