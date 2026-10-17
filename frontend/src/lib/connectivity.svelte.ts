/**
 * Состояние связи с сервером. Офлайн — когда браузер сообщил о потере сети
 * или запрос не дошёл до сервера; онлайн — как только любой запрос получил ответ.
 */
class ConnectivityStore {
	offline = $state(!navigator.onLine);

	#listeners = new Set<() => void>();

	constructor() {
		window.addEventListener('offline', () => this.markOffline());
		// Сеть вернулась, но сервер может быть ещё недоступен: подписчики
		// перезапрашивают данные, и первый ответ сам снимет офлайн.
		window.addEventListener('online', () => this.#notify());
	}

	markOffline() {
		this.offline = true;
	}

	markOnline() {
		if (!this.offline) return;
		this.offline = false;
		this.#notify();
	}

	/** Вызывает cb при восстановлении связи; возвращает функцию отписки. */
	onReconnect(cb: () => void): () => void {
		this.#listeners.add(cb);
		return () => this.#listeners.delete(cb);
	}

	#notify() {
		for (const cb of this.#listeners) cb();
	}
}

export const connectivity = new ConnectivityStore();
