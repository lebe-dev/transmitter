import type { Torrent, TorrentDetail, TorrentFile, UISettings } from './types.js';

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
];

async function rpc<T>(method: string, args?: Record<string, unknown>): Promise<T> {
	const res = await fetch('/api/rpc', {
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

export async function getSession(): Promise<{ 'download-dir': string }> {
	return rpc('session-get');
}

export async function addTorrentMagnet(filename: string, downloadDir?: string): Promise<void> {
	const args: Record<string, unknown> = { filename };
	if (downloadDir) args['download-dir'] = downloadDir;
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
	const res = await fetch('/api/settings');
	if (!res.ok) throw new Error(`HTTP ${res.status}`);
	return res.json();
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
