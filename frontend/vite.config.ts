import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// Le frontend est servi depuis une autre origine que l'API. En developpement,
// Vite mandate /v1 vers le backend, ce qui evite d'avoir a configurer CORS
// pour travailler. En production, l'origine de l'API est passee par
// VITE_API_URL et CORS_ORIGINS doit l'autoriser cote serveur.
export default defineConfig({
  plugins: [react()],
  server: {
    port: 5173,
    proxy: {
      '/v1': { target: 'http://localhost:3000', changeOrigin: true },
      '/health': { target: 'http://localhost:3000', changeOrigin: true },
    },
  },
})
