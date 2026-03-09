import adapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';
import precompileIntl from 'svelte-intl-precompile/sveltekit-plugin';

/** @type {import('@sveltejs/kit').Config} */
const config = {
	preprocess: vitePreprocess([precompileIntl('locales')]),

	kit: {
		adapter: adapter({ fallback: 'index.html' })
	}
};

export default config;
