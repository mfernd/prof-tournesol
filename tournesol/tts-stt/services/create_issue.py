import requests

GITHUB_REPO = "prof-tournesol"
GITHUB_OWNER = "mfernd"

def create_issue(url, title, body):
    url = f'{url}/issues'
    data = {
        "title": title,
        "body": body,
        "owner": GITHUB_OWNER,
        "repo": GITHUB_REPO
    }
    response = requests.post(url, json=data)
    return response.json()