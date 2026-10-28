package transmission

import (
	"encoding/json"
	"errors"
)

// ErrTorrentNotFound is returned when no torrent matches the requested ID or hash.
var ErrTorrentNotFound = errors.New("torrent not found")

// RPCRequest is a Transmission JSON-RPC request.
type RPCRequest struct {
	Method    string          `json:"method"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
	Tag       int             `json:"tag,omitempty"`
}

// RPCResponse is a Transmission JSON-RPC response.
type RPCResponse struct {
	Result    string          `json:"result"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
	Tag       int             `json:"tag,omitempty"`
}

// Torrent holds fields returned by torrent-get.
type Torrent struct {
	ID           int64   `json:"id"`
	Name         string  `json:"name"`
	Status       int     `json:"status"`
	PercentDone  float64 `json:"percentDone"`
	TotalSize    int64   `json:"totalSize"`
	RateDownload int64   `json:"rateDownload"`
	RateUpload   int64   `json:"rateUpload"`
	AddedDate    int64   `json:"addedDate"`
	ETA          int64   `json:"eta"`
	HashString   string  `json:"hashString"`
	DownloadDir  string  `json:"downloadDir"`
	Error        int     `json:"error"`
	ErrorString  string  `json:"errorString"`
	UploadedEver int64   `json:"uploadedEver"`
	// DownloadedEver is the payload downloaded over the torrent's lifetime; it is
	// the denominator of UploadRatio.
	DownloadedEver int64 `json:"downloadedEver"`
	// UploadRatio is uploadedEver/downloadedEver as reported by Transmission.
	// It is negative when the ratio is unknown (nothing downloaded yet).
	UploadRatio float64  `json:"uploadRatio"`
	Labels      []string `json:"labels"`
}

// TorrentGetArgs are arguments for torrent-get.
type TorrentGetArgs struct {
	Fields []string `json:"fields"`
	IDs    []int64  `json:"ids,omitempty"`
}

// TorrentGetResult holds the result of torrent-get.
type TorrentGetResult struct {
	Torrents []Torrent `json:"torrents"`
}

// TorrentAddArgs are arguments for torrent-add.
type TorrentAddArgs struct {
	Filename string `json:"filename,omitempty"`
	Metainfo string `json:"metainfo,omitempty"`
	Paused   bool   `json:"paused,omitempty"`
}

// TorrentAdded holds the added/duplicate torrent info.
type TorrentAdded struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	HashString string `json:"hashString"`
}

// TorrentAddResult holds the result of torrent-add.
type TorrentAddResult struct {
	TorrentAdded     *TorrentAdded `json:"torrent-added,omitempty"`
	TorrentDuplicate *TorrentAdded `json:"torrent-duplicate,omitempty"`
}

// TorrentActionArgs are arguments for torrent-start/stop/remove.
type TorrentActionArgs struct {
	IDs             []int64 `json:"ids"`
	DeleteLocalData bool    `json:"delete-local-data,omitempty"`
}

// TorrentSetArgs are arguments for torrent-set.
type TorrentSetArgs struct {
	IDs           []int64   `json:"ids"`
	PriorityHigh  []int     `json:"priority-high,omitempty"`
	PriorityLow   []int     `json:"priority-low,omitempty"`
	FilesWanted   []int     `json:"files-wanted,omitempty"`
	FilesUnwanted []int     `json:"files-unwanted,omitempty"`
	Labels        *[]string `json:"labels,omitempty"`
}

// TorrentFile holds file info returned by torrent-get with "files" field.
type TorrentFile struct {
	Name   string `json:"name"`
	Length int64  `json:"length"`
}

// TorrentWithFiles holds torrent info with files list. HashString, Name and
// AddedDate are filled only when requested.
type TorrentWithFiles struct {
	ID         int64         `json:"id"`
	HashString string        `json:"hashString,omitempty"`
	Name       string        `json:"name,omitempty"`
	AddedDate  int64         `json:"addedDate,omitempty"`
	Files      []TorrentFile `json:"files"`
}

// TorrentWithFilesResult holds the result of torrent-get with files.
type TorrentWithFilesResult struct {
	Torrents []TorrentWithFiles `json:"torrents"`
}

// TorrentFields are the fields requested from Transmission for the torrent list.
var TorrentFields = []string{
	"id", "name", "status", "percentDone", "totalSize",
	"rateDownload", "rateUpload", "addedDate", "eta",
	"hashString", "downloadDir", "error", "errorString", "uploadedEver",
	"downloadedEver", "uploadRatio",
	"labels",
}
