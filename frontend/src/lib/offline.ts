import type { TorrentDetail } from './types.js';

/**
 * Thrown by the API layer when a request never reached the server (no network,
 * server down, DNS failure). Distinguishes "we are offline" from "the server
 * answered with an error".
 */
export class OfflineError extends Error {
	constructor() {
		super('offline');
		this.name = 'OfflineError';
	}
}

type Translator = (id: string) => string;

/** Toast text for a failed action: the offline hint, or the action's own failure text. */
export function failText(err: unknown, t: Translator, fallbackKey: string): string {
	return err instanceof OfflineError ? t('offline.unavailable') : t(fallbackKey);
}

/** Like failText, but prefers the server's error message when there is one. */
export function errorText(err: unknown, t: Translator, fallbackKey: string): string {
	if (err instanceof OfflineError) return t('offline.unavailable');
	return err instanceof Error ? err.message : t(fallbackKey);
}

// ── Last known server data (localStorage) ────────────────────────────────

const CACHE_PREFIX = 'tx_cache_';

export type Cached<T> = { data: T; savedAt: number };

export function readCache<T>(key: string): Cached<T> | null {
	try {
		const raw = localStorage.getItem(CACHE_PREFIX + key);
		if (!raw) return null;
		const parsed = JSON.parse(raw);
		if (typeof parsed?.savedAt !== 'number' || !('data' in parsed)) return null;
		return parsed as Cached<T>;
	} catch {
		return null;
	}
}

export function writeCache<T>(key: string, data: T, now = Date.now()): void {
	try {
		localStorage.setItem(CACHE_PREFIX + key, JSON.stringify({ data, savedAt: now }));
	} catch {
		// Quota exceeded or storage disabled: offline mode just has less to show.
	}
}

// Details (files, peers, trackers) are cached only for torrents the user has
// opened, and only the most recent ones, so the cache cannot grow unbounded.
const DETAILS_KEY = 'details';
export const MAX_CACHED_DETAILS = 20;

type DetailMap = Record<string, Cached<TorrentDetail>>;

export function readDetail(hashString: string): Cached<TorrentDetail> | null {
	return readCache<DetailMap>(DETAILS_KEY)?.data[hashString] ?? null;
}

export function writeDetail(hashString: string, detail: TorrentDetail, now = Date.now()): void {
	const all: DetailMap = { ...readCache<DetailMap>(DETAILS_KEY)?.data };
	all[hashString] = { data: detail, savedAt: now };
	const stale = Object.keys(all)
		.sort((a, b) => all[b].savedAt - all[a].savedAt)
		.slice(MAX_CACHED_DETAILS);
	for (const key of stale) delete all[key];
	writeCache(DETAILS_KEY, all, now);
}
