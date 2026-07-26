import type { Torrent } from './types.js';

export const NIGHT_SHIFT_LABEL = 'night-shift';
export const DAY_SHIFT_LABEL = 'day-shift';

/** Торренты, которые есть смысл останавливать: всё, кроме уже остановленных. */
export function pausableIds(torrents: Torrent[]): number[] {
	return torrents.filter((t) => t.status !== 0).map((t) => t.id);
}

/** Один запрос torrent-set: всем ids назначается один и тот же набор меток. */
export type LabelUpdate = { ids: number[]; labels: string[] };

/**
 * Планирует снятие или простановку метки для списка торрентов.
 *
 * torrent-set присваивает labels целиком, поэтому одним запросом можно накрыть
 * только те торренты, у которых совпадает итоговый набор меток — отсюда
 * группировка. Торренты, которых изменение не касается, отбрасываются, чтобы не
 * дёргать Transmission зря.
 */
export function labelUpdates(torrents: Torrent[], label: string, add: boolean): LabelUpdate[] {
	const groups = new Map<string, LabelUpdate>();

	for (const t of torrents) {
		const current = t.labels ?? [];
		if (current.includes(label) === add) continue;

		const next = add ? [...current, label] : current.filter((l) => l !== label);
		const key = JSON.stringify(next);
		const group = groups.get(key);
		if (group) {
			group.ids.push(t.id);
			continue;
		}
		groups.set(key, { ids: [t.id], labels: next });
	}

	return [...groups.values()];
}

/** Сколько торрентов затронет набор обновлений. */
export function countIds(updates: LabelUpdate[]): number {
	return updates.reduce((n, u) => n + u.ids.length, 0);
}
