import { configure } from "@testing-library/react";

// The first render in a file is cold, and on a busy machine it passed testing-library's one
// second to wait for something that was on its way.
configure({ asyncUtilTimeout: 5000 });

/** jsdom lays nothing out, and has no ResizeObserver to say so. */
globalThis.ResizeObserver ??= class {
  observe() {}
  unobserve() {}
  disconnect() {}
};
