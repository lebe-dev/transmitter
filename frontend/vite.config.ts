import tailwindcss from "@tailwindcss/vite";
import { sveltekit } from "@sveltejs/kit/vite";
import { defineConfig } from "vite";
import precompileIntl from "svelte-intl-precompile/sveltekit-plugin";
import pkg from "./package.json" with { type: "json" };

export default defineConfig({
  plugins: [tailwindcss(), sveltekit(), precompileIntl("locales")],
  define: {
    __APP_VERSION__: JSON.stringify(pkg.version),
  },
  server: {
    allowedHosts: ["test.home"],
  },
});
