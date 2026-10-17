/// <reference types="@sveltejs/kit" />
/// <reference no-default-lib="true"/>
/// <reference lib="esnext" />
/// <reference lib="webworker" />

// Кэширует оболочку приложения, чтобы UI открывался без связи с сервером.
// Данные (торренты, настройки, заметки) сюда не попадают: их последнее
// состояние хранится в localStorage (см. $lib/offline.ts), а /api всегда
// идёт в сеть.

import { base, build, files, version } from '$service-worker';

const sw = self as unknown as ServiceWorkerGlobalScope;

const CACHE = `transmitter-${version}`;
// Все роуты SPA отдаются сервером одним и тем же index.html.
const SHELL = `${base}/`;
const ASSETS = new Set([...build, ...files]);

sw.addEventListener('install', (event) => {
	event.waitUntil(
		caches
			.open(CACHE)
			.then((cache) => cache.addAll([...ASSETS, SHELL]))
			.then(() => sw.skipWaiting()),
	);
});

sw.addEventListener('activate', (event) => {
	event.waitUntil(
		caches
			.keys()
			.then((keys) => Promise.all(keys.filter((k) => k !== CACHE).map((k) => caches.delete(k))))
			.then(() => sw.clients.claim()),
	);
});

sw.addEventListener('fetch', (event) => {
	const { request } = event;
	if (request.method !== 'GET') return;

	const url = new URL(request.url);
	if (url.origin !== sw.location.origin || url.pathname.startsWith(`${base}/api/`)) return;

	if (request.mode === 'navigate') {
		event.respondWith(navigate(request));
	} else if (ASSETS.has(url.pathname)) {
		event.respondWith(cached(request));
	}
});

/** Сеть первой — чтобы сразу получать новую версию; без сети — сохранённый index.html. */
async function navigate(request: Request): Promise<Response> {
	const cache = await caches.open(CACHE);
	try {
		const response = await fetch(request);
		if (response.ok) await cache.put(SHELL, response.clone());
		return response;
	} catch {
		const shell = await cache.match(SHELL);
		if (shell) return shell;
		throw new Error('offline and no cached shell');
	}
}

/** Файлы сборки неизменяемы в пределах версии — отдаём из кэша. */
async function cached(request: Request): Promise<Response> {
	const cache = await caches.open(CACHE);
	const hit = await cache.match(request);
	if (hit) return hit;
	const response = await fetch(request);
	if (response.ok) await cache.put(request, response.clone());
	return response;
}
