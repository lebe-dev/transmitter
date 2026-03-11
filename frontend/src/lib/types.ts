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
}

export type FilterStatus = 'all' | 'downloading' | 'seeding' | 'paused' | 'done';

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
