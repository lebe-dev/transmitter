import { describe, it, expect } from 'vitest';
import { blockedBySpace, bytesNeeded, dirsToCheck } from './space.js';
import type { Torrent } from './types.js';

function torrent(patch: Partial<Torrent> = {}): Torrent {
	return {
		id: 1,
		name: 't',
		status: 0,
		percentDone: 0,
		totalSize: 1000,
		rateDownload: 0,
		rateUpload: 0,
		addedDate: 0,
		eta: -1,
		hashString: 'h',
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

describe('bytesNeeded', () => {
	it('prefers leftUntilDone', () => {
		expect(bytesNeeded(torrent({ leftUntilDone: 300, percentDone: 0.1 }))).toBe(300);
	});

	it('falls back to the unfinished share of totalSize', () => {
		expect(bytesNeeded(torrent({ percentDone: 0.25 }))).toBe(750);
	});
});

describe('blockedBySpace', () => {
	it('blocks a stopped torrent that does not fit', () => {
		expect(blockedBySpace(torrent(), { '/downloads': 999 })).toBe(true);
	});

	it('allows a torrent that fits exactly', () => {
		expect(blockedBySpace(torrent(), { '/downloads': 1000 })).toBe(false);
	});

	it('does not block when free space is unknown', () => {
		expect(blockedBySpace(torrent(), {})).toBe(false);
	});

	it('ignores torrents that are not stopped', () => {
		expect(blockedBySpace(torrent({ status: 4 }), { '/downloads': 0 })).toBe(false);
	});
});

describe('dirsToCheck', () => {
	it('collects unique dirs of stopped unfinished torrents only', () => {
		const list = [
			torrent({ id: 1, downloadDir: '/a' }),
			torrent({ id: 2, downloadDir: '/a' }),
			torrent({ id: 3, downloadDir: '/b', percentDone: 1 }),
			torrent({ id: 4, downloadDir: '/c', status: 4 }),
		];
		expect(dirsToCheck(list)).toEqual(['/a']);
	});
});
