// Plain Vite: no framework, three pages. index.html is the file manager; viewer.html is a Lister window (F3), a real
// window of its own made by Go (feature_viewer_window.go); dialog.html is a dialog window, which shows a live copy of a
// dialog of another page (feature_dialog_window.go, src/core/mirror.js). All are served by the same asset server from
// dist/. The tests (vitest) run the pure modules of src/core in Node; anything that needs the window is checked in the
// real app through control mode (docs/control.md).
import { resolve } from 'node:path';

export default {
  build: {
    target: 'es2022', chunkSizeWarningLimit: 1500,
    rollupOptions: { input: { main: resolve(__dirname, 'index.html'), viewer: resolve(__dirname, 'viewer.html'), dialog: resolve(__dirname, 'dialog.html') } },
  },
  test: { include: ['test/**/*.test.js'], environment: 'node' },
};
