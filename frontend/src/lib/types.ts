export type TorrentStatus = 0 | 1 | 2 | 3 | 4 | 5 | 6;

export interface Torrent {
	id: number;
	name: string;
	status: TorrentStatus;
	percentDone: number;
	totalSize: number;
	rateDownload: number;
	rateUpload: number;
	addedDate: number;
	eta: number;
	hashString: string;
	downloadDir: string;
	error: number;
	errorString: string;
	uploadedEver: number;
	downloadedEver: number;
	/** uploadedEver / downloadedEver по версии Transmission; отрицательное — соотношение неизвестно. */
	uploadRatio: number;
	labels: string[];
	/** Сколько байт осталось докачать (с учётом снятых файлов). */
	leftUntilDone?: number;
}

/**
 * «undergiven» — раздача скачала больше, чем отдала (impact ниже цели).
 * «unwatched» и «missingSubs» — фильтры Movie Plugin, есть только когда он включён.
 */
export type FilterStatus =
	| 'all'
	| 'downloading'
	| 'seeding'
	| 'paused'
	| 'done'
	| 'undergiven'
	| 'unwatched'
	| 'missingSubs';

export interface TorrentFile {
	bytesCompleted: number;
	length: number;
	name: string;
}

export interface TorrentPeer {
	address: string;
	clientName: string;
	rateToClient: number;
	rateToPeer: number;
	progress: number;
	isEncrypted: boolean;
	flagStr: string;
}

export interface TorrentTrackerStat {
	host: string;
	announce: string;
	lastAnnounceResult: string;
	lastAnnounceSucceeded: boolean;
	lastAnnounceTime: number;
	seederCount: number;
	leecherCount: number;
	tier: number;
	announceState: number;
}

export type FilePriority = -1 | 0 | 1;

export interface TorrentFileStat {
	bytesCompleted: number;
	wanted: boolean;
	priority: FilePriority;
}

export interface TorrentDetail {
	id: number;
	files: TorrentFile[];
	fileStats: TorrentFileStat[];
	peers: TorrentPeer[];
	trackerStats: TorrentTrackerStat[];
}

/**
 * Смена «configured» — её окно задано переменными окружения; «enabled» —
 * пользователь не выключил её в настройках.
 */
export interface UISettings {
	version: string;
	deleteWithData: boolean;
	nightShiftConfigured: boolean;
	nightShiftEnabled: boolean;
	nightShiftStart?: string;
	nightShiftEnd?: string;
	dayShiftConfigured: boolean;
	dayShiftEnabled: boolean;
	dayShiftStart?: string;
	dayShiftEnd?: string;
	noteMaxLength: number;
	/** Коэффициент отдачи, считающийся полным возвратом (1 = отдали столько, сколько скачали). */
	impactTargetRatio: number;
	/** Снимать ли с раздачи торренты, достигшие цели. */
	impactAutoStopEnabled: boolean;
	/** Movie Plugin включён переменной MOVIE_PLUGIN_ENABLED. */
	moviePluginEnabled: boolean;
	/** Удалять ли просмотренные раздачи, отдавшие цель (только при включённом Movie Plugin). */
	movieAutoRemoveEnabled?: boolean;
}

export type ShiftName = 'night' | 'day';

export interface ServerConfig {
	transmissionUrl: string;
	listenAddr: string;
	corsOrigin: string;
	maxRequestBodyBytes: number;
	webUiEnabled: boolean;
	telegramBotEnabled: boolean;
	telegramUsers: string[];
	logLevel: string;
	filePriorityEnabled: boolean;
	filePriorityHighCount: number;
	deleteWithData: boolean;
	monitorInterval: string;
	fileSelectTimeout: string;
	nightShiftConfigured: boolean;
	nightShiftStart?: string;
	nightShiftEnd?: string;
	dayShiftConfigured: boolean;
	dayShiftStart?: string;
	dayShiftEnd?: string;
	dbPath: string;
	noteMaxLength: number;
	noteCleanupInterval: string;
	impactTargetRatio: number;
	impactInterval: string;
	moviePluginEnabled: boolean;
	/** Есть только при включённом Movie Plugin; ключ TMDB сервер не отдаёт. */
	movie?: MovieServerConfig;
}

export interface MovieServerConfig {
	tmdbApiUrl: string;
	tmdbImageUrl: string;
	language: string;
	autoDetect: boolean;
	backfill: boolean;
	autoRename: boolean;
	renameTemplate: string;
	scanInterval: string;
	scanBatch: number;
	posterDir: string;
}

/** Torrent notes keyed by torrent hash. */
export type TorrentNotes = Record<string, string>;

/**
 * Состояние распознавания раздачи Movie Plugin:
 * '' — ещё не смотрели (у раздачи есть только отметки), matched — найден фильм/сериал,
 * unmatched — не нашли или пользователь сбросил, skipped — в раздаче нет видео,
 * failed — TMDB/сеть недоступны, сервер повторит позже.
 */
export type MovieState = '' | 'matched' | 'unmatched' | 'skipped' | 'failed';

export interface MovieRecord {
	state: MovieState;
	/** Совпадение задано ссылкой или сброшено вручную — фон его не трогает. */
	manual: boolean;
	/** Название и год совпали точно — только такие раздачи переименовываются сами. */
	confident: boolean;
	mediaType?: 'movie' | 'tv';
	tmdbId?: number;
	imdbId?: string;
	title?: string;
	originalTitle?: string;
	/** Английское название — по нему переименовывается раздача. */
	englishTitle?: string;
	year?: number;
	/** Сезон из имени раздачи сериала (S02); у фильмов нет. */
	season?: number;
	overview?: string;
	hasPoster: boolean;
	rating?: number;
	/** Что искали в TMDB (или ссылка, если задано вручную). */
	query?: string;
	/** Имя, в которое раздачу переименовали на диске. */
	renamedTo?: string;
	watched: boolean;
	missingSubs: boolean;
}

/** Movie Plugin records keyed by torrent hash. */
export type MovieRecords = Record<string, MovieRecord>;
