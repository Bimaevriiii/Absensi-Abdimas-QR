self.addEventListener('install', () => self.skipWaiting());
self.addEventListener('activate', event => event.waitUntil(self.clients.claim()));
// Kehadiran dan data akun selalu memerlukan jaringan. Tidak ada cache atau antrean offline.
