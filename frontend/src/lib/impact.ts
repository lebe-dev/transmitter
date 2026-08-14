import type { Torrent } from './types.js';

/**
 * Impact — сколько раздача вернула от того, что скачала. Зеркалит
 * internal/impact: Transmission присылает uploadRatio, но для торрента без
 * закачки он отрицательный, поэтому считаем сами.
 */
export function ratio(t: Torrent): number {
	if (t.uploadRatio >= 0) return t.uploadRatio;
	if (t.downloadedEver <= 0) return 0;
	return t.uploadedEver / t.downloadedEver;
}

/**
 * Цель достигнута: отдали не меньше, чем скачали (или заданное кратное).
 * Раздача, которая ничего не скачала, долга не имеет — цель к ней не применяется.
 */
export function reached(t: Torrent, target: number): boolean {
	if (target <= 0 || t.downloadedEver <= 0) return false;
	return ratio(t) >= target;
}

/** Раздача ещё не вернула долг: скачала, но отдала меньше цели. */
export function undergiven(t: Torrent, target: number): boolean {
	if (target <= 0 || t.downloadedEver <= 0) return false;
	return ratio(t) < target;
}

/** Процент отданного от скачанного для подсказки, округлённый до целого. */
export function percent(t: Torrent): number {
	return Math.round(ratio(t) * 100);
}
