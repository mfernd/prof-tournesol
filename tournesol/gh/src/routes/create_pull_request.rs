use crate::AppState;
use axum::{Json, extract::State, http::StatusCode};
use octocrab::{
    models::repos::{CommitAuthor, Object},
    params::repos::Reference,
};
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
    /// Action to perform on the file (update/create)
    action: FileAction,
    /// Path to the file in the repository
    path: String,
    /// Content of the file
    content: String,
}

#[derive(Debug, serde::Deserialize)]
pub enum FileAction {
    Create,
    Update,
}

pub async fn create_pull_request_handler(
    _: super::CreatePullRequestPath,
    State(state): State<AppState>,
    Json(req): Json<CreatePullRequestBody>,
) -> Result<String, StatusCode> {
    let owner = req.owner;
    let repo_name = req.repo;
    let new_branch_name = format!("fix/prof-tournesol/{}", uuid::Uuid::now_v7());

    let octocrab = crate::utils::get_octocrab_client_for_repo(
        state.github_app_id,
        &state.github_app_private_key,
        &owner,
        &repo_name,
    )
    .await
    .map_err(|_| StatusCode::BAD_REQUEST)?;

    let repo_handler = octocrab.repos(&owner, &repo_name);
    let pr_handler = octocrab.pulls(&owner, &repo_name);

    // Get repository information
    let repository = repo_handler.get().await.map_err(|e| {
        error!("get repository error: {:?}", e);
        StatusCode::NOT_FOUND
    })?;

    // Get main (default) branch
    let default_branch = repository.default_branch.ok_or(StatusCode::NOT_FOUND)?;

    let base_branch = repo_handler
        .get_ref(&Reference::Branch(default_branch.clone()))
        .await
        .map_err(|e| {
            error!("get base branch error: {:?}", e);
            StatusCode::NOT_FOUND
        })?;

    // Get latest commit SHA of the base branch
    let base_sha = match base_branch.object {
        Object::Commit { sha, .. } => sha,
        _ => return Err(StatusCode::BAD_REQUEST),
    };

    // Create the new branch
    let new_branch = repo_handler
        .create_ref(&Reference::Branch(new_branch_name.clone()), base_sha)
        .await
        .map_err(|e| {
            error!("create branch error: {:?}", e);
            StatusCode::INTERNAL_SERVER_ERROR
        })?;
    info!(new_branch = ?new_branch.url.path(), "branch created");

    // Create changes
    for file in req.pr.files {
        let new_file = match file.action {
            FileAction::Create => {
                let commit_message = format!("fix: created file {}", file.path);
                repo_handler.create_file(file.path, commit_message, file.content)
            }
            FileAction::Update => {
                let commit_message = format!("fix: updated file {}", file.path);
                let content_items = repo_handler
                    .get_content()
                    .path(&file.path)
                    .send()
                    .await
                    .map_err(|e| {
                        error!("get file content error: {:?}", e);
                        StatusCode::BAD_REQUEST
                    })?
                    .items;
                let sha = content_items
                    .first()
                    .ok_or_else(|| {
                        error!("file \"{}\" not found", file.path);
                        StatusCode::BAD_REQUEST
                    })?
                    .sha
                    .clone();
                repo_handler.update_file(file.path, commit_message, file.content, sha)
            }
        };

        new_file
            .branch(&new_branch_name)
            .author(CommitAuthor {
                name: crate::utils::GITHUB_APP_NAME.into(),
                email: crate::utils::GITHUB_APP_EMAIL.into(),
                date: None,
            })
            .send()
            .await
            .map_err(|e| {
                error!("create file error: {:?}", e);
                StatusCode::INTERNAL_SERVER_ERROR
            })?;
    }
    info!(new_branch = ?new_branch.url.path(), "changes applied");

    let pr_created = pr_handler
        .create(&req.pr.title, &new_branch_name, default_branch)
        .body(&req.pr.body)
        .maintainer_can_modify(true)
        .send()
        .await
        .map_err(|e| {
            error!("create pull request error: {:?}", e);
            StatusCode::INTERNAL_SERVER_ERROR
        })?;

    info!(pr_created = ?pr_created.url, "pull request created");

    Ok("pull request created".into())
}
