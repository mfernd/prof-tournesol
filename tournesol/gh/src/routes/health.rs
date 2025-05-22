use axum::response::IntoResponse;

pub async fn health_handler(_: super::HealthPath) -> impl IntoResponse {
    "healthy"
}
