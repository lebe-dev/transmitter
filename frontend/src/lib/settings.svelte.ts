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

/** Зеркалит internal/shift.InWindow: HH:MM границы, окно может переходить через полночь. */
function inWindow(nowMinutes: number, start: string, end: string): boolean {
	const [startH, startM] = start.split(':').map(Number);
	const [endH, endM] = end.split(':').map(Number);
	const a = startH * 60 + startM;
	const b = endH * 60 + endM;
	if (a === b) return false;
	if (a < b) return nowMinutes >= a && nowMinutes < b;
	return nowMinutes >= a || nowMinutes < b;
}

/** Смена активна прямо сейчас: настроена, включена и текущее время попадает в её окно. */
function isShiftActive(shift: ShiftState, nowMinutes: number): boolean {
	if (!shift.configured || !shift.enabled || !shift.start || !shift.end) return false;
	return inWindow(nowMinutes, shift.start, shift.end);
}

const ACTIVE_SHIFT_CHECK_INTERVAL = 30_000;

/**
 * Настройки приложения: локальные (localStorage) и серверные (/api/settings,
 * /api/config). Живут в сторе, потому что нужны и списку торрентов, и странице
 * настроек — это отдельные роуты.
 */
class SettingsStore {
	compactView = $state(false);
	showFreeSpace = $state(true);

	/** Версия приложения (semver + короткий git-хэш билда), приходит с сервера. */
	version = $state('');
	/** Значение чекбокса «удалить вместе с данными» по умолчанию. */
	deleteWithData = $state(false);
	/** Лимит длины заметки в символах, приходит с сервера. */
	noteMaxLength = $state(0);

	day = $state<ShiftState>({ configured: false, enabled: false });
	night = $state<ShiftState>({ configured: false, enabled: false });

	/** Минуты с начала суток по местному времени; тикает, чтобы обновлять индикатор активной смены. */
	#nowMinutes = $state(new Date().getHours() * 60 + new Date().getMinutes());

	get activeShift(): ShiftName | null {
		if (isShiftActive(this.night, this.#nowMinutes)) return 'night';
		if (isShiftActive(this.day, this.#nowMinutes)) return 'day';
		return null;
	}

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
		setInterval(() => {
			const now = new Date();
			this.#nowMinutes = now.getHours() * 60 + now.getMinutes();
		}, ACTIVE_SHIFT_CHECK_INTERVAL);
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
				this.version = s.version;
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
