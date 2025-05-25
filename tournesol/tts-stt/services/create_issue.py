import requests
import os
from dotenv import load_dotenv

load_dotenv()

GITHUB_SERVICE_URL = os.getenv("GITHUB_SERVICE_URL", "http://localhost:8888")
GITHUB_REPO = "prof-tournesol"
GITHUB_OWNER = "mfernd"

def create_issue(title, body):
    url = f'{GITHUB_SERVICE_URL}/issues'
    data = {
        "title": title,
        "body": body,
        "owner": GITHUB_OWNER,
        "repo": GITHUB_REPO
    }
    response = requests.post(url, json=data)
    return response.json()