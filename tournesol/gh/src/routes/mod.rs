use axum_extra::routing::TypedPath;
use serde::Deserialize;

pub mod create_issue;
pub mod health;

#[derive(TypedPath, Deserialize)]
#[typed_path("/issues")]
pub struct CreateIssuePath;

#[derive(TypedPath, Deserialize)]
#[typed_path("/health")]
pub struct HealthPath;
