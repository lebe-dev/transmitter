import adapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';
import precompileIntl from 'svelte-intl-precompile/sveltekit-plugin';

/** @type {import('@sveltejs/kit').Config} */
const config = {
	preprocess: vitePreprocess([precompileIntl('locales')]),

	kit: {
		// precompress: the Go server sends the .br/.gz siblings as they are,
		// so a Raspberry Pi never has to compress the bundle on the fly.
		adapter: adapter({ fallback: 'index.html', precompress: true })
	}
};

export default config;
