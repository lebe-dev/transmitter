import type { Torrent } from './types.js';

/** Сколько байт ещё предстоит скачать на диск. */
export function bytesNeeded(t: Torrent): number {
	return t.leftUntilDone ?? Math.max(0, t.totalSize * (1 - t.percentDone));
}

/**
 * Остановленный торрент, которому не хватает места в его папке: стартовать его
 * нельзя, пока место не освободится. Если свободное место папки неизвестно —
 * не блокируем.
 */
export function blockedBySpace(t: Torrent, freeByDir: Record<string, number>): boolean {
	if (t.status !== 0) return false;
	const free = freeByDir[t.downloadDir];
	if (free === undefined) return false;
	return bytesNeeded(t) > free;
}

/** Папки остановленных незавершённых торрентов — для них нужно знать свободное место. */
export function dirsToCheck(torrents: Torrent[]): string[] {
	const dirs = new Set<string>();
	for (const t of torrents) {
		if (t.status === 0 && t.downloadDir && bytesNeeded(t) > 0) dirs.add(t.downloadDir);
	}
	return [...dirs];
}
