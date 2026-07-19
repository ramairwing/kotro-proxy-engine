//! `kotro-proxy` — single-binary local LLM reverse proxy (Rust Phase 2).

use kotro_proxy::{config::Config, server::Server};
use std::env;
use tracing::info;

const VERSION: &str = env!("CARGO_PKG_VERSION");

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
    let args: Vec<String> = std::env::args().collect();
    if args.iter().any(|a| a == "--version" || a == "-V") {
        println!("kotro-proxy {VERSION}");
        return Ok(());
    }

    let cfg = Config::load();

    // Initialise telemetry and retain the provider handle so we can flush
    // buffered spans before exit (only Some when KOTRO_OTEL_ENDPOINT is set).
    let otel_provider = match kotro_proxy::telemetry::otel::init_telemetry(cfg.otel_endpoint.as_deref()) {
        Ok(p) => p,
        Err(e) => {
            eprintln!("Failed to initialize telemetry: {e}");
            None
        }
    };

    let bridge_enabled = cfg
        .bridge_token
        .as_deref()
        .map(str::trim)
        .filter(|s| !s.is_empty())
        .is_some();
    if bridge_enabled && cfg.upstream_api_key.as_deref().map(str::trim).filter(|s| !s.is_empty()).is_none() {
        tracing::warn!(
            "KOTRO_BRIDGE_TOKEN is set without KOTRO_UPSTREAM_API_KEY — \
             upstream LLM calls will return 503 until the provider key is configured"
        );
    }

    info!(
        service = "kotro-proxy",
        listen = %cfg.listen_addr,
        metrics = %cfg.metrics_addr,
        upstream = %cfg.upstream_url,
        fallback_configured = cfg.fallback_url.is_some(),
        bridge_auth = bridge_enabled,
        profile = %env::var("KOTRO_PROFILE").unwrap_or_default(),
        cache_strategy = ?cfg.cache_key_strategy,
        cache_window = cfg.cache_window_size,
        redaction = cfg.enable_redaction,
        compression = cfg.enable_compression,
        "starting kotrolabs proxy"
    );

    let server = Server::new(cfg)?;
    server.run().await?;

    // Flush any buffered OTel spans before the process exits.
    if let Some(provider) = otel_provider {
        if let Err(e) = provider.shutdown() {
            eprintln!("OTel provider shutdown error: {e}");
        }
    }

    Ok(())
}
