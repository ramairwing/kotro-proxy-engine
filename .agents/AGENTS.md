# Kotro Proxy Engine: AI Engineering Roadmap & Directives

This document acts as permanent memory. Whenever making architectural decisions or starting a new feature for Kotro, refer to these prioritized directives.

## The 4 Strategic Breakthroughs
Kotro's goal is to be the ultimate developer-first Local AI Gateway (running at 0ms latency on localhost). Our roadmap out-competes cloud giants (Cloudflare, Portkey) by leveraging our local edge advantage.

1. **The "Death Loop Shield" (Agent Circuit Breaker) [PRIORITY 1]**
   - **Goal:** Stop autonomous agents (Cursor Auto, Devin) from burning API credits in infinite error loops.
   - **Execution:** Add temporal frequency monitoring in `handlers.rs`. If the exact same prompt/stack trace hits $\ge 4$ times in 60s, trip a circuit breaker, abort the request, and inject a synthetic system warning.
2. **Local AST-Aware Semantic Pruning [PRIORITY 2]**
   - **Goal:** Slash token usage by 50% natively.
   - **Execution:** Embed `tree-sitter` in Rust. Parse the AST of code blocks in `<1ms`. Automatically collapse unmodified functions into 1-line signatures and strip license headers before sending to OpenAI.
3. **Proactive Complexity Routing (Local MoE) [PRIORITY 3]**
   - **Goal:** Cut bills by 60% offline.
   - **Execution:** Build a local classifier. Route trivial prompts (typos, JSON formats) to local Llama 3 ($0). Route complex architectural prompts to Claude 3.5 Sonnet.
4. **Local Vector Semantic Caching [PRIORITY 4]**
   - **Goal:** Match intents, not exact strings.
   - **Execution:** Embed `candle-core` into Rust. Generate 384-d embeddings for prompts in 3ms. Cache hits trigger on `> 0.94` cosine similarity.
