use crate::AppState;
use axum::{Json, extract::State};
use tracing::{error, info};

#[derive(Debug, serde::Deserialize)]
pub struct CreateIssueBody {
    title: String,
    body: String,
    owner: String,
    repo: String,
}

pub async fn create_issue_handler(
    _: super::CreateIssuePath,
    State(state): State<AppState>,
    Json(req): Json<CreateIssueBody>,
) -> Result<String, axum::http::StatusCode> {
    info!("creating issue \"{}\" in repo {}/{}", req.title, req.owner, req.repo);

    let octocrab = crate::utils::get_octocrab_client_for_repo(
        state.github_app_id,
        &state.github_app_private_key,
        &req.owner,
        &req.repo,
    )
    .await
    .map_err(|e| {
        error!("failed to get octocrab client: {:?}", e);
        axum::http::StatusCode::INTERNAL_SERVER_ERROR
    })?;

    let issue = octocrab
        .issues(req.owner, req.repo)
        .create(req.title)
        .body(req.body)
        .send()
        .await
        .map_err(|e| {
            error!("failed to create issue: {:?}", e);
            axum::http::StatusCode::INTERNAL_SERVER_ERROR
        })?;
    info!(
        "created the issue #{} in the repo \"{}\"",
        issue.number,
        issue.repository_url.path()
    );

    Ok("issue created".into())
}
