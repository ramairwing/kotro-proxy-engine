use std::sync::OnceLock;
use regex::Regex;

static TRIVIAL_KEYWORDS: OnceLock<Regex> = OnceLock::new();

fn get_trivial_regex() -> &'static Regex {
    TRIVIAL_KEYWORDS.get_or_init(|| {
        Regex::new(r"(?i)\b(fix\s+typo|lint|format|cleanup|add\s+comment|json|syntax\s+error)\b").unwrap()
    })
}

/// Returns true if the prompt is considered "trivial" enough to bypass
/// the heavy cloud models and go straight to the local MoE (like Llama 3).
pub fn is_trivial_prompt(prompt: &str) -> bool {
    // Strip markdown code blocks before measuring length
    let code_block_regex = Regex::new(r"```[\s\S]*?```").unwrap();
    let stripped = code_block_regex.replace_all(prompt, "");
    let text = stripped.trim();

    if text.is_empty() {
        return false;
    }

    // If the textual portion of the prompt is very short (< 150 characters)
    // and contains specific low-level tasks, we classify as trivial.
    if text.len() < 150 {
        let re = get_trivial_regex();
        if re.is_match(text) {
            return true;
        }
    }

    false
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_is_trivial_prompt() {
        assert!(is_trivial_prompt("fix typo in the print statement"));
        assert!(is_trivial_prompt("format this file as json"));
        assert!(is_trivial_prompt("cleanup the whitespace here"));
        assert!(is_trivial_prompt("fix typo\n```rust\nfn main() {}\n```"));
        
        assert!(!is_trivial_prompt("Explain the microservice architecture and how the proxy intercepts requests."));
        assert!(!is_trivial_prompt("Design a new database schema for the user table with these fields: id, name, email."));
        assert!(is_trivial_prompt("please lint this code"));
        
        let long_prompt = "I need you to fix a typo, but also redesign the entire backend architecture so that it can scale to millions of users. Please implement the caching layer using Redis and add unit tests.";
        assert!(!is_trivial_prompt(long_prompt));
    }
}
