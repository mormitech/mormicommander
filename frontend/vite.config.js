// Plain Vite: one page, no framework. The tests (vitest) run the pure modules of src/core in Node; anything that needs the
// window is checked in the real app through control mode (docs/control.md).
export default {
  build: { target: 'es2022', chunkSizeWarningLimit: 1500 },
  test: { include: ['test/**/*.test.js'], environment: 'node' },
};
