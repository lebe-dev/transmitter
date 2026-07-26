import { getSettings, getServerConfig, setShiftEnabled } from './api.js';
import type { ServerConfig, ShiftName, UISettings } from './types.js';

const COMPACT_STORAGE_KEY = 'transmitter-compact';
const FREE_SPACE_STORAGE_KEY = 'transmitter-show-free-space';

/**
 * configured — окно смены задано переменными окружения (смена вообще есть);
 * enabled — планировщик включён в настройках (состояние живёт на сервере).
 */
export type ShiftState = {
	configured: boolean;
	enabled: boolean;
	start?: string;
	end?: string;
};

/**
 * Настройки приложения: локальные (localStorage) и серверные (/api/settings,
 * /api/config). Живут в сторе, потому что нужны и списку торрентов, и странице
 * настроек — это отдельные роуты.
 */
class SettingsStore {
	compactView = $state(false);
	showFreeSpace = $state(true);

	/** Значение чекбокса «удалить вместе с данными» по умолчанию. */
	deleteWithData = $state(false);
	/** Лимит длины заметки в символах, приходит с сервера. */
	noteMaxLength = $state(0);

	day = $state<ShiftState>({ configured: false, enabled: false });
	night = $state<ShiftState>({ configured: false, enabled: false });

	serverConfig = $state<ServerConfig | null>(null);
	serverConfigLoading = $state(false);
	serverConfigError = $state(false);

	/**
	 * Запрос на открытие диалога добавления торрента со страницы настроек:
	 * сам диалог живёт на главной, поэтому кнопка в шапке ставит флаг и уходит
	 * на «/», а главная его подхватывает.
	 */
	addRequested = $state(false);

	#settings: Promise<UISettings> | null = null;

	constructor() {
		this.compactView = localStorage.getItem(COMPACT_STORAGE_KEY) === '1';
		this.showFreeSpace = localStorage.getItem(FREE_SPACE_STORAGE_KEY) !== '0';
	}

	setCompactView(enabled: boolean) {
		this.compactView = enabled;
		localStorage.setItem(COMPACT_STORAGE_KEY, enabled ? '1' : '0');
	}

	setShowFreeSpace(enabled: boolean) {
		this.showFreeSpace = enabled;
		localStorage.setItem(FREE_SPACE_STORAGE_KEY, enabled ? '1' : '0');
	}

	requestAdd() {
		this.addRequested = true;
	}

	/** Грузит /api/settings один раз за сессию; повторные вызовы разделяют промис. */
	load(): Promise<UISettings> {
		this.#settings ??= getSettings()
			.then((s) => {
				this.deleteWithData = s.deleteWithData;
				this.noteMaxLength = s.noteMaxLength;
				this.day = {
					configured: s.dayShiftConfigured,
					enabled: s.dayShiftEnabled,
					start: s.dayShiftStart,
					end: s.dayShiftEnd,
				};
				this.night = {
					configured: s.nightShiftConfigured,
					enabled: s.nightShiftEnabled,
					start: s.nightShiftStart,
					end: s.nightShiftEnd,
				};
				return s;
			})
			.catch((err) => {
				// Разрешаем повторную попытку на следующем заходе.
				this.#settings = null;
				throw err;
			});
		return this.#settings;
	}

	/** Включает/выключает планировщик смены, откатывая переключатель при ошибке. */
	async setShiftEnabled(shift: ShiftName, enabled: boolean) {
		const target = shift === 'night' ? this.night : this.day;
		const previous = target.enabled;
		target.enabled = enabled;
		try {
			await setShiftEnabled(shift, enabled);
		} catch (err) {
			target.enabled = previous;
			throw err;
		}
	}

	async loadServerConfig() {
		if (this.serverConfig !== null || this.serverConfigLoading) return;
		this.serverConfigLoading = true;
		this.serverConfigError = false;
		try {
			this.serverConfig = await getServerConfig();
		} catch {
			this.serverConfigError = true;
		} finally {
			this.serverConfigLoading = false;
		}
	}
}

export const settingsStore = new SettingsStore();
