import { describe, expect, it } from 'vitest';
import { imdbUrl, matchesSearch, missingSubs, movieLabel, previousLink, tmdbUrl, unwatched } from './movie.js';
import type { MovieRecord } from './types.js';

function rec(patch: Partial<MovieRecord> = {}): MovieRecord {
	return { state: 'matched', manual: false, confident: true, hasPoster: false, watched: false, missingSubs: false, ...patch };
}

describe('movieLabel', () => {
	it('shows the title with the year', () => {
		expect(movieLabel(rec({ title: 'Интерстеллар', year: 2014 }))).toBe('Интерстеллар (2014)');
	});
	it('adds the season of a series pack', () => {
		expect(movieLabel(rec({ title: 'Lost', year: 2004, season: 2 }))).toBe('Lost (2004) · S02');
	});
	it('omits a missing year', () => {
		expect(movieLabel(rec({ title: 'Lost' }))).toBe('Lost');
	});
	it('is empty without a match', () => {
		expect(movieLabel(undefined)).toBe('');
		expect(movieLabel(rec({ state: 'unmatched', title: 'stale' }))).toBe('');
	});
});

describe('links', () => {
	it('builds TMDB and IMDb links', () => {
		const r = rec({ mediaType: 'tv', tmdbId: 1396, imdbId: 'tt0903747' });
		expect(tmdbUrl(r)).toBe('https://www.themoviedb.org/tv/1396');
		expect(imdbUrl(r)).toBe('https://www.imdb.com/title/tt0903747/');
	});
	it('returns null without ids', () => {
		expect(tmdbUrl(rec())).toBeNull();
		expect(imdbUrl(rec())).toBeNull();
	});
});

describe('filters', () => {
	it('counts only video torrents as unwatched', () => {
		expect(unwatched(rec())).toBe(true);
		expect(unwatched(rec({ state: 'unmatched' }))).toBe(true);
		expect(unwatched(rec({ watched: true }))).toBe(false);
		expect(unwatched(rec({ state: 'skipped' }))).toBe(false);
		expect(unwatched(rec({ state: '' }))).toBe(false);
		expect(unwatched(undefined)).toBe(false);
	});
	it('picks torrents marked as missing subtitles', () => {
		expect(missingSubs(rec({ missingSubs: true }))).toBe(true);
		expect(missingSubs(rec())).toBe(false);
		expect(missingSubs(undefined)).toBe(false);
	});
});

describe('matchesSearch', () => {
	it('finds by the localized or the original title', () => {
		const r = rec({ title: 'Интерстеллар', originalTitle: 'Interstellar' });
		expect(matchesSearch(r, 'интер')).toBe(true);
		expect(matchesSearch(r, 'stellar')).toBe(true);
		expect(matchesSearch(r, 'matrix')).toBe(false);
		expect(matchesSearch(undefined, 'x')).toBe(false);
	});
});

describe('previousLink', () => {
	it('returns the link the user applied', () => {
		expect(previousLink(rec({ manual: true, query: 'https://www.imdb.com/title/tt0816692/', tmdbId: 157336, mediaType: 'movie' }))).toBe(
			'https://www.imdb.com/title/tt0816692/',
		);
	});
	it('builds the TMDB link of a detected title', () => {
		expect(previousLink(rec({ query: 'Interstellar (2014)', tmdbId: 157336, mediaType: 'movie' }))).toBe(
			'https://www.themoviedb.org/movie/157336',
		);
	});
	it('is empty for a title that was not matched or was cleared', () => {
		expect(previousLink(undefined)).toBe('');
		expect(previousLink(rec({ state: 'unmatched', query: 'Interstellar' }))).toBe('');
		expect(previousLink(rec({ state: 'unmatched', manual: true, query: '' }))).toBe('');
	});
});
