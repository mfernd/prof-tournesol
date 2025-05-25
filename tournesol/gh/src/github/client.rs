use octocrab::{
    models::repos::{CommitAuthor, Object, Ref},
    params::repos::Reference,
};
use tracing::{error, info};

/// `GitHubClient` is a client for interacting with one GitHub repository
pub struct GitHubClient {
    pub octocrab: octocrab::Octocrab,
    pub owner: String,
    pub repo: String,
}

impl GitHubClient {
    pub fn new(octocrab: octocrab::Octocrab, owner: String, repo: String) -> Self {
        Self {
            octocrab,
            owner,
            repo,
        }
    }

    /// Creates a new issue in the repository.
    pub async fn create_issue(
        &self,
        title: &str,
        body: &str,
    ) -> Result<octocrab::models::issues::Issue, GitHubClientError> {
        self.octocrab
            .issues(&self.owner, &self.repo)
            .create(title)
            .body(body)
            .send()
            .await
            .map_err(GitHubClientError::CreateIssue)
    }

    /// Creates a new branch in the repository. From the base/default branch.
    pub async fn create_branch(
        &self,
        new_branch_name: String,
    ) -> Result<CreateBranchResult, GitHubClientError> {
        let repo_handler = self.octocrab.repos(&self.owner, &self.repo);

        // Get repository information
        let repository = repo_handler.get().await.map_err(|e| {
            error!("get repository error: {:?}", e);
            GitHubClientError::Unknown
        })?;

        // Get main (default) branch
        let default_branch_name = repository
            .default_branch
            .ok_or(GitHubClientError::DefaultBranchNotFound)?;

        let default_branch = repo_handler
            .get_ref(&Reference::Branch(default_branch_name.clone()))
            .await
            .map_err(|e| {
                error!("get base branch error: {:?}", e);
                GitHubClientError::Unknown
            })?;

        // Get latest commit SHA of the base branch
        let latest_sha = match default_branch.object {
            Object::Commit { ref sha, .. } => sha,
            _ => return Err(GitHubClientError::NoCommitInDefaultBranch),
        };

        // Create the new branch
        let new_branch = repo_handler
            .create_ref(&Reference::Branch(new_branch_name.clone()), latest_sha)
            .await
            .map_err(GitHubClientError::CreateBranch)?;

        Ok(CreateBranchResult {
            default_branch_name,
            default_branch,
            new_branch_name,
            new_branch,
        })
    }

    /// Adds a change to the repository on a branch.
    pub async fn add_change(
        &self,
        branch_name: &str,
        change: Change,
    ) -> Result<(), GitHubClientError> {
        let repo_handler = self.octocrab.repos(&self.owner, &self.repo);

        // Check if the file already exists in the repository
        let content_result = repo_handler.get_content().path(&change.path).send().await;

        // Create or update the file in the repository
        let new_file = match content_result {
            Err(_) => {
                info!(file_path = ?change.path, "file does not exist, creating");
                let commit_message = format!("fix: created file {}", change.path);
                repo_handler.create_file(change.path, commit_message, change.content)
            }
            Ok(content) => {
                info!(file_path = ?change.path, "file already exists, updating");
                let commit_message = format!("fix: updated file {}", change.path);
                let sha = content
                    .items
                    .first()
                    .ok_or_else(|| {
                        error!("file \"{}\" not found", change.path);
                        GitHubClientError::Unknown
                    })?
                    .sha
                    .clone();
                repo_handler.update_file(change.path, commit_message, change.content, sha)
            }
        };

        // Commit the file to the specified branch
        new_file
            .branch(branch_name)
            .author(CommitAuthor {
                name: super::GITHUB_APP_NAME.into(),
                email: super::GITHUB_APP_EMAIL.into(),
                date: None,
            })
            .send()
            .await
            .map_err(GitHubClientError::CreateCommit)?;

        Ok(())
    }

    /// Creates a pull request in the repository.
    pub async fn create_pull_request(
        &self,
        title: &str,
        body: &str,
        base_branch: &str,
        new_branch_name: &str,
    ) -> Result<octocrab::models::pulls::PullRequest, GitHubClientError> {
        self.octocrab
            .pulls(&self.owner, &self.repo)
            .create(title, new_branch_name, base_branch)
            .body(body)
            .maintainer_can_modify(true)
            .send()
            .await
            .map_err(GitHubClientError::CreatePullRequest)
    }
}

#[derive(Debug)]
pub enum GitHubClientError {
    CreateIssue(octocrab::Error),
    DefaultBranchNotFound,
    NoCommitInDefaultBranch,
    CreateBranch(octocrab::Error),
    CreateCommit(octocrab::Error),
    CreatePullRequest(octocrab::Error),
    Unknown,
}

#[derive(Debug)]
pub struct CreateBranchResult {
    pub default_branch_name: String,
    pub default_branch: Ref,
    pub new_branch_name: String,
    pub new_branch: Ref,
}

#[derive(Debug)]
pub struct Change {
    /// Path to the file in the repository.
    pub path: String,
    /// Content of the file.
    pub content: String,
}
