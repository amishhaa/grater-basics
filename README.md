# grater-basics
Setup Instructions
Prerequisites

You need:

Go ≥ 1.21

Docker

Git

Install grater (from source)

Clone the repo:

git clone https://github.com/amishhaa/grater-basics.git
cd grater-basics


Build and install the CLI:

go install ./cmd/grater


Make sure $GOPATH/bin is in your PATH:

echo $PATH | grep go/bin

grater --help

Workspace

grater uses a local workspace directory:

.grater/


This is created automatically when you run:

grater prepare


It stores:

modules.txt → list of downstream modules (Currently the functionality to fetch modules is not yet implemented)

results.json → test results (when grater run is executed)

Usage
1. Prepare downstream modules
grater prepare


Creates:

.grater/modules.txt

2. Run tests //basic run functionality, currently heavily in development
grater run \
  --repo github.com/open-telemetry/opentelemetry-go \
  --base main \
  --head HEAD

3. View report (not implemented yet)
grater report

Docker runner

Build the runner image:

docker build -t grater-runner -f docker/Dockerfile .


## Quick Start

go install ./cmd/grater
docker build -t grater-runner -f docker/Dockerfile .
