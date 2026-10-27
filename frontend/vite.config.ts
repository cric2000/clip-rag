import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";
export default defineConfig({
  server: {
    host: "127.0.0.1",
    port: Number(process.env.WAILS_VITE_PORT) || 5173,
    strictPort: true,
  },
  plugins: [
    react(),
    tailwindcss(),
    {
      name: "cliprag-development-csp",
      apply: "serve",
      // React refresh requires an inline preamble and a loopback WebSocket.
      // This transform never applies to the packaged production index.
      transformIndexHtml: (html: string) =>
        html
          .replace("script-src 'self'", "script-src 'self' 'unsafe-inline'")
          .replace(
            "connect-src 'self'",
            "connect-src 'self' ws://localhost:* ws://127.0.0.1:*",
          ),
    },
  ],
  test: {
    environment: "jsdom",
    setupFiles: "./src/test-setup.ts",
    restoreMocks: true,
  },
});
