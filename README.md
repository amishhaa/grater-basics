<!DOCTYPE html>
<html>
<head>
  <meta charset="UTF-8">
</head>
<body>

<h1>grater-basics</h1>

<h2>Setup Instructions</h2>

<h3>Prerequisites</h3>
<p>You need:</p>
<ul>
  <li>Go ≥ 1.21</li>
  <li>Docker</li>
  <li>Git</li>
</ul>

<h2>Install grater (from source)</h2>

<p>Clone the repo:</p>
<pre><code>git clone https://github.com/amishhaa/grater-basics.git
cd grater-basics</code></pre>

<p>Build and install the CLI:</p>
<pre><code>go install ./cmd/grater</code></pre>

<p>Make sure $GOPATH/bin is in your PATH:</p>
<pre><code>echo $PATH | grep go/bin</code></pre>

<pre><code>grater --help</code></pre>

<h2>Workspace</h2>

<p>grater uses a local workspace directory:</p>
<pre><code>.grater/</code></pre>

<p>This is created automatically when you run:</p>
<pre><code>grater prepare</code></pre>

<p>It stores:</p>
<ul>
  <li>modules.txt → list of downstream modules (Currently the functionality to fetch modules is not yet implemented)</li>
  <li>results.json → test results (when grater run is executed)</li>
</ul>

<h2>Usage</h2>

<h3>1. Prepare downstream modules</h3>
<pre><code>grater prepare</code></pre>

<p>Creates:</p>
<pre><code>.grater/modules.txt</code></pre>

<h3>2. Run tests //basic run functionality, currently heavily in development</h3>
<pre><code>grater run \
  --repo github.com/open-telemetry/opentelemetry-go \
  --base main \
  --head HEAD</code></pre>

<h3>3. View report (not implemented yet)</h3>
<pre><code>grater report</code></pre>

<h2>Docker runner</h2>

<p>Build the runner image:</p>
<pre><code>docker build -t grater-runner -f docker/Dockerfile .</code></pre>

<h2>Quick Start</h2>
<pre><code>go install ./cmd/grater
docker build -t grater-runner -f docker/Dockerfile .</code></pre>

</body>
</html>

