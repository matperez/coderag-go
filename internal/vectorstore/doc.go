// Package vectorstore provides vector persistence and k-NN search for code chunks.
//
// The Store interface is implemented by LanceStore (LanceDB) when building with
// the "lancedb" build tag and the LanceDB native library; see
// https://github.com/lancedb/lancedb-go for CGO setup. MockStore is available
// for tests without LanceDB.
package vectorstore
