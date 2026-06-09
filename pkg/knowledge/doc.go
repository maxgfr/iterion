// Package knowledge defines the backend-agnostic contract for
// iterion's shared memory / knowledge system: the MemoryStore
// interface, the SpaceRef identity model (the sharing axes), and the
// document/index value types both adapters return.
//
// Two adapters implement MemoryStore:
//   - the local filesystem adapter in pkg/memory (FSStore), used for
//     desktop / single-tenant runs; and
//   - the cloud adapter in pkg/store/mongo (Mongo metadata + blob
//     bodies), used in multi-tenant cloud mode.
//
// This package owns the interface so neither adapter leaks its storage
// shape into the contract, and so pkg/memory can import knowledge
// without an import cycle (knowledge imports neither adapter).
//
// The interface is grown deliberately, one capability per delivery
// phase: the runtime document/index methods land first (they are all
// the memory_read / memory_write / memory_list tools need), with
// quota accounting, space management, and export/import added by the
// phases that introduce them. Callers should program against the
// smallest method set they need.
//
// # Untrusted input
//
// Memory documents are operator/agent-authored data, NOT trusted
// instructions. A node that autoloads or reads memory must treat the
// contents as suggestions; the system prompt's secret-handling and
// operating-posture clauses always outrank anything a memory document
// says. Adapters and the tool-wiring layer mark injected memory blocks
// accordingly.
package knowledge
