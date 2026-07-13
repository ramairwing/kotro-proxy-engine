//! Context deduplication for multi-turn LLM conversations.
//!
//! Strips unchanged MCP tool schemas and repeated file/directory blocks from
//! subsequent conversation turns so they don't re-consume context window tokens.
//!
//! ## How it works
//!
//! On each turn the compressor maintains a session fingerprint of which content
//! blocks have already been sent. Any block whose fingerprint matches a
//! previously-seen one is stripped from the outgoing request. The upstream LLM
//! never sees the duplicate; the response is returned verbatim.
//!
//! This is entirely local — no content leaves the machine for this step.

mod fingerprint;
mod shrink;

pub use fingerprint::ContentFingerprint;
pub use shrink::{Compressor, CompressorSession};
