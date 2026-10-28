import {
	clearMovie,
	detectMovie,
	getMovies,
	linkMovie,
	renameMovie,
	setMovieFlags,
} from './api.js';
import { connectivity } from './connectivity.svelte.js';
import { readCache, writeCache } from './offline.js';
import type { MovieRecord, MovieRecords } from './types.js';

const MOVIES_CACHE_KEY = 'movies';
/** Фон распознаёт новые раздачи сам — подтягиваем результаты, пока вкладка видна. */
const POLL_MS = 15_000;

/**
 * Данные Movie Plugin (backend SQLite, ключ — хэш раздачи). Опрашивает сервер,
 * только пока плагин включён: без него маршрутов /api/movies нет вовсе.
 */
class MovieStore {
	enabled = $state(false);

	#records = $state<MovieRecords>(readCache<MovieRecords>(MOVIES_CACHE_KEY)?.data ?? {});
	#timer: ReturnType<typeof setInterval> | null = null;

	constructor() {
		connectivity.onReconnect(() => {
			if (this.enabled) void this.reload();
		});
	}

	get(hashString: string): MovieRecord | undefined {
		return this.#records[hashString];
	}

	/** Включает опрос, если плагин включён на сервере; идемпотентно. */
	async init(enabled: boolean) {
		this.enabled = enabled;
		if (!enabled) {
			this.#stop();
			return;
		}
		await this.reload();
		this.#timer ??= setInterval(() => {
			if (!document.hidden) void this.reload();
		}, POLL_MS);
	}

	destroy() {
		this.#stop();
	}

	async reload() {
		try {
			this.#records = await getMovies();
			writeCache(MOVIES_CACHE_KEY, this.#records);
		} catch {
			// Supplementary data: the torrent list stays usable without it.
		}
	}

	async detect(hashString: string) {
		this.#put(hashString, await detectMovie(hashString));
	}

	async link(hashString: string, url: string) {
		this.#put(hashString, await linkMovie(hashString, url));
	}

	async clear(hashString: string) {
		this.#put(hashString, await clearMovie(hashString));
	}

	async setFlags(hashString: string, flags: { watched?: boolean; missingSubs?: boolean }) {
		this.#put(hashString, await setMovieFlags(hashString, flags));
	}

	/** Переименовывает раздачу на диске и возвращает новое имя. */
	async rename(hashString: string): Promise<string> {
		const name = await renameMovie(hashString);
		const rec = this.#records[hashString];
		if (rec) this.#put(hashString, { ...rec, renamedTo: name });
		return name;
	}

	#put(hashString: string, rec: MovieRecord) {
		this.#records = { ...this.#records, [hashString]: rec };
		writeCache(MOVIES_CACHE_KEY, this.#records);
	}

	#stop() {
		if (this.#timer !== null) {
			clearInterval(this.#timer);
			this.#timer = null;
		}
	}
}

export const movieStore = new MovieStore();
