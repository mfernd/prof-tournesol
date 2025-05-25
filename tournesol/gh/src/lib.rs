use axum::Router;
use axum_extra::routing::RouterExt;
use tower::ServiceBuilder;
use tower_http::trace::TraceLayer;

pub mod github;
mod routes;

pub fn create_root_app(state: AppState) -> Router {
    Router::new()
        .typed_post(routes::create_issue::create_issue_handler)
        .typed_post(routes::create_pull_request::create_pull_request_handler)
        .typed_get(routes::health::health_handler)
        .with_state(state)
        .layer(ServiceBuilder::new().layer(TraceLayer::new_for_http()))
}

#[derive(Clone)]
pub struct AppState {
    pub github_app_id: u64,
    pub github_app_private_key: String,
}

impl AppState {
    pub fn from_env() -> Result<Self, String> {
        let github_app_id = std::env::var("GITHUB_APP_ID")
            .map_err(|_| String::from("GITHUB_APP_ID must be set"))?
            .parse::<u64>()
            .map_err(|_| String::from("GITHUB_APP_ID must be a valid u64"))?;
        let github_app_private_key = std::env::var("GITHUB_APP_PRIVATE_KEY")
            .map_err(|_| String::from("GITHUB_APP_PRIVATE_KEY must be set"))?;

        Ok(Self {
            github_app_id,
            github_app_private_key,
        })
    }
}
