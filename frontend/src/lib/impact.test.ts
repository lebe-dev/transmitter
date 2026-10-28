import { describe, it, expect } from 'vitest';
import { percent, ratio, reached, undergiven } from './impact.js';
import type { Torrent } from './types.js';

function torrent(patch: Partial<Torrent> = {}): Torrent {
	return {
		id: 1,
		name: 'torrent',
		status: 6,
		percentDone: 1,
		totalSize: 1000,
		rateDownload: 0,
		rateUpload: 0,
		addedDate: 0,
		eta: -1,
		hashString: 'hash',
		downloadDir: '/downloads',
		error: 0,
		errorString: '',
		uploadedEver: 0,
		downloadedEver: 0,
		uploadRatio: -1,
		labels: [],
		...patch,
	};
}

describe('ratio', () => {
	it('uses the ratio reported by Transmission', () => {
		expect(ratio(torrent({ uploadRatio: 1.5 }))).toBe(1.5);
	});

	it('falls back to the counters when the ratio is unknown', () => {
		expect(ratio(torrent({ uploadRatio: -1, uploadedEver: 300, downloadedEver: 200 }))).toBe(1.5);
	});

	it('is zero when nothing was downloaded', () => {
		expect(ratio(torrent({ uploadRatio: -1, uploadedEver: 100 }))).toBe(0);
	});
});

describe('reached', () => {
	it('is true at and above the target', () => {
		expect(reached(torrent({ uploadRatio: 1, downloadedEver: 100 }), 1)).toBe(true);
		expect(reached(torrent({ uploadRatio: 2, downloadedEver: 100 }), 1)).toBe(true);
	});

	it('is false below the target', () => {
		expect(reached(torrent({ uploadRatio: 0.99, downloadedEver: 100 }), 1)).toBe(false);
	});

	it('ignores torrents that downloaded nothing', () => {
		expect(reached(torrent({ uploadRatio: 5 }), 1)).toBe(false);
	});
});

describe('undergiven', () => {
	it('is true while the torrent still owes', () => {
		expect(undergiven(torrent({ uploadRatio: 0.4, downloadedEver: 100 }), 1)).toBe(true);
	});

	it('is false once the target is reached', () => {
		expect(undergiven(torrent({ uploadRatio: 1, downloadedEver: 100 }), 1)).toBe(false);
	});

	it('is false for a torrent with no debt', () => {
		expect(undergiven(torrent(), 1)).toBe(false);
	});
});

describe('percent', () => {
	it('rounds to whole percents', () => {
		expect(percent(torrent({ uploadRatio: 0.523, downloadedEver: 100 }))).toBe(52);
		expect(percent(torrent({ uploadRatio: 2, downloadedEver: 100 }))).toBe(200);
	});
});
