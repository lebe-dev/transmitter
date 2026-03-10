import { getLocaleFromNavigator, init } from 'svelte-intl-precompile';
import { registerAll } from '$locales';

registerAll();

const LOCALE_STORAGE_KEY = 'transmitter-locale';
const FALLBACK_LOCALE = 'en';

function getInitialLocale(): string {
	if (typeof window !== 'undefined') {
		const saved = localStorage.getItem(LOCALE_STORAGE_KEY);
		if (saved) return saved;
	}
	return getLocaleFromNavigator() ?? FALLBACK_LOCALE;
}

init({ initialLocale: getInitialLocale(), fallbackLocale: FALLBACK_LOCALE });

export const ssr = false;
export const prerender = false;
