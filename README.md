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

![Architecture diagram](architecture.excalidraw.png)
