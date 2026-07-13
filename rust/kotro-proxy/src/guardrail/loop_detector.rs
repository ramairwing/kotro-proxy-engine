//! Agent tool-call loop detector.
//!
//! The existing request-level circuit breaker (in `router/handlers.rs`) fires
//! when the **same entire request** is repeated ≥ 4 times — it catches
//! identical cache keys. That doesn't catch the more insidious case where an
//! agent calls the **same tool with the same arguments** across multiple turns
//! of a single conversation: the surrounding context keeps changing (so the
//! cache key differs) while the agent is stuck in a semantic loop.
//!
//! This module detects that per-conversation loop by scanning the `tool_calls`
//! arrays in `assistant` messages and counting `(function_name, sha256(args))`
//! occurrences. When any pair exceeds the configured threshold, Kotro returns a
//! structured error instead of forwarding to the upstream provider — preventing
//! uncapped token spend on stuck agent loops.
//!
//! ## Example loop pattern caught
//!
//! ```text
//! turn 1: assistant → call get_repo_file("src/main.rs")
//! turn 2: assistant → call get_repo_file("src/main.rs")   ← dup #2
//! turn 3: assistant → call get_repo_file("src/main.rs")   ← fires at threshold=3
//! ```
//!
//! ## Configuration
//!
//! `KOTRO_TOOL_LOOP_THRESHOLD=3` (default). Set to `0` to disable.

use std::collections::HashMap;

use sha2::{Digest, Sha256};
use serde_json::Value;

use crate::models::unified::UnifiedMessage;

/// Describes a detected tool-call loop.
#[derive(Debug, Clone)]
pub struct LoopFinding {
    /// The MCP function name that was called repeatedly.
    pub function_name: String,
    /// How many times the call was seen (≥ threshold).
    pub call_count: u32,
}

/// Extract `(function_name, sha256_of_args)` tuples from a single message.
///
/// Only processes messages that have a non-null `tool_calls` field.
fn tool_call_keys(msg: &UnifiedMessage) -> Vec<(String, [u8; 32])> {
    let tool_calls = match &msg.tool_calls {
        Some(Value::Array(arr)) => arr,
        _ => return vec![],
    };

    tool_calls
        .iter()
        .filter_map(|call| {
            let func = call.get("function")?;
            let name = func.get("name")?.as_str()?.to_string();
            // Serialize args to a canonical string for hashing.
            // Use the raw string if it's already a JSON string (OpenAI format
            // sends `arguments` as a pre-serialized string), otherwise
            // serialize the value.
            let args = match func.get("arguments") {
                Some(Value::String(s)) => s.clone(),
                Some(v) => v.to_string(),
                None => String::new(),
            };
            let digest: [u8; 32] = Sha256::digest(args.as_bytes()).into();
            Some((name, digest))
        })
        .collect()
}

/// Scan a conversation for repeated tool calls.
///
/// Returns `Some(LoopFinding)` when any `(function_name, arguments)` pair
/// appears in `assistant` messages at least `threshold` times.
///
/// Returns `None` immediately when `threshold == 0` (feature disabled).
pub fn detect_tool_call_loops(
    messages: &[UnifiedMessage],
    threshold: u32,
) -> Option<LoopFinding> {
    if threshold == 0 {
        return None;
    }

    // (function_name, args_digest) → count
    let mut seen: HashMap<(String, [u8; 32]), u32> = HashMap::new();

    for msg in messages {
        if msg.role != "assistant" {
            continue;
        }
        for (name, digest) in tool_call_keys(msg) {
            let count = seen.entry((name.clone(), digest)).or_insert(0);
            *count += 1;
            if *count >= threshold {
                return Some(LoopFinding {
                    function_name: name,
                    call_count: *count,
                });
            }
        }
    }
    None
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    fn make_tool_call_msg(function: &str, args: &str) -> UnifiedMessage {
        UnifiedMessage {
            role: "assistant".to_string(),
            content: Value::Null,
            name: None,
            tool_call_id: None,
            tool_calls: Some(json!([{
                "id": "call_test",
                "type": "function",
                "function": {
                    "name": function,
                    "arguments": args
                }
            }])),
        }
    }

    fn make_user_msg(text: &str) -> UnifiedMessage {
        UnifiedMessage {
            role: "user".to_string(),
            content: Value::String(text.to_string()),
            name: None,
            tool_call_id: None,
            tool_calls: None,
        }
    }

    #[test]
    fn no_loop_on_single_call() {
        let msgs = vec![make_tool_call_msg("get_file", r#"{"path":"main.rs"}"#)];
        assert!(detect_tool_call_loops(&msgs, 3).is_none());
    }

    #[test]
    fn detects_loop_at_threshold() {
        let msgs = vec![
            make_user_msg("what is in main.rs?"),
            make_tool_call_msg("get_file", r#"{"path":"main.rs"}"#),
            make_user_msg("try again"),
            make_tool_call_msg("get_file", r#"{"path":"main.rs"}"#),
            make_user_msg("and again"),
            make_tool_call_msg("get_file", r#"{"path":"main.rs"}"#),
        ];
        let f = detect_tool_call_loops(&msgs, 3);
        assert!(f.is_some());
        let f = f.unwrap();
        assert_eq!(f.function_name, "get_file");
        assert_eq!(f.call_count, 3);
    }

    #[test]
    fn does_not_fire_below_threshold() {
        let msgs = vec![
            make_tool_call_msg("get_file", r#"{"path":"main.rs"}"#),
            make_tool_call_msg("get_file", r#"{"path":"main.rs"}"#),
        ];
        assert!(detect_tool_call_loops(&msgs, 3).is_none());
    }

    #[test]
    fn different_args_are_not_a_loop() {
        let msgs = vec![
            make_tool_call_msg("get_file", r#"{"path":"main.rs"}"#),
            make_tool_call_msg("get_file", r#"{"path":"lib.rs"}"#),
            make_tool_call_msg("get_file", r#"{"path":"mod.rs"}"#),
        ];
        // Three calls to the same function, but with different args — not a loop
        assert!(detect_tool_call_loops(&msgs, 3).is_none());
    }

    #[test]
    fn different_functions_are_not_a_loop() {
        let args = r#"{"path":"main.rs"}"#;
        let msgs = vec![
            make_tool_call_msg("read_file", args),
            make_tool_call_msg("write_file", args),
            make_tool_call_msg("delete_file", args),
        ];
        assert!(detect_tool_call_loops(&msgs, 3).is_none());
    }

    #[test]
    fn threshold_zero_disables_detection() {
        let msgs = vec![
            make_tool_call_msg("nuke", r#"{}"#),
            make_tool_call_msg("nuke", r#"{}"#),
            make_tool_call_msg("nuke", r#"{}"#),
            make_tool_call_msg("nuke", r#"{}"#),
            make_tool_call_msg("nuke", r#"{}"#),
        ];
        // Threshold = 0 means disabled
        assert!(detect_tool_call_loops(&msgs, 0).is_none());
    }

    #[test]
    fn user_role_messages_are_not_scanned() {
        // tool_calls in a user message is not a real pattern, but even if it
        // appeared we must not count it (only assistant messages carry tool calls)
        let mut msg = make_user_msg("hello");
        msg.tool_calls = Some(json!([{
            "id": "x",
            "type": "function",
            "function": {"name": "get_file", "arguments": r#"{"path":"a.rs"}"#}
        }]));
        let msgs = vec![msg.clone(), msg.clone(), msg.clone()];
        assert!(detect_tool_call_loops(&msgs, 3).is_none());
    }

    #[test]
    fn returns_correct_count_above_threshold() {
        let msgs = vec![
            make_tool_call_msg("send_slack", r#"{"msg":"hello"}"#),
            make_tool_call_msg("send_slack", r#"{"msg":"hello"}"#),
            make_tool_call_msg("send_slack", r#"{"msg":"hello"}"#),
            make_tool_call_msg("send_slack", r#"{"msg":"hello"}"#),
        ];
        let f = detect_tool_call_loops(&msgs, 3).unwrap();
        assert_eq!(f.call_count, 3); // fires at threshold, not at the end
    }
}
