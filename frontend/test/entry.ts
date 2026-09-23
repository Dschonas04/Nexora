// The entry point the probe bundles.
//
// Yjs may only lie in memory ONCE: it checks objects through constructors, and
// two copies lead to errors that look like a bug in the program. That is why
// this module hands out the same copy the wire uses, instead of letting the
// probe pull a second one.
export { Leitung } from "../src/mitschrift";
export * as Y from "yjs";
