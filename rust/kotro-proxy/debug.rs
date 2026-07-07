use tree_sitter::Parser;
fn main() {
    let code = "// License header\nfn main() {}";
    let mut parser = Parser::new();
    parser.set_language(&tree_sitter_rust::LANGUAGE.into()).unwrap();
    let tree = parser.parse(code, None).unwrap();
    println!("{}", tree.root_node().to_sexp());
}
