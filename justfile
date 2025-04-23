cluster_name := "prof-tournesol-dev"

init:
  kind create cluster --name {{ cluster_name }}
  just --justfile {{justfile()}} helmfile-apply

helmfile-apply:
  #!/bin/bash
  set -a; source .env; set +a;
  helmfile apply

delete:
  kind delete cluster --name {{ cluster_name }}

recreate: delete init
