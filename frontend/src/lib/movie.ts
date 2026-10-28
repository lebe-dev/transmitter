import type { MovieRecord } from './types.js';

/** Диалоги Movie Plugin, которые открываются из меню карточки. */
export type MovieDialogKind = 'info' | 'link' | 'rename';

/** «Интерстеллар (2014)», «Lost (2004) · S02» — как фильм подписан на карточке. */
export function movieLabel(rec: MovieRecord | undefined): string {
	if (!rec || rec.state !== 'matched' || !rec.title) return '';
	const label = rec.year ? `${rec.title} (${rec.year})` : rec.title;
	return rec.season ? `${label} · S${String(rec.season).padStart(2, '0')}` : label;
}

export function tmdbUrl(rec: MovieRecord): string | null {
	if (!rec.tmdbId || !rec.mediaType) return null;
	return `https://www.themoviedb.org/${rec.mediaType}/${rec.tmdbId}`;
}

export function imdbUrl(rec: MovieRecord): string | null {
	return rec.imdbId ? `https://www.imdb.com/title/${rec.imdbId}/` : null;
}

/** Раздача с видео: распознана или хотя бы искалась (а не пропущена как не-видео). */
function isVideo(rec: MovieRecord | undefined): boolean {
	return rec?.state === 'matched' || rec?.state === 'unmatched';
}

/** Фильтр «Не просмотрено»: видео без отметки «просмотрено». */
export function unwatched(rec: MovieRecord | undefined): boolean {
	return isVideo(rec) && !rec!.watched;
}

export function missingSubs(rec: MovieRecord | undefined): boolean {
	return rec?.missingSubs === true;
}

/** Совпадает ли поисковая строка (уже в нижнем регистре) с распознанным названием. */
export function matchesSearch(rec: MovieRecord | undefined, query: string): boolean {
	if (!rec || rec.state !== 'matched') return false;
	return [rec.title, rec.originalTitle].some((t) => t?.toLowerCase().includes(query));
}
