import { defineConfig } from 'vitest/config';

export default defineConfig({
	test: {
		include: ['src/**/*.test.ts'],
		coverage: {
			provider: 'v8',
			// Scope coverage to the testable logic modules; Svelte UI components
			// and generated bits-ui wrappers are exercised via the app, not units.
			include: ['src/lib/*.ts'],
			exclude: ['src/lib/index.ts', 'src/lib/types.ts', '**/*.test.ts'],
			reporter: ['text', 'html'],
		},
	},
});
