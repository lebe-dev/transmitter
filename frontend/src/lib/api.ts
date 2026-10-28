import type {
	MovieRecord,
	MovieRecords,
	ShiftName,
	Torrent,
	TorrentDetail,
	TorrentFile,
	TorrentNotes,
	UISettings,
	ServerConfig,
} from './types.js';
import { connectivity } from './connectivity.svelte.js';
import { OfflineError } from './offline.js';

const TORRENT_FIELDS = [
	'id',
	'name',
	'status',
	'percentDone',
	'totalSize',
	'rateDownload',
	'rateUpload',
	'addedDate',
	'eta',
	'hashString',
	'downloadDir',
	'error',
	'errorString',
	'uploadedEver',
	'downloadedEver',
	'uploadRatio',
	'labels',
	'leftUntilDone',
];

/** fetch, который отличает «сервер недоступен» от ошибок самого сервера. */
async function request(input: string, init?: RequestInit): Promise<Response> {
	let res: Response;
	try {
		res = await fetch(input, init);
	} catch {
		connectivity.markOffline();
		throw new OfflineError();
	}
	connectivity.markOnline();
	return res;
}

async function rpc<T>(method: string, args?: Record<string, unknown>): Promise<T> {
	const res = await request('/api/rpc', {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ method, arguments: args }),
	});
	if (!res.ok) throw new Error(`HTTP ${res.status}`);
	const data = await res.json();
	if (data.result !== 'success') throw new Error(data.result);
	return data.arguments as T;
}

export async function getTorrents(): Promise<Torrent[]> {
	const data = await rpc<{ torrents: Torrent[] }>('torrent-get', { fields: TORRENT_FIELDS });
	return data.torrents;
}

export async function getSession(): Promise<{
	'download-dir': string;
	'download-dir-free-space': number;
}> {
	return rpc('session-get');
}

export async function getFreeSpace(path: string): Promise<number> {
	const data = await rpc<{ 'size-bytes': number }>('free-space', { path });
	return data['size-bytes'];
}

export async function addTorrentMagnet(
	filename: string,
	downloadDir?: string,
	paused?: boolean,
): Promise<void> {
	const args: Record<string, unknown> = { filename };
	if (downloadDir) args['download-dir'] = downloadDir;
	if (paused) args['paused'] = true;
	await rpc('torrent-add', args);
}

export async function addTorrentFile(
	metainfo: string,
	downloadDir?: string,
	paused?: boolean,
): Promise<{ id: number; name: string; duplicate: boolean }> {
	const args: Record<string, unknown> = { metainfo };
	if (downloadDir) args['download-dir'] = downloadDir;
	if (paused) args['paused'] = true;
	const data = await rpc<{
		'torrent-added'?: { id: number; name: string };
		'torrent-duplicate'?: { id: number; name: string };
	}>('torrent-add', args);
	const added = data['torrent-added'];
	const duplicate = data['torrent-duplicate'];
	const result = added ?? duplicate;
	if (!result) throw new Error('Unexpected empty torrent-add result');
	return { ...result, duplicate: !added };
}

export async function startTorrents(ids: number[]): Promise<void> {
	await rpc('torrent-start', { ids });
}

export async function stopTorrents(ids: number[]): Promise<void> {
	await rpc('torrent-stop', { ids });
}

export async function removeTorrents(ids: number[], deleteLocalData: boolean): Promise<void> {
	await rpc('torrent-remove', { ids, 'delete-local-data': deleteLocalData });
}

export async function getTorrentFiles(id: number): Promise<TorrentFile[]> {
	const data = await rpc<{ torrents: Array<{ files: TorrentFile[] }> }>('torrent-get', {
		ids: [id],
		fields: ['id', 'files'],
	});
	return data.torrents[0]?.files ?? [];
}

export async function getTorrentDetails(id: number): Promise<TorrentDetail> {
	const data = await rpc<{ torrents: TorrentDetail[] }>('torrent-get', {
		ids: [id],
		fields: ['id', 'files', 'fileStats', 'peers', 'trackerStats'],
	});
	return data.torrents[0];
}

export async function setFilesWanted(
	torrentId: number,
	wantedIndices: number[],
	unwantedIndices: number[],
): Promise<void> {
	const args: Record<string, unknown> = { ids: [torrentId] };
	if (wantedIndices.length > 0) args['files-wanted'] = wantedIndices;
	if (unwantedIndices.length > 0) args['files-unwanted'] = unwantedIndices;
	await rpc('torrent-set', args);
}

export async function getSettings(): Promise<UISettings> {
	const res = await request('/api/settings');
	if (!res.ok) throw new Error(`HTTP ${res.status}`);
	return res.json();
}

export async function getServerConfig(): Promise<ServerConfig> {
	const res = await request('/api/config');
	if (!res.ok) throw new Error(`HTTP ${res.status}`);
	return res.json();
}

/** Включает или выключает планировщик смены; состояние хранится на сервере. */
export async function setShiftEnabled(shift: ShiftName, enabled: boolean): Promise<void> {
	const res = await request(`/api/shifts/${shift}`, {
		method: 'PUT',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ enabled }),
	});
	if (!res.ok) throw new Error(`HTTP ${res.status}`);
}

/** Включает или выключает авто-снятие раздач, отдавших целевой процент. */
export async function setImpactAutoStop(enabled: boolean): Promise<void> {
	const res = await request('/api/impact', {
		method: 'PUT',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ enabled }),
	});
	if (!res.ok) throw new Error(`HTTP ${res.status}`);
}

/** Включает или выключает удаление просмотренных раздач, отдавших цель. */
export async function setMovieAutoRemove(enabled: boolean): Promise<void> {
	const res = await request('/api/movies/autoremove', {
		method: 'PUT',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ enabled }),
	});
	if (!res.ok) throw new Error(`HTTP ${res.status}`);
}

export async function getNotes(): Promise<TorrentNotes> {
	const res = await request('/api/notes');
	if (!res.ok) throw new Error(`HTTP ${res.status}`);
	return res.json();
}

/** Stores the note of a torrent; an empty text removes it. */
export async function setNote(hashString: string, text: string): Promise<void> {
	const res = await request(`/api/notes/${hashString}`, {
		method: 'PUT',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ text }),
	});
	if (!res.ok) throw new Error(`HTTP ${res.status}`);
}

export async function deleteNote(hashString: string): Promise<void> {
	const res = await request(`/api/notes/${hashString}`, { method: 'DELETE' });
	if (!res.ok) throw new Error(`HTTP ${res.status}`);
}

export async function setTorrentLabels(torrentId: number, labels: string[]): Promise<void> {
	await setTorrentsLabels([torrentId], labels);
}

/** Назначает один и тот же набор меток всем перечисленным торрентам. */
export async function setTorrentsLabels(ids: number[], labels: string[]): Promise<void> {
	if (ids.length === 0) return;
	await rpc('torrent-set', { ids, labels });
}

export async function setFilePriority(
	torrentId: number,
	fileIndex: number,
	priority: 'low' | 'normal' | 'high',
): Promise<void> {
	await rpc('torrent-set', {
		ids: [torrentId],
		[`priority-${priority}`]: [fileIndex],
	});
}

/** Глобальная заметка над списком торрентов. */
export async function getMemo(): Promise<string> {
	const res = await request('/api/memo');
	if (!res.ok) throw new Error(`HTTP ${res.status}`);
	const data: { text: string } = await res.json();
	return data.text;
}

export async function setMemo(text: string): Promise<void> {
	const res = await request('/api/memo', {
		method: 'PUT',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ text }),
	});
	if (!res.ok) throw new Error(`HTTP ${res.status}`);
}

export async function getGlobalPause(): Promise<boolean> {
	const res = await request('/api/pause');
	if (!res.ok) throw new Error(`HTTP ${res.status}`);
	const data: { paused: boolean } = await res.json();
	return data.paused;
}

/**
 * Ставит все раздачи на паузу или снимает с неё. Сервер запоминает, какие
 * раздачи работали до паузы, и при снятии запускает только их.
 * Возвращает, сколько раздач было остановлено/запущено.
 */
export async function setGlobalPause(paused: boolean): Promise<number> {
	const res = await request('/api/pause', {
		method: 'PUT',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ paused }),
	});
	if (!res.ok) throw new Error(`HTTP ${res.status}`);
	const data: { count: number } = await res.json();
	return data.count;
}

// ── Movie Plugin ─────────────────────────────────────────────────────────

/** Достаёт текст ошибки из {"error": ...}, иначе — HTTP-статус. */
async function failure(res: Response): Promise<Error> {
	try {
		const data: { error?: string } = await res.json();
		if (data.error) return new Error(data.error);
	} catch {
		// not JSON
	}
	return new Error(`HTTP ${res.status}`);
}

async function movieRequest<T>(path: string, init?: RequestInit): Promise<T> {
	const res = await request(`/api/movies${path}`, init);
	if (!res.ok) throw await failure(res);
	return res.json();
}

function jsonBody(method: string, body: unknown): RequestInit {
	return { method, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) };
}

export function getMovies(): Promise<MovieRecords> {
	return movieRequest('');
}

/** Заново распознаёт фильм по имени раздачи. */
export function detectMovie(hashString: string): Promise<MovieRecord> {
	return movieRequest(`/${hashString}/detect`, { method: 'POST' });
}

/** Привязывает раздачу к фильму по ссылке на TMDB или IMDb. */
export function linkMovie(hashString: string, url: string): Promise<MovieRecord> {
	return movieRequest(`/${hashString}/link`, jsonBody('PUT', { url }));
}

/** Сбрасывает найденный фильм; фон больше не будет искать его сам. */
export function clearMovie(hashString: string): Promise<MovieRecord> {
	return movieRequest(`/${hashString}/match`, { method: 'DELETE' });
}

export function setMovieFlags(
	hashString: string,
	flags: { watched?: boolean; missingSubs?: boolean },
): Promise<MovieRecord> {
	return movieRequest(`/${hashString}/flags`, jsonBody('PUT', flags));
}

export function getRenamePreview(hashString: string): Promise<{ current: string; target: string }> {
	return movieRequest(`/${hashString}/rename`);
}

/** Переименовывает папку (или единственный файл) раздачи на диске; возвращает новое имя. */
export async function renameMovie(hashString: string): Promise<string> {
	const data = await movieRequest<{ name: string }>(`/${hashString}/rename`, { method: 'POST' });
	return data.name;
}

/** URL обложки; id фильма в query сбрасывает кэш браузера при смене фильма. */
export function moviePosterUrl(hashString: string, tmdbId: number | undefined): string {
	return `/api/movies/${hashString}/poster?v=${tmdbId ?? 0}`;
}
