//! Cursor Bridge / public-tunnel auth.
//!
//! When `KOTRO_BRIDGE_TOKEN` is set, LLM routes require that token in
//! `Authorization: Bearer …`, `x-api-key`, or `x-kotro-bridge-token`.
//! The real provider key stays in `KOTRO_UPSTREAM_API_KEY` and is injected
//! on upstream forward so a leaked `*.trycloudflare.com` URL alone is useless.

use axum::http::{HeaderMap, HeaderName, HeaderValue, StatusCode};
use axum::response::{IntoResponse, Response};

const HEADER_BRIDGE: &str = "x-kotro-bridge-token";

/// How to present `KOTRO_UPSTREAM_API_KEY` to the provider.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum UpstreamAuthStyle {
    /// `Authorization: Bearer <key>` (OpenAI-compatible, DeepSeek, …)
    Bearer,
    /// `x-api-key: <key>` (Anthropic Messages API)
    AnthropicXApiKey,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum BridgeAuthError {
    Missing,
    Invalid,
    UpstreamKeyRequired,
}

impl BridgeAuthError {
    pub fn into_response(self) -> Response {
        match self {
            Self::Missing => problem(
                StatusCode::UNAUTHORIZED,
                "Unauthorized",
                "KOTRO_BRIDGE_TOKEN is required. Send Authorization: Bearer <token>, x-api-key, or x-kotro-bridge-token.",
            ),
            Self::Invalid => problem(
                StatusCode::UNAUTHORIZED,
                "Unauthorized",
                "Invalid bridge token.",
            ),
            Self::UpstreamKeyRequired => problem(
                StatusCode::SERVICE_UNAVAILABLE,
                "Misconfigured",
                "KOTRO_BRIDGE_TOKEN is set but KOTRO_UPSTREAM_API_KEY is missing. \
                 Set the provider key on the proxy (extension setting kotrolabs.upstreamApiKey); \
                 put the bridge token in Cursor’s API key field.",
            ),
        }
    }
}

fn problem(status: StatusCode, title: &str, detail: &str) -> Response {
    let body = serde_json::json!({
        "type": "about:blank",
        "title": title,
        "status": status.as_u16(),
        "detail": detail,
    });
    (status, [("content-type", "application/problem+json")], body.to_string()).into_response()
}

/// Constant-time-ish equality for tokens of equal length.
fn tokens_equal(a: &str, b: &str) -> bool {
    if a.len() != b.len() {
        return false;
    }
    a.as_bytes()
        .iter()
        .zip(b.as_bytes().iter())
        .fold(0u8, |acc, (x, y)| acc | (x ^ y))
        == 0
}

/// Credential presented by the client for bridge auth.
pub fn extract_presented_token(headers: &HeaderMap) -> Option<String> {
    if let Some(auth) = headers.get("authorization").and_then(|v| v.to_str().ok()) {
        if let Some(token) = auth.strip_prefix("Bearer ") {
            let token = token.trim();
            if !token.is_empty() {
                return Some(token.to_string());
            }
        }
    }
    for name in [HEADER_BRIDGE, "x-api-key"] {
        if let Some(v) = headers
            .get(name)
            .and_then(|v| v.to_str().ok())
            .map(str::trim)
            .filter(|v| !v.is_empty())
        {
            return Some(v.to_string());
        }
    }
    None
}

/// No-op when `bridge_token` is unset/empty. Otherwise require a matching token.
pub fn require_bridge_token(
    bridge_token: Option<&str>,
    headers: &HeaderMap,
) -> Result<(), BridgeAuthError> {
    let Some(expected) = bridge_token.map(str::trim).filter(|s| !s.is_empty()) else {
        return Ok(());
    };

    let Some(presented) = extract_presented_token(headers) else {
        return Err(BridgeAuthError::Missing);
    };

    if tokens_equal(&presented, expected) {
        Ok(())
    } else {
        Err(BridgeAuthError::Invalid)
    }
}

/// Build headers for the upstream provider call.
///
/// When bridge auth is active, replaces client credentials with
/// `KOTRO_UPSTREAM_API_KEY` so the bridge token never reaches the provider.
pub fn prepare_upstream_headers(
    bridge_token: Option<&str>,
    upstream_api_key: Option<&str>,
    inbound: &HeaderMap,
    style: UpstreamAuthStyle,
) -> Result<HeaderMap, BridgeAuthError> {
    let bridge_active = bridge_token
        .map(str::trim)
        .filter(|s| !s.is_empty())
        .is_some();

    if !bridge_active {
        return Ok(inbound.clone());
    }

    let Some(key) = upstream_api_key.map(str::trim).filter(|s| !s.is_empty()) else {
        return Err(BridgeAuthError::UpstreamKeyRequired);
    };

    let mut out = HeaderMap::new();
    // Preserve Anthropic version headers; drop inbound auth (bridge token).
    for name in ["anthropic-version", "anthropic-beta", "content-type"] {
        if let Some(value) = inbound.get(name) {
            out.insert(HeaderName::from_static(name), value.clone());
        }
    }

    match style {
        UpstreamAuthStyle::Bearer => {
            let value = format!("Bearer {key}");
            out.insert(
                HeaderName::from_static("authorization"),
                HeaderValue::from_str(&value).map_err(|_| BridgeAuthError::UpstreamKeyRequired)?,
            );
        }
        UpstreamAuthStyle::AnthropicXApiKey => {
            out.insert(
                HeaderName::from_static("x-api-key"),
                HeaderValue::from_str(key).map_err(|_| BridgeAuthError::UpstreamKeyRequired)?,
            );
            // Anthropic clients sometimes also send Bearer; keep only x-api-key.
        }
    }

    Ok(out)
}

#[cfg(test)]
mod tests {
    use super::*;

    fn headers_with(pairs: &[(&str, &str)]) -> HeaderMap {
        let mut h = HeaderMap::new();
        for (k, v) in pairs {
            h.insert(
                HeaderName::from_bytes(k.as_bytes()).unwrap(),
                HeaderValue::from_str(v).unwrap(),
            );
        }
        h
    }

    #[test]
    fn disabled_when_unset() {
        let h = headers_with(&[]);
        assert!(require_bridge_token(None, &h).is_ok());
        assert!(require_bridge_token(Some(""), &h).is_ok());
    }

    #[test]
    fn requires_token_when_set() {
        let h = headers_with(&[]);
        assert_eq!(
            require_bridge_token(Some("secret"), &h),
            Err(BridgeAuthError::Missing)
        );
    }

    #[test]
    fn accepts_bearer() {
        let h = headers_with(&[("authorization", "Bearer secret")]);
        assert!(require_bridge_token(Some("secret"), &h).is_ok());
    }

    #[test]
    fn accepts_x_api_key_and_bridge_header() {
        let h1 = headers_with(&[("x-api-key", "secret")]);
        assert!(require_bridge_token(Some("secret"), &h1).is_ok());
        let h2 = headers_with(&[("x-kotro-bridge-token", "secret")]);
        assert!(require_bridge_token(Some("secret"), &h2).is_ok());
    }

    #[test]
    fn rejects_wrong_token() {
        let h = headers_with(&[("authorization", "Bearer nope")]);
        assert_eq!(
            require_bridge_token(Some("secret"), &h),
            Err(BridgeAuthError::Invalid)
        );
    }

    #[test]
    fn prepare_passthrough_without_bridge() {
        let inbound = headers_with(&[("authorization", "Bearer sk-real")]);
        let out = prepare_upstream_headers(None, None, &inbound, UpstreamAuthStyle::Bearer).unwrap();
        assert_eq!(
            out.get("authorization").unwrap().to_str().unwrap(),
            "Bearer sk-real"
        );
    }

    #[test]
    fn prepare_injects_upstream_key() {
        let inbound = headers_with(&[("authorization", "Bearer bridge-tok")]);
        let out = prepare_upstream_headers(
            Some("bridge-tok"),
            Some("sk-provider"),
            &inbound,
            UpstreamAuthStyle::Bearer,
        )
        .unwrap();
        assert_eq!(
            out.get("authorization").unwrap().to_str().unwrap(),
            "Bearer sk-provider"
        );
    }

    #[test]
    fn prepare_requires_upstream_key_in_bridge_mode() {
        let inbound = headers_with(&[("authorization", "Bearer bridge-tok")]);
        assert_eq!(
            prepare_upstream_headers(
                Some("bridge-tok"),
                None,
                &inbound,
                UpstreamAuthStyle::Bearer,
            ),
            Err(BridgeAuthError::UpstreamKeyRequired)
        );
    }

    #[test]
    fn prepare_anthropic_style() {
        let inbound = headers_with(&[
            ("authorization", "Bearer bridge-tok"),
            ("anthropic-version", "2023-06-01"),
        ]);
        let out = prepare_upstream_headers(
            Some("bridge-tok"),
            Some("sk-ant"),
            &inbound,
            UpstreamAuthStyle::AnthropicXApiKey,
        )
        .unwrap();
        assert_eq!(out.get("x-api-key").unwrap().to_str().unwrap(), "sk-ant");
        assert!(out.get("authorization").is_none());
        assert_eq!(
            out.get("anthropic-version").unwrap().to_str().unwrap(),
            "2023-06-01"
        );
    }
}
