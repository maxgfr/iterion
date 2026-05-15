// Minimal consumer pinning vulnerable tokio 1.7.1.
// Not production code — exists so secured-renovacy's align_code has a
// concrete call-site to update on breaking changes.

#[tokio::main(flavor = "current_thread")]
async fn main() {
    let v = compute().await;
    println!("{}", v);
}

async fn compute() -> i32 {
    42
}
