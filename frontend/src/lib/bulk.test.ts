import { describe, it, expect } from 'vitest';
import { countIds, labelUpdates, pausableIds } from './bulk.js';
import type { Torrent } from './types.js';

function torrent(id: number, patch: Partial<Torrent> = {}): Torrent {
	return {
		id,
		name: `torrent-${id}`,
		status: 4,
		percentDone: 0.5,
		totalSize: 1000,
		rateDownload: 0,
		rateUpload: 0,
		addedDate: 0,
		eta: -1,
		hashString: `hash-${id}`,
		downloadDir: '/downloads',
		error: 0,
		errorString: '',
		uploadedEver: 0,
		labels: [],
		...patch,
	};
}

describe('pausableIds', () => {
	it('skips torrents that are already stopped', () => {
		const list = [torrent(1, { status: 0 }), torrent(2, { status: 4 }), torrent(3, { status: 6 })];
		expect(pausableIds(list)).toEqual([2, 3]);
	});

	it('returns an empty list when everything is stopped', () => {
		expect(pausableIds([torrent(1, { status: 0 })])).toEqual([]);
	});
});

describe('labelUpdates', () => {
	it('skips torrents that already carry the label', () => {
		const list = [torrent(1, { labels: ['night-shift'] }), torrent(2)];
		expect(labelUpdates(list, 'night-shift', true)).toEqual([{ ids: [2], labels: ['night-shift'] }]);
	});

	it('skips torrents that do not carry the label on removal', () => {
		const list = [torrent(1, { labels: ['night-shift'] }), torrent(2)];
		expect(labelUpdates(list, 'night-shift', false)).toEqual([{ ids: [1], labels: [] }]);
	});

	it('groups torrents that end up with the same label set', () => {
		const list = [
			torrent(1),
			torrent(2),
			torrent(3, { labels: ['movies'] }),
			torrent(4, { labels: ['movies'] }),
		];
		expect(labelUpdates(list, 'day-shift', true)).toEqual([
			{ ids: [1, 2], labels: ['day-shift'] },
			{ ids: [3, 4], labels: ['movies', 'day-shift'] },
		]);
	});

	it('keeps unrelated labels when removing', () => {
		const list = [torrent(1, { labels: ['movies', 'day-shift', 'hd'] })];
		expect(labelUpdates(list, 'day-shift', false)).toEqual([
			{ ids: [1], labels: ['movies', 'hd'] },
		]);
	});

	it('tolerates a missing labels field', () => {
		const list = [torrent(1, { labels: undefined as unknown as string[] })];
		expect(labelUpdates(list, 'day-shift', true)).toEqual([{ ids: [1], labels: ['day-shift'] }]);
	});

	it('returns nothing when no torrent changes', () => {
		const list = [torrent(1, { labels: ['day-shift'] })];
		expect(labelUpdates(list, 'day-shift', true)).toEqual([]);
	});
});

describe('countIds', () => {
	it('sums the ids of every group', () => {
		expect(
			countIds([
				{ ids: [1, 2], labels: [] },
				{ ids: [3], labels: [] },
			]),
		).toBe(3);
	});

	it('is zero for no groups', () => {
		expect(countIds([])).toBe(0);
	});
});
