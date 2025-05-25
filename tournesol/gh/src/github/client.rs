use tracing::error;

#[derive(Clone)]
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

    pub async fn create_issue(
        &self,
        title: &str,
        body: &str,
    ) -> Result<octocrab::models::issues::Issue, octocrab::Error> {
        self.octocrab
            .issues(&self.owner, &self.repo)
            .create(title)
            .body(body)
            .send()
            .await
            .map_err(|e: octocrab::Error| {
                error!("failed to create issue: {:?}", e);
                e
            })
    }
}
