// Minimal bencode reader — just enough to compute a torrent's total size
// (info.length for single-file torrents, or the sum of info.files[].length
// for multi-file ones). Binary string values (e.g. `pieces`) are kept as raw
// byte slices and never decoded, so large .torrent files stay cheap to parse.

const KEY_DECODER = new TextDecoder('utf-8', { fatal: false });

type BValue = number | Uint8Array | BValue[] | { [key: string]: BValue };

function parse(data: Uint8Array): BValue {
	let pos = 0;

	// Reads ASCII digits until `terminator`. Throws on unexpected bytes or a
	// missing terminator so malformed input fails fast instead of looping.
	function readNumber(terminator: number): number {
		let negative = false;
		if (data[pos] === 0x2d /* - */) {
			negative = true;
			pos++;
		}
		let n = 0;
		let digits = 0;
		while (pos < data.length && data[pos] !== terminator) {
			const d = data[pos] - 0x30;
			if (d < 0 || d > 9) throw new Error('bad digit');
			n = n * 10 + d;
			pos++;
			digits++;
		}
		if (pos >= data.length || digits === 0) throw new Error('unterminated number');
		pos++; // consume terminator
		return negative ? -n : n;
	}

	function readString(): Uint8Array {
		let len = 0;
		let digits = 0;
		while (pos < data.length && data[pos] !== 0x3a /* : */) {
			const d = data[pos] - 0x30;
			if (d < 0 || d > 9) throw new Error('bad length prefix');
			len = len * 10 + d;
			pos++;
			digits++;
		}
		if (pos >= data.length || digits === 0) throw new Error('unterminated string length');
		pos++; // consume ':'
		if (pos + len > data.length) throw new Error('string overruns buffer');
		const slice = data.subarray(pos, pos + len);
		pos += len;
		return slice;
	}

	function readValue(): BValue {
		if (pos >= data.length) throw new Error('unexpected end of input');
		const marker = data[pos];
		if (marker === 0x69 /* i */) {
			pos++;
			return readNumber(0x65 /* e */);
		}
		if (marker === 0x6c /* l */) {
			pos++;
			const list: BValue[] = [];
			while (pos < data.length && data[pos] !== 0x65 /* e */) list.push(readValue());
			if (pos >= data.length) throw new Error('unterminated list');
			pos++;
			return list;
		}
		if (marker === 0x64 /* d */) {
			pos++;
			const dict: { [key: string]: BValue } = {};
			while (pos < data.length && data[pos] !== 0x65 /* e */) {
				const key = KEY_DECODER.decode(readString());
				dict[key] = readValue();
			}
			if (pos >= data.length) throw new Error('unterminated dict');
			pos++;
			return dict;
		}
		return readString();
	}

	return readValue();
}

// Returns the total size in bytes, or null when the buffer isn't a parseable
// .torrent — callers should treat null as "unknown" and not block on it.
export function parseTorrentSize(buffer: ArrayBuffer): number | null {
	try {
		const root = parse(new Uint8Array(buffer));
		if (typeof root !== 'object' || root === null || Array.isArray(root) || root instanceof Uint8Array) {
			return null;
		}
		const info = (root as Record<string, BValue>).info;
		if (typeof info !== 'object' || info === null || Array.isArray(info) || info instanceof Uint8Array) {
			return null;
		}
		const infoDict = info as Record<string, BValue>;

		if (typeof infoDict.length === 'number') return infoDict.length;

		if (Array.isArray(infoDict.files)) {
			let total = 0;
			for (const file of infoDict.files) {
				if (typeof file === 'object' && file !== null && !Array.isArray(file) && !(file instanceof Uint8Array)) {
					const len = (file as Record<string, BValue>).length;
					if (typeof len === 'number') total += len;
				}
			}
			return total;
		}

		return null;
	} catch {
		return null;
	}
}
