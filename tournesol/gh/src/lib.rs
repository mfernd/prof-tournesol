use axum::{Router, response::IntoResponse, routing::get};
use tower::ServiceBuilder;
use tower_http::trace::TraceLayer;

pub fn create_root_app() -> Router {
    Router::new()
        .route("/health", get(health))
        .layer(ServiceBuilder::new().layer(TraceLayer::new_for_http()))
}

async fn health() -> impl IntoResponse {
    "healthy"
}
