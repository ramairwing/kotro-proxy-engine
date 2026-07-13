//! Guardrails — PII redaction (Phase 2.4), MCP prompt injection scanning,
//! and agent tool-call loop detection.

pub mod injection;
pub mod loop_detector;
pub mod redaction_map;
pub mod redactor;

pub use injection::{scan_messages, scan_text, InjectionFinding};
pub use loop_detector::{detect_tool_call_loops, LoopFinding};
pub use redaction_map::{restore_payload, restore_payload_counted, RedactionMap};
pub use redactor::{redact_chat_request, redact_messages_request};
