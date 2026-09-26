// Package storagetest provides test doubles for package storage: an
// in-memory storage.Engine, and the contract tests every Engine
// implementation must pass. Running the same contract against the fake and
// against Pebble keeps the fake honest (docs/engineering/testing.md §7).
package storagetest
