import { describe, it, expect, beforeEach, vi } from 'vitest';
import {
	MAX_CACHED_DETAILS,
	OfflineError,
	errorText,
	failText,
	readCache,
	readDetail,
	writeCache,
	writeDetail,
} from './offline.js';
import type { TorrentDetail } from './types.js';

function memoryStorage(): Storage {
	const data = new Map<string, string>();
	return {
		get length() {
			return data.size;
		},
		clear: () => data.clear(),
		getItem: (k) => data.get(k) ?? null,
		key: (i) => [...data.keys()][i] ?? null,
		removeItem: (k) => void data.delete(k),
		setItem: (k, v) => void data.set(k, String(v)),
	};
}

function detail(id: number): TorrentDetail {
	return { id, files: [], fileStats: [], peers: [], trackerStats: [] } as unknown as TorrentDetail;
}

const t = (id: string) => `<${id}>`;

beforeEach(() => {
	vi.stubGlobal('localStorage', memoryStorage());
});

describe('readCache / writeCache', () => {
	it('round-trips data with its timestamp', () => {
		writeCache('torrents', [{ id: 1 }], 1234);
		expect(readCache('torrents')).toEqual({ data: [{ id: 1 }], savedAt: 1234 });
	});

	it('returns null for a missing key', () => {
		expect(readCache('nope')).toBeNull();
	});

	it('returns null for corrupt or foreign data', () => {
		localStorage.setItem('tx_cache_bad', '{not json');
		localStorage.setItem('tx_cache_shape', JSON.stringify({ foo: 1 }));
		expect(readCache('bad')).toBeNull();
		expect(readCache('shape')).toBeNull();
	});

	it('swallows storage errors on write', () => {
		vi.stubGlobal('localStorage', {
			...memoryStorage(),
			setItem: () => {
				throw new Error('QuotaExceededError');
			},
		});
		expect(() => writeCache('torrents', [], 1)).not.toThrow();
	});
});

describe('readDetail / writeDetail', () => {
	it('stores details per torrent hash', () => {
		writeDetail('a', detail(1), 10);
		writeDetail('b', detail(2), 20);
		expect(readDetail('a')).toEqual({ data: detail(1), savedAt: 10 });
		expect(readDetail('b')?.data.id).toBe(2);
		expect(readDetail('c')).toBeNull();
	});

	it('keeps only the most recently saved torrents', () => {
		for (let i = 0; i <= MAX_CACHED_DETAILS; i++) writeDetail(`h${i}`, detail(i), i);
		expect(readDetail('h0')).toBeNull();
		expect(readDetail('h1')).not.toBeNull();
		expect(readDetail(`h${MAX_CACHED_DETAILS}`)).not.toBeNull();
	});
});

describe('failText / errorText', () => {
	it('uses the offline hint for OfflineError', () => {
		expect(failText(new OfflineError(), t, 'toast.failStart')).toBe('<offline.unavailable>');
		expect(errorText(new OfflineError(), t, 'toast.failAdd')).toBe('<offline.unavailable>');
	});

	it('falls back to the action text otherwise', () => {
		expect(failText(new Error('HTTP 500'), t, 'toast.failStart')).toBe('<toast.failStart>');
		expect(errorText(new Error('HTTP 500'), t, 'toast.failAdd')).toBe('HTTP 500');
		expect(errorText('weird', t, 'toast.failAdd')).toBe('<toast.failAdd>');
	});
});
