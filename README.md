# prof-tournesol

Fix your Kubernetes errors with AI. 🌻

## Goals

1. Connecting to a log system to detect Kubernetes errors
2. Attempting automatic error resolution via a model
3. If successful, automatically opening a pull request
4. If unsuccessful, making an AI-powered phone call to the on-call person
    - explaining the error verbally (the speech)
    - asking them to intervene
5. Logging the following information in a file:
    - time of the call
    - number called
    - speech used
    - response (boolean) to the question: can they intervene?

## Architecture

![Architecture diagram](architecture.svg)

## Use k8sgpt

Add you Hugging Face token to the `.env` file:

```bash
cp .env.example .env

vim .env
```

Create a kind cluster, serve gemma3-1b and K8sGPT workload:

```bash
# start kind cluster
just init

# serve a gemma3-1b-cpu model with kubeai
# (this will take a while)
kubectl apply -f manifests/gemma3-1b-model.yaml

# create a K8sGPT workload to use our gemma3 model
kubectl apply -f manifests/k8sgpt-config.yaml
```

Once the K8sGPT workload is created, you can use the following command to create an invalid Deployment that will be in a `CrashLoopBackOff` status:

```bash
kubectl apply -f manifests/invalid-deployment.yaml
```

After the K8sGPT workload has analyzed it, it will create `Result` CRD that will contain its diagnostic:

```bash
kubectl get result -n k8sgpt-operator-system
```

And you should see a `Result` entry for the invalid deployment.
