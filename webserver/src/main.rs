use reqwest::Client;
use std::env;
use tide::prelude::*;
use tide::Request;

#[async_std::main]
async fn main() -> tide::Result<()> {
    println!("Set up");
    let mut app = tide::new();
    app.at("/search").get(handle_search);
    app.listen("127.0.0.1:22920").await?;
    Ok(())
}

#[derive(Debug, Deserialize)]
struct SearchRequest {
    query: String,
}

async fn handle_search(mut req: Request<()>) -> tide::Result {
    let search_req: SearchRequest = req.body_json().await?;
    println!("Received {}", search_req.query);

    let tvdb_proxy_ip = env::var("TVDB_IP_ADDRESS").unwrap_or("localhost:22000".to_string());

    let params = [("mediaType", "series"), ("mediaName", "naruto")];

    let url = format!("http://{}/series", tvdb_proxy_ip);
    println!("Making REST request to TVDB Proxy at: {}", url);

    let client = Client::new();
    let response = client
        .get(&url)
        .query(&params)
        .send()
        .await
        .map_err(|e| {
            eprintln!("Failed to send request: {:?}", e);
            tide::Error::new(500, e)
        })?
        .text()
        .await
        .map_err(|e| {
            eprintln!("Failed to read response: {:?}", e);
            tide::Error::new(500, e)
        })?;

    Ok(response.into())
}
