import tailwindcss from "@tailwindcss/vite";
import { sveltekit } from "@sveltejs/kit/vite";
import { defineConfig } from "vite";
import precompileIntl from "svelte-intl-precompile/sveltekit-plugin";

export default defineConfig({
  plugins: [tailwindcss(), sveltekit(), precompileIntl("locales")],
  server: {
    allowedHosts: ["test.home"],
  },
});
