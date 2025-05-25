use crate::{
    AppState,
    github::{Change, GitHubClient},
};
use axum::{Json, extract::State, http::StatusCode};
use tracing::{error, info};

#[derive(Debug, serde::Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct CreatePullRequestBody {
    owner: String,
    repo: String,
    pr: PullRequest,
}

#[derive(Debug, serde::Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct PullRequest {
    title: String,
    body: String,
    files: Vec<File>,
}

#[derive(Debug, serde::Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct File {
    /// Path to the file in the repository
    path: String,
    /// Content of the file
    content: String,
}

pub async fn create_pull_request_handler(
    _: super::CreatePullRequestPath,
    State(state): State<AppState>,
    Json(req): Json<CreatePullRequestBody>,
) -> Result<String, StatusCode> {
    let owner = req.owner;
    let repo_name = req.repo;

    let octocrab = crate::github::get_octocrab_client_for_repo(
        state.github_app_id,
        &state.github_app_private_key,
        &owner,
        &repo_name,
    )
    .await
    .map_err(|e| {
        error!("failed to get octocrab client: {:?}", e);
        StatusCode::INTERNAL_SERVER_ERROR
    })?;

    let gh_client = GitHubClient::new(octocrab, owner, repo_name);

    let create_branch_result = gh_client
        .create_branch(format!("fix/prof-tournesol/{}", uuid::Uuid::now_v7()))
        .await
        .map_err(|e| {
            error!("create branch error: {:?}", e);
            StatusCode::INTERNAL_SERVER_ERROR
        })?;
    info!(new_branch = ?create_branch_result.new_branch.url.path(), "branch created");

    // Create changes
    for file in req.pr.files {
        gh_client
            .add_change(
                &create_branch_result.new_branch_name,
                Change {
                    path: file.path,
                    content: file.content,
                },
            )
            .await
            .map_err(|e| {
                error!("create changes error: {:?}", e);
                StatusCode::INTERNAL_SERVER_ERROR
            })?;
    }
    info!(new_branch = ?create_branch_result.new_branch_name, "changes applied");

    // Create pull request
    let pr_created = gh_client
        .create_pull_request(
            &req.pr.title,
            &req.pr.body,
            "main",
            &create_branch_result.new_branch_name,
        )
        .await
        .map_err(|e| {
            error!("create pull request error: {:?}", e);
            StatusCode::INTERNAL_SERVER_ERROR
        })?;

    info!(pr_created = ?pr_created.url, "pull request created");

    Ok("pull request created".into())
}
