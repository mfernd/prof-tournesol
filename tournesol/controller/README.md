# Prof Tournesol Controller

The Prof Tournesol Controller is a Kubernetes controller that monitors K8SGPT Result resources. When a diagnostic Result is detected, the controller:

1. Extracts the diagnostic information from the Result resource, including the namespace of the problematic resource
2. Fetches related files from GitHub repository in the `apps/<namespace>` directory using wget
3. Sends both the files and the solution from K8SGPT to an AI endpoint for analysis and explanation

![Controller Workflow](https://mermaid.ink/img/pako:eNptkMFqwzAMhl_F-NRDM5Zdegy7DtZDL2WXQhcjWE6CY7vByYZp8t7rsgTKQJYQ-vSL_0-Tz9Qaw6SgdY7-aDky6jrr8AfD5if0B8yZndiJBTsYGW9xAxLuOSgs6_p4W_n87ubMKFx722JSGU3qSGHnvIE-eFBwOrk3MT_INntsJ5cdAyZQQDjm1mB0BZ6ZoGbjujTTx-5xvavq6q55qu-Jik8w1Ev-MYvUfixlaXl2Czn9JRhA_z8OY30aIyalPS3xucZUEk2rxdwwKYptVjaGVO5JbZikchs0RRS4pp4U2FnquSBVuHQlnbjkohK5EREdlU4WPP33D5L9Zi0?type=png)

## Configuration

The controller has minimal configuration needs:

| Variable | Description | Default |
|----------|-------------|---------|
| `GITHUB_OWNER` | GitHub repository owner | `mfernd` |
| `GITHUB_REPO` | GitHub repository name | `prof-tournesol` |
| `GITHUB_BRANCH` | GitHub branch to use | `main` |

The AI endpoint is hardcoded to `http://kubeai.kubeai.svc.cluster.local:80/openai/v1` using the `gemma3-1b-cpu` model.

## Running the Controller

### With Skaffold

The controller is designed to run inside a Kubernetes cluster using Skaffold:

```bash
# Deploy with Skaffold
cd prof-tournesol/tournesol/controller
skaffold dev
```

### In Kubernetes

You can deploy the controller using the provided Dockerfile and Kubernetes manifests:

```bash
# Build the Docker image (includes SVN for GitHub access)
docker build -t your-registry/prof-tournesol-controller:latest .

# Push to your registry
docker push your-registry/prof-tournesol-controller:latest
```

Then apply the Kubernetes deployment:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: prof-tournesol-controller
  namespace: k8sgpt-operator-system
spec:
  replicas: 1
  selector:
    matchLabels:
      app: prof-tournesol-controller
  template:
    metadata:
      labels:
        app: prof-tournesol-controller
    spec:
      containers:
      - name: controller
        image: your-registry/prof-tournesol-controller:latest
        env:
        - name: GITHUB_OWNER
          value: "your-github-username"
        - name: GITHUB_REPO
          value: "your-repo-name"
```

## AI Endpoint Format

The controller sends data to the AI endpoint in OpenAI-compatible format:

```json
{
  "model": "gemma3-1b-cpu",
  "messages": [
    {
      "role": "user",
      "content": "# Problem Solution from K8SGPT\n\nThe solution text from K8SGPT Result\n\n# Related Files\n\n## deployment.yaml\n```yaml\ncontent of deployment.yaml\n```\n\n## service.yaml\n```yaml\ncontent of service.yaml\n```\n"
    }
  ]
}
```

## GitHub Repository Structure

The controller expects to find relevant files in the GitHub repository under:

```
app/<namespace>/
```

Where `<namespace>` is the Kubernetes namespace extracted from the `spec.name` field (in format "namespace/name") of the problematic resource detected by K8SGPT. Files must be located in the `apps/<namespace>` directory of the repository.

## Namespace Extraction & File Access

### Namespace Extraction
The controller intelligently extracts the namespace from the K8SGPT Result:
- Parses the `spec.name` field which contains the resource name in "namespace/name" format
- Example: In `nginx-unstable/nginx-oom-7445cfcc57-mh8s7`, the namespace is `nginx-unstable`
- Falls back to the Result's own namespace only if the resource name format doesn't include a namespace

### File Access Method
The controller fetches files from GitHub using wget:
- Looks exclusively in the `apps/<namespace>` directory
- Uses GitHub's raw content URLs: `https://raw.githubusercontent.com/[owner]/[repo]/main/apps/[namespace]`
- No authentication required for public repositories
- Not subject to API rate limits that affect the GitHub API
- Retrieves all files in the directory without needing to specify filenames

## Testing

You can test the controller by creating a custom K8SGPT Result resource:

```yaml
# test-result.yaml
apiVersion: core.k8sgpt.ai/v1alpha1
kind: Result
metadata:
  name: test-result
  namespace: default
spec:
  name: "TestError"
  kind: "Deployment"
  details: "Error: Test error message for deployment
  
  Solution: Update container image tag to a valid version"
```

Apply the resource to your cluster:

```bash
kubectl apply -f test-result.yaml
```

This will trigger the controller to:
1. Read the Result
2. Fetch files from the GitHub repository at `app/default/` 
3. Send those files and the solution to the configured AI endpoint

