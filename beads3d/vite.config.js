import { defineConfig, loadEnv } from 'vite';

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '');
  const target = env.VITE_BD_API_URL || 'http://localhost:9080';
  const token = env.VITE_BD_TOKEN;

  // Shared proxy config: rewrite /api prefix, inject auth header
  const addAuth = (proxy) => {
    proxy.on('proxyReq', (proxyReq) => {
      if (token) proxyReq.setHeader('Authorization', `Bearer ${token}`);
    });
  };

  return {
    server: {
      port: 3333,
      host: true, // bd-gbxri: bind to all interfaces so beads3d.local works
      open: true,
      proxy: {
        // SSE streams — long-lived connections, must come before /api catch-all
        '/api/v1/events/stream': {
          target,
          changeOrigin: true,
          rewrite: (path) => path.replace(/^\/api/, ''),
          timeout: 0,       // no timeout for SSE
          configure: addAuth,
        },
        '/api/events': {
          target,
          changeOrigin: true,
          rewrite: (path) => path.replace(/^\/api/, ''),
          timeout: 0,       // no timeout for SSE
          configure: addAuth,
        },
        '/api/bus/': {
          target,
          changeOrigin: true,
          rewrite: (path) => path.replace(/^\/api/, ''),
          timeout: 0,
          configure: addAuth,
        },
        // API catch-all — short-lived request/response (RPC or REST)
        '/api': {
          target,
          changeOrigin: true,
          rewrite: (path) => path.replace(/^\/api/, ''),
          configure: addAuth,
        },
      },
    },
  };
});
