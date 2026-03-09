import { getTorrents } from './api.js';
import type { Torrent, FilterStatus } from './types.js';

const POLL_ACTIVE = 5_000;
const POLL_HIDDEN = 30_000;
const STALE_MS = 4_000;
const LEADER_KEY = 'tx_leader';
const HB_KEY = 'tx_heartbeat';
const CHANNEL_NAME = 'transmitter';

class TorrentStore {
	torrents = $state<Torrent[]>([]);
	loading = $state(true);
	error = $state<string | null>(null);
	filterStatus = $state<FilterStatus>('all');
	search = $state('');

	readonly #id = crypto.randomUUID();
	#timer: ReturnType<typeof setTimeout> | null = null;
	#channel: BroadcastChannel | null = null;
	#leader = false;

	get filtered(): Torrent[] {
		let list = this.torrents;

		if (this.search) {
			const q = this.search.toLowerCase();
			list = list.filter((t) => t.name.toLowerCase().includes(q));
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
		}

		return list;
	}

	init() {
		this.#channel = new BroadcastChannel(CHANNEL_NAME);
		this.#channel.onmessage = (ev: MessageEvent) => {
			if (ev.data?.type === 'data') {
				this.torrents = ev.data.torrents;
				this.loading = false;
				this.error = null;
				if (!this.#leader) {
					this.#clearTimer();
					this.#scheduleNext();
				}
			}
		};

		window.addEventListener('beforeunload', this.#onUnload);
		document.addEventListener('visibilitychange', this.#onVisibilityChange);

		this.#tryLead();
	}

	destroy() {
		this.#clearTimer();
		this.#channel?.close();
		window.removeEventListener('beforeunload', this.#onUnload);
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

	#onUnload = () => {
		if (this.#leader) {
			localStorage.removeItem(LEADER_KEY);
			localStorage.removeItem(HB_KEY);
		}
	};

	#onVisibilityChange = () => {
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
			this.torrents = torrents;
			this.loading = false;
			this.error = null;
			this.#channel?.postMessage({ type: 'data', torrents });
		} catch (err) {
			this.error = err instanceof Error ? err.message : 'Connection error';
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
