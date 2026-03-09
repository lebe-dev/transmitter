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
}

export type FilterStatus = 'all' | 'downloading' | 'seeding' | 'paused' | 'done';
