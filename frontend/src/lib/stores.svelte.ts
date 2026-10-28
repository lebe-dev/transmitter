import { getTorrents, getSession, getNotes, setNote, getMemo, setMemo, getGlobalPause, setGlobalPause } from './api.js';
import { undergiven } from './impact.js';
import { matchesSearch, missingSubs, unwatched } from './movie.js';
import { movieStore } from './movies.svelte.js';
import { settingsStore } from './settings.svelte.js';
import { connectivity } from './connectivity.svelte.js';
import { OfflineError, readCache, writeCache } from './offline.js';
import type { Torrent, FilterStatus, TorrentNotes } from './types.js';

const POLL_ACTIVE = 5_000;
const POLL_HIDDEN = 30_000;
const STALE_MS = 4_000;
const LEADER_KEY = 'tx_leader';
const HB_KEY = 'tx_heartbeat';
const CHANNEL_NAME = 'transmitter';
const TORRENTS_CACHE_KEY = 'torrents';

class TorrentStore {
	torrents = $state<Torrent[]>([]);
	loading = $state(true);
	error = $state<string | null>(null);
	filterStatus = $state<FilterStatus>('all');
	search = $state('');
	/** Когда список последний раз пришёл с сервера (ms); показывается в офлайне. */
	syncedAt = $state<number | null>(null);

	readonly #id = crypto.randomUUID();
	#timer: ReturnType<typeof setTimeout> | null = null;
	#channel: BroadcastChannel | null = null;
	#leader = false;

	constructor() {
		// Последний известный список: виден сразу при старте и остаётся в офлайне.
		const cached = readCache<Torrent[]>(TORRENTS_CACHE_KEY);
		if (cached) {
			this.torrents = cached.data;
			this.syncedAt = cached.savedAt;
			this.loading = false;
		}
	}

	get filtered(): Torrent[] {
		let list = this.torrents;

		if (this.search) {
			const q = this.search.toLowerCase();
			list = list.filter(
				(t) =>
					t.name.toLowerCase().includes(q) ||
					(movieStore.enabled && matchesSearch(movieStore.get(t.hashString), q)),
			);
		}

		switch (this.filterStatus) {
			case 'downloading':
				list = list.filter((t) => t.status === 4 || t.status === 3);
				break;
			case 'seeding':
				list = list.filter((t) => t.status === 6 || t.status === 5);
				break;
			case 'paused':
				list = list.filter((t) => t.status === 0);
				break;
			case 'done':
				list = list.filter(
					(t) =>
						t.percentDone === 1 && (t.status === 0 || t.status === 5 || t.status === 6)
				);
				break;
			case 'undergiven':
				list = list.filter((t) => undergiven(t, settingsStore.impactTargetRatio));
				break;
			case 'unwatched':
				list = list.filter((t) => unwatched(movieStore.get(t.hashString)));
				break;
			case 'missingSubs':
				list = list.filter((t) => missingSubs(movieStore.get(t.hashString)));
				break;
		}

		return list;
	}

	init() {
		this.#channel = new BroadcastChannel(CHANNEL_NAME);
		this.#channel.onmessage = (ev: MessageEvent) => {
			if (ev.data?.type === 'data') {
				this.torrents = ev.data.torrents;
				this.syncedAt = ev.data.syncedAt ?? Date.now();
				this.loading = false;
				this.error = null;
				if (!this.#leader) {
					this.#clearTimer();
					this.#scheduleNext();
				}
			}
		};

		window.addEventListener('beforeunload', this.#onUnload);
		window.addEventListener('online', this.#onOnline);
		document.addEventListener('visibilitychange', this.#onVisibilityChange);

		this.#tryLead();
	}

	destroy() {
		this.#clearTimer();
		this.#channel?.close();
		window.removeEventListener('beforeunload', this.#onUnload);
		window.removeEventListener('online', this.#onOnline);
		document.removeEventListener('visibilitychange', this.#onVisibilityChange);
		if (this.#leader) {
			localStorage.removeItem(LEADER_KEY);
			localStorage.removeItem(HB_KEY);
		}
	}

	async refresh() {
		if (!this.#leader) return;
		this.#clearTimer();
		await this.#fetch();
		this.#scheduleNext();
	}

	#tryLead() {
		const hb = Number(localStorage.getItem(HB_KEY) ?? 0);
		if (Date.now() - hb > STALE_MS) {
			this.#startLeading();
		} else {
			this.#leader = false;
			this.#scheduleNext();
		}
	}

	#startLeading() {
		this.#leader = true;
		localStorage.setItem(LEADER_KEY, this.#id);
		this.#fetch().then(() => this.#scheduleNext());
	}

	readonly #onUnload = () => {
		if (this.#leader) {
			localStorage.removeItem(LEADER_KEY);
			localStorage.removeItem(HB_KEY);
		}
	};

	readonly #onOnline = () => {
		void this.refresh();
	};

	readonly #onVisibilityChange = () => {
		if (document.hidden) return;
		if (this.#leader) {
			this.#clearTimer();
			this.#fetch().then(() => this.#scheduleNext());
		} else {
			const hb = Number(localStorage.getItem(HB_KEY) ?? 0);
			if (Date.now() - hb > STALE_MS) this.#startLeading();
		}
	};

	async #fetch() {
		if (!this.#leader) return;
		localStorage.setItem(HB_KEY, String(Date.now()));
		try {
			const torrents = await getTorrents();
			const syncedAt = Date.now();
			this.torrents = torrents;
			this.syncedAt = syncedAt;
			this.loading = false;
			this.error = null;
			writeCache(TORRENTS_CACHE_KEY, torrents, syncedAt);
			this.#channel?.postMessage({ type: 'data', torrents, syncedAt });
		} catch (err) {
			// В офлайне список остаётся последним известным, об этом говорит баннер.
			if (err instanceof OfflineError) this.error = null;
			else this.error = err instanceof Error ? err.message : 'Connection error';
			this.loading = false;
		}
	}

	#scheduleNext() {
		this.#clearTimer();
		if (this.#leader) {
			const interval = document.hidden ? POLL_HIDDEN : POLL_ACTIVE;
			this.#timer = setTimeout(() => this.#fetch().then(() => this.#scheduleNext()), interval);
		} else {
			// Follower: check periodically if leader died
			this.#timer = setTimeout(() => {
				const hb = Number(localStorage.getItem(HB_KEY) ?? 0);
				if (Date.now() - hb > STALE_MS) {
					this.#startLeading();
				} else {
					this.#scheduleNext();
				}
			}, POLL_HIDDEN);
		}
	}

	#clearTimer() {
		if (this.#timer !== null) {
			clearTimeout(this.#timer);
			this.#timer = null;
		}
	}
}

export const torrentStore = new TorrentStore();

// ── Pin store (localStorage) ─────────────────────────────────────────────

const PIN_KEY = 'tx_pinned';

class PinStore {
	#pinned = $state<Set<string>>(new Set());

	constructor() {
		try {
			const raw = localStorage.getItem(PIN_KEY);
			if (raw) this.#pinned = new Set(JSON.parse(raw));
		} catch {
			// ignore corrupt data
		}
	}

	isPinned(hashString: string): boolean {
		return this.#pinned.has(hashString);
	}

	toggle(hashString: string) {
		if (this.#pinned.has(hashString)) {
			this.#pinned.delete(hashString);
		} else {
			this.#pinned.add(hashString);
		}
		// trigger reactivity by reassigning
		this.#pinned = new Set(this.#pinned);
		this.#persist();
	}

	#persist() {
		localStorage.setItem(PIN_KEY, JSON.stringify([...this.#pinned]));
	}
}

export const pinStore = new PinStore();

// ── Torrent notes (backend SQLite, keyed by torrent hash) ────────────────

const NOTES_CACHE_KEY = 'notes';

class NoteStore {
	/** Character limit reported by the server via /api/settings. */
	maxLength = $state(0);

	#notes = $state<TorrentNotes>(readCache<TorrentNotes>(NOTES_CACHE_KEY)?.data ?? {});

	constructor() {
		connectivity.onReconnect(() => void this.reload());
	}

	get(hashString: string): string {
		return this.#notes[hashString] ?? '';
	}

	async init(maxLength: number) {
		this.maxLength = maxLength;
		await this.reload();
	}

	async reload() {
		try {
			this.#notes = await getNotes();
			writeCache(NOTES_CACHE_KEY, this.#notes);
		} catch {
			// Notes are supplementary: keep the torrent list usable without them.
		}
	}

	/** Persists a note. An empty text removes it. Throws if the server rejects it. */
	async save(hashString: string, text: string) {
		const trimmed = text.trim();
		await setNote(hashString, trimmed);

		const next = { ...this.#notes };
		if (trimmed) next[hashString] = trimmed;
		else delete next[hashString];
		this.#notes = next;
		writeCache(NOTES_CACHE_KEY, next);
	}
}

export const noteStore = new NoteStore();

// ── Global memo (backend SQLite, shown above the torrent list) ───────────

const MEMO_CACHE_KEY = 'memo';

class MemoStore {
	text = $state(readCache<string>(MEMO_CACHE_KEY)?.data ?? '');
	loaded = $state(false);

	constructor() {
		connectivity.onReconnect(() => {
			this.loaded = false;
			void this.init();
		});
	}

	async init() {
		if (this.loaded) return;
		try {
			this.text = await getMemo();
			this.loaded = true;
			writeCache(MEMO_CACHE_KEY, this.text);
		} catch {
			// Заметка вспомогательная: список торрентов работает и без неё.
		}
	}

	/** Сохраняет текст как есть (с переводами строк); бросает, если сервер отказал. */
	async save(text: string) {
		await setMemo(text);
		this.text = text;
		writeCache(MEMO_CACHE_KEY, text);
	}
}

export const memoStore = new MemoStore();

// ── Global pause (state lives on the server) ──────────────────────────────

const PAUSE_CACHE_KEY = 'pause';

class PauseStore {
	paused = $state(readCache<boolean>(PAUSE_CACHE_KEY)?.data ?? false);
	busy = $state(false);

	constructor() {
		connectivity.onReconnect(() => void this.init());
	}

	async init() {
		try {
			this.paused = await getGlobalPause();
			writeCache(PAUSE_CACHE_KEY, this.paused);
		} catch {
			// Кнопка просто покажет «не на паузе»; следующий клик синхронизирует состояние.
		}
	}

	/** Переключает паузу и возвращает число затронутых раздач. */
	async toggle(): Promise<number> {
		this.busy = true;
		try {
			const next = !this.paused;
			const count = await setGlobalPause(next);
			this.paused = next;
			writeCache(PAUSE_CACHE_KEY, next);
			return count;
		} finally {
			this.busy = false;
		}
	}
}

export const pauseStore = new PauseStore();

// ── Download directory store (localStorage + session-get) ────────────────

const DIR_KEY = 'tx_download_dirs';
const SESSION_CACHE_KEY = 'session';

type SessionInfo = { defaultDir: string; defaultFreeSpace: number | null };

class DownloadDirStore {
	defaultDir = $state('');
	defaultFreeSpace = $state<number | null>(null);
	savedDirs = $state<string[]>([]);
	selectedDir = $state('');

	get allDirs(): string[] {
		const set = new Set<string>();
		if (this.defaultDir) set.add(this.defaultDir);
		for (const d of this.savedDirs) set.add(d);
		return [...set];
	}

	constructor() {
		try {
			const raw = localStorage.getItem(DIR_KEY);
			if (raw) this.savedDirs = JSON.parse(raw);
		} catch {
			// ignore corrupt data
		}
		const cached = readCache<SessionInfo>(SESSION_CACHE_KEY);
		if (cached) {
			this.defaultDir = cached.data.defaultDir;
			this.defaultFreeSpace = cached.data.defaultFreeSpace;
			this.selectedDir = this.defaultDir;
		}
		connectivity.onReconnect(() => void this.refreshFreeSpace());
	}

	#inited = false;

	/** Идемпотентно: и главная, и шапка настроек вызывают init() при монтировании. */
	async init() {
		if (this.#inited) return;
		this.#inited = true;
		try {
			const session = await getSession();
			const prevDefault = this.defaultDir;
			this.defaultDir = session['download-dir'] ?? '';
			this.defaultFreeSpace = session['download-dir-free-space'] ?? null;
			// selectedDir мог прийти из кэша вместе со старым путём по умолчанию.
			if (!this.selectedDir || this.selectedDir === prevDefault) this.selectedDir = this.defaultDir;
			this.#cacheSession();
		} catch {
			// session-get may fail if server is unreachable
		}
	}

	async refreshFreeSpace() {
		try {
			const session = await getSession();
			if (session['download-dir']) this.defaultDir = session['download-dir'];
			this.defaultFreeSpace = session['download-dir-free-space'] ?? null;
			this.#cacheSession();
		} catch {
			// leave the last known value in place on transient failures
		}
	}

	selectDir(path: string) {
		this.selectedDir = path;
	}

	addDir(path: string) {
		if (!path || path === this.defaultDir) return;
		if (this.savedDirs.includes(path)) return;
		this.savedDirs = [...this.savedDirs, path];
		this.#persist();
	}

	removeDir(path: string) {
		if (path === this.defaultDir) return;
		this.savedDirs = this.savedDirs.filter((d) => d !== path);
		this.#persist();
	}

	#persist() {
		localStorage.setItem(DIR_KEY, JSON.stringify(this.savedDirs));
	}

	#cacheSession() {
		writeCache<SessionInfo>(SESSION_CACHE_KEY, {
			defaultDir: this.defaultDir,
			defaultFreeSpace: this.defaultFreeSpace,
		});
	}
}

export const downloadDirStore = new DownloadDirStore();

// ── Expanded directories per torrent (localStorage) ──────────────────────

const EXPANDED_KEY = 'tx_expanded_dirs';

class ExpandedDirsStore {
	#byHash = $state<Map<string, Set<string>>>(new Map());

	constructor() {
		try {
			const raw = localStorage.getItem(EXPANDED_KEY);
			if (raw) {
				const obj = JSON.parse(raw) as Record<string, string[]>;
				this.#byHash = new Map(
					Object.entries(obj).map(([h, paths]) => [h, new Set(paths)]),
				);
			}
		} catch {
			// ignore corrupt data
		}
	}

	get(hashString: string): Set<string> | null {
		return this.#byHash.get(hashString) ?? null;
	}

	set(hashString: string, paths: Set<string>) {
		this.#byHash.set(hashString, new Set(paths));
		this.#byHash = new Map(this.#byHash);
		this.#persist();
	}

	#persist() {
		const obj: Record<string, string[]> = {};
		for (const [h, set] of this.#byHash) obj[h] = [...set];
		localStorage.setItem(EXPANDED_KEY, JSON.stringify(obj));
	}
}

export const expandedDirsStore = new ExpandedDirsStore();
