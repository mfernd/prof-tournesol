# Get health
GET http://localhost:3000/health
HTTP 200

# Create issue
POST http://localhost:3000/issues
{
  "owner": "mfernd",
  "repo": "prof-tournesol",
  "title": "Creating an issue",
  "body": "This is a test issue\nwith multiple lines\nwith **markdown**."
}

# Create pull request
POST http://localhost:3000/pull_requests
{
  "owner": "mfernd",
  "repo": "prof-tournesol",
  "pr": {
    "title": "fix: Fixed your problems! 😄😮",
    "body": "This PR fix all your problems. 😸🌻",
    "files": [
      {
        "path": "tournesol/gh/README.md",
        "content": "# gh service\n\nUsed to do actions (create issues, create pull requests) on GitHub.\n"
      },
      {
        "path": "tournesol/gh/src/routes/health.rs",
        "content": "use axum::response::IntoResponse;\n\npub async fn health_handler(_: super::HealthPath) -> impl IntoResponse {\n    \"healthy ++ 😻😻😻\"\n}\n"
      }
    ]
  }
}
