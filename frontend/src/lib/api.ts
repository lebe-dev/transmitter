import type { Torrent, TorrentDetail } from './types.js';

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

export async function addTorrentMagnet(filename: string): Promise<void> {
	await rpc('torrent-add', { filename });
}

export async function addTorrentFile(metainfo: string): Promise<void> {
	await rpc('torrent-add', { metainfo });
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

export async function getTorrentDetails(id: number): Promise<TorrentDetail> {
	const data = await rpc<{ torrents: TorrentDetail[] }>('torrent-get', {
		ids: [id],
		fields: ['id', 'files', 'peers', 'trackerStats'],
	});
	return data.torrents[0];
}
