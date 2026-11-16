import { defineConfig, loadEnv } from 'vite';
import react from '@vitejs/plugin-react';
import { readFileSync } from 'node:fs';

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '');
  const apiTarget = env.API_PROXY_TARGET || 'http://127.0.0.1:8080';
  return {
    plugins: [react()],
    server: {
      // Optional dev HTTPS (phones only allow the camera on https): set HTTPS_CERT and HTTPS_KEY file paths.
      https: env.HTTPS_CERT && env.HTTPS_KEY ? { cert: readFileSync(env.HTTPS_CERT), key: readFileSync(env.HTTPS_KEY) } : undefined,
      proxy: { '/api': { target: apiTarget, changeOrigin: false } },
    },
  };
});
