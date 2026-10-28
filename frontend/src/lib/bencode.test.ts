import { describe, it, expect } from 'vitest';
import { parseTorrentSize } from './bencode.js';

// ── Minimal bencode encoder used only to build test fixtures ─────────────────

type Encodable = number | Uint8Array | string | Encodable[] | { [key: string]: Encodable };

const enc = new TextEncoder();

function concat(chunks: Uint8Array[]): Uint8Array {
	const total = chunks.reduce((n, c) => n + c.length, 0);
	const out = new Uint8Array(total);
	let pos = 0;
	for (const c of chunks) {
		out.set(c, pos);
		pos += c.length;
	}
	return out;
}

function encode(value: Encodable): Uint8Array {
	if (typeof value === 'number') return enc.encode(`i${value}e`);
	if (typeof value === 'string') {
		const bytes = enc.encode(value);
		return concat([enc.encode(`${bytes.length}:`), bytes]);
	}
	if (value instanceof Uint8Array) {
		return concat([enc.encode(`${value.length}:`), value]);
	}
	if (Array.isArray(value)) {
		return concat([enc.encode('l'), ...value.map(encode), enc.encode('e')]);
	}
	// dict — bencode requires keys sorted lexicographically
	const keys = Object.keys(value).sort();
	const parts: Uint8Array[] = [enc.encode('d')];
	for (const key of keys) {
		parts.push(encode(key), encode(value[key]));
	}
	parts.push(enc.encode('e'));
	return concat(parts);
}

function buf(value: Encodable): ArrayBuffer {
	const bytes = encode(value);
	return bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength) as ArrayBuffer;
}

function rawBuf(s: string): ArrayBuffer {
	const bytes = enc.encode(s);
	return bytes.buffer.slice(0, bytes.byteLength) as ArrayBuffer;
}

// ── Tests ────────────────────────────────────────────────────────────────────

describe('parseTorrentSize', () => {
	it('returns info.length for a single-file torrent', () => {
		const t = { announce: 'http://tracker/', info: { name: 'file.iso', length: 123456 } };
		expect(parseTorrentSize(buf(t))).toBe(123456);
	});

	it('sums info.files[].length for a multi-file torrent', () => {
		const t = {
			info: {
				name: 'folder',
				files: [
					{ length: 100, path: ['a.txt'] },
					{ length: 250, path: ['sub', 'b.txt'] },
					{ length: 650, path: ['c.txt'] },
				],
			},
		};
		expect(parseTorrentSize(buf(t))).toBe(1000);
	});

	it('handles an empty files list as zero', () => {
		const t = { info: { name: 'folder', files: [] } };
		expect(parseTorrentSize(buf(t))).toBe(0);
	});

	it('skips file entries without a numeric length', () => {
		const files: Encodable[] = [
			{ length: 100, path: ['a'] },
			{ path: ['b'] }, // no length
			{ length: 'bad', path: ['c'] }, // non-numeric length
			{ length: 400, path: ['d'] },
		];
		const t: Encodable = { info: { files } };
		expect(parseTorrentSize(buf(t))).toBe(500);
	});

	it('prefers info.length when both length and files are present', () => {
		const t = { info: { length: 999, files: [{ length: 100, path: ['a'] }] } };
		expect(parseTorrentSize(buf(t))).toBe(999);
	});

	it('is unaffected by binary values such as pieces', () => {
		const pieces = new Uint8Array(60);
		for (let i = 0; i < pieces.length; i++) pieces[i] = i % 256;
		const t = {
			info: { name: 'file', length: 42, 'piece length': 16384, pieces },
		};
		expect(parseTorrentSize(buf(t))).toBe(42);
	});

	it('parses binary bytes that collide with bencode markers (e, i, l, d)', () => {
		// 0x65='e' 0x69='i' 0x6c='l' 0x64='d' — must be read as string bytes, not markers
		const tricky = new Uint8Array([0x65, 0x69, 0x6c, 0x64, 0x65, 0x65]);
		const t = { info: { length: 7, pieces: tricky } };
		expect(parseTorrentSize(buf(t))).toBe(7);
	});

	it('returns null when info is missing', () => {
		expect(parseTorrentSize(buf({ announce: 'http://x/' }))).toBeNull();
	});

	it('returns null when info has neither length nor files', () => {
		expect(parseTorrentSize(buf({ info: { name: 'x' } }))).toBeNull();
	});

	it('returns null when info is not a dict', () => {
		expect(parseTorrentSize(buf({ info: 5 }))).toBeNull();
		expect(parseTorrentSize(buf({ info: 'str' }))).toBeNull();
		expect(parseTorrentSize(buf({ info: [1, 2] }))).toBeNull();
	});

	it('returns null when the root is not a dict', () => {
		expect(parseTorrentSize(buf(42))).toBeNull();
		expect(parseTorrentSize(buf('hello'))).toBeNull();
		expect(parseTorrentSize(buf([1, 2, 3]))).toBeNull();
	});

	it('returns null for an empty buffer', () => {
		expect(parseTorrentSize(new ArrayBuffer(0))).toBeNull();
	});

	it('returns null on malformed input', () => {
		expect(parseTorrentSize(rawBuf('d'))).toBeNull(); // unterminated dict
		expect(parseTorrentSize(rawBuf('l'))).toBeNull(); // unterminated list
		expect(parseTorrentSize(rawBuf('i42'))).toBeNull(); // unterminated number
		expect(parseTorrentSize(rawBuf('ie'))).toBeNull(); // number with no digits
		expect(parseTorrentSize(rawBuf('5:abc'))).toBeNull(); // string overruns buffer
		expect(parseTorrentSize(rawBuf(':abc'))).toBeNull(); // no length prefix
		expect(parseTorrentSize(rawBuf('iabce'))).toBeNull(); // bad digit
		expect(parseTorrentSize(rawBuf('garbage'))).toBeNull();
	});

	it('parses a negative integer length (documents current behavior)', () => {
		expect(parseTorrentSize(rawBuf('d4:infod6:lengthi-5eee'))).toBe(-5);
	});

	it('handles multi-byte UTF-8 keys and names', () => {
		const t = { info: { name: 'фильм.mkv', length: 555 } };
		expect(parseTorrentSize(buf(t))).toBe(555);
	});

	it('handles large sizes beyond 32-bit range', () => {
		const big = 8_000_000_000; // 8 GB, > 2^32
		const t = { info: { name: 'big.bin', length: big } };
		expect(parseTorrentSize(buf(t))).toBe(big);
	});
});
