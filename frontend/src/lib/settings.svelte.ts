import { getSettings, getServerConfig, setImpactAutoStop, setShiftEnabled } from './api.js';
import { connectivity } from './connectivity.svelte.js';
import { readCache, writeCache } from './offline.js';
import type { ServerConfig, ShiftName, UISettings } from './types.js';

const COMPACT_STORAGE_KEY = 'transmitter-compact';
const FREE_SPACE_STORAGE_KEY = 'transmitter-show-free-space';
const SETTINGS_CACHE_KEY = 'settings';
const SERVER_CONFIG_CACHE_KEY = 'server_config';

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
	/** Коэффициент отдачи, считающийся полным возвратом (impact), приходит с сервера. */
	impactTargetRatio = $state(1);
	/** Снимать ли раздачу автоматически, когда она отдала цель. */
	impactAutoStopEnabled = $state(false);

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
		// Последние серверные настройки: нужны в офлайне и до ответа /api/settings.
		const cached = readCache<UISettings>(SETTINGS_CACHE_KEY);
		if (cached) this.#apply(cached.data);
		this.serverConfig = readCache<ServerConfig>(SERVER_CONFIG_CACHE_KEY)?.data ?? null;
		connectivity.onReconnect(() => {
			// Без связи load() отдал кэш и сбросил промис — теперь берём свежие.
			if (this.#settings === null) this.load().catch(() => {});
		});
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

	/**
	 * Грузит /api/settings один раз за сессию; повторные вызовы разделяют промис.
	 * Без связи отдаёт последние сохранённые настройки, если они есть.
	 */
	load(): Promise<UISettings> {
		this.#settings ??= getSettings()
			.then((s) => {
				writeCache(SETTINGS_CACHE_KEY, s);
				this.#apply(s);
				return s;
			})
			.catch((err) => {
				// Разрешаем повторную попытку на следующем заходе.
				this.#settings = null;
				const cached = readCache<UISettings>(SETTINGS_CACHE_KEY);
				if (cached) return cached.data;
				throw err;
			});
		return this.#settings;
	}

	#apply(s: UISettings) {
		this.version = s.version;
		this.deleteWithData = s.deleteWithData;
		this.noteMaxLength = s.noteMaxLength;
		this.impactTargetRatio = s.impactTargetRatio;
		this.impactAutoStopEnabled = s.impactAutoStopEnabled;
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
	}

	/** Включает/выключает планировщик смены, откатывая переключатель при ошибке. */
	async setShiftEnabled(shift: ShiftName, enabled: boolean) {
		const target = shift === 'night' ? this.night : this.day;
		const previous = target.enabled;
		target.enabled = enabled;
		try {
			await setShiftEnabled(shift, enabled);
			this.#patchCache(shift === 'night' ? { nightShiftEnabled: enabled } : { dayShiftEnabled: enabled });
		} catch (err) {
			target.enabled = previous;
			throw err;
		}
	}

	/** Включает/выключает авто-снятие с раздачи, откатывая переключатель при ошибке. */
	async setImpactAutoStopEnabled(enabled: boolean) {
		const previous = this.impactAutoStopEnabled;
		this.impactAutoStopEnabled = enabled;
		try {
			await setImpactAutoStop(enabled);
			this.#patchCache({ impactAutoStopEnabled: enabled });
		} catch (err) {
			this.impactAutoStopEnabled = previous;
			throw err;
		}
	}

	/** Переключатели меняются без перезагрузки настроек — держим кэш в актуальном виде. */
	#patchCache(patch: Partial<UISettings>) {
		const cached = readCache<UISettings>(SETTINGS_CACHE_KEY);
		if (cached) writeCache(SETTINGS_CACHE_KEY, { ...cached.data, ...patch });
	}

	#serverConfigFresh = false;

	async loadServerConfig() {
		if (this.#serverConfigFresh || this.serverConfigLoading) return;
		// Кэш показываем сразу, без спиннера, и тихо обновляем поверх.
		this.serverConfigLoading = this.serverConfig === null;
		this.serverConfigError = false;
		try {
			this.serverConfig = await getServerConfig();
			this.#serverConfigFresh = true;
			writeCache(SERVER_CONFIG_CACHE_KEY, this.serverConfig);
		} catch {
			this.serverConfigError = this.serverConfig === null;
		} finally {
			this.serverConfigLoading = false;
		}
	}
}

export const settingsStore = new SettingsStore();
