type Translator = (id: string) => string;

const SIZE_KEYS = ['units.b', 'units.kb', 'units.mb', 'units.gb', 'units.tb'] as const;

export function formatSize(bytes: number, t: Translator): string {
	if (bytes <= 0) return `0 ${t('units.b')}`;
	const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), SIZE_KEYS.length - 1);
	return `${(bytes / 1024 ** i).toFixed(1)} ${t(SIZE_KEYS[i])}`;
}

export function formatSpeed(bps: number, t: Translator): string {
	if (bps <= 0) return '';
	return `${formatSize(bps, t)}/${t('units.perSec')}`;
}

export function formatEta(secs: number, t: Translator): string {
	if (secs < 0) return t('eta.infinity');
	if (secs === 0) return '';
	const h = Math.floor(secs / 3600);
	const m = Math.floor((secs % 3600) / 60);
	const s = secs % 60;
	if (h > 0) return `${h}${t('eta.h')} ${m}${t('eta.m')}`;
	if (m > 0) return `${m}${t('eta.m')} ${s}${t('eta.s')}`;
	return `${s}${t('eta.s')}`;
}

export function formatDate(ts: number, locale: string | null | undefined): string {
	return new Date(ts * 1000).toLocaleDateString(locale ?? undefined, {
		month: 'short',
		day: 'numeric',
	});
}
