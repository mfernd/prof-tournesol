#[allow(dead_code)]
#[derive(Debug)]
pub enum GetOctocrabError {
    InvalidJsonWebToken(jsonwebtoken::errors::Error),
    OctocrabError(octocrab::Error),
}

pub async fn get_octocrab_client_for_repo(
    github_app_id: u64,
    github_app_private_key: &str,
    owner: &str,
    repo: &str,
) -> Result<octocrab::Octocrab, GetOctocrabError> {
    let private_key = jsonwebtoken::EncodingKey::from_rsa_pem(github_app_private_key.as_bytes())
        .map_err(GetOctocrabError::InvalidJsonWebToken)?;

    // 1. Typically you will first construct an Octocrab using `OctocrabBuilder::app` to authenticate as your Github App.
    let octocrab = octocrab::Octocrab::builder()
        .app(octocrab::models::AppId(github_app_id), private_key)
        .build()
        .map_err(GetOctocrabError::OctocrabError)?;

    // 2. Then obtain an installation ID.
    let installation = octocrab
        .apps()
        .get_repository_installation(&owner, &repo)
        .await
        .map_err(GetOctocrabError::OctocrabError)?;

    // 3. And then pass that here to obtain a new Octocrab
    // with which you can make API calls with the permissions of that installation.
    octocrab
        .installation(installation.id)
        .map_err(GetOctocrabError::OctocrabError)
}
