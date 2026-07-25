/// <reference types="vitest/config" />
import { defineConfig, loadEnv } from 'vite'
import react from '@vitejs/plugin-react'

// The deployed Go backend. Overridable with BACKEND_ORIGIN in .env.local when
// pointing at a locally running `make run`.
const DEFAULT_BACKEND = 'https://crucible-backend-103350253775.us-central1.run.app'

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '')
  const backend = env.BACKEND_ORIGIN || DEFAULT_BACKEND

  return {
    plugins: [react()],

    server: {
      port: 5173,

      // The backend ships no CORS middleware and registers no OPTIONS routes,
      // so a browser on a different origin is blocked on every /v1 call. The
      // WebSocket is unaffected (handshakes skip CORS), which makes the symptom
      // confusing: the socket connects while every REST call fails.
      //
      // Proxying makes the browser see a single origin. Production does the same
      // job with the Firebase Hosting rewrite in firebase.json, so app code only
      // ever uses relative paths and the two environments cannot drift.
      proxy: {
        '/v1': {
          target: backend,
          changeOrigin: true,
          ws: true, // /v1/sessions/{id}/live
        },
        '/health': { target: backend, changeOrigin: true },
        '/readyz': { target: backend, changeOrigin: true },
      },
    },

    test: {
      environment: 'jsdom',
      include: ['src/**/*.test.{ts,tsx}'],
    },
  }
})
