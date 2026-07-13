//! Guardrails — PII redaction (Phase 2.4) and MCP prompt injection scanning.

pub mod injection;
pub mod redaction_map;
pub mod redactor;

pub use injection::{scan_messages, scan_text, InjectionFinding};
pub use redaction_map::{restore_payload, restore_payload_counted, RedactionMap};
pub use redactor::{redact_chat_request, redact_messages_request};
