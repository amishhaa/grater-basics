#!/bin/sh
# NOTE: No "set -e" - we handle all errors manually so the script never dies silently

MODULE="${MODULE:-}"
REPO="${REPO:-}"
BASE_REF="${BASE_REF:-}"
HEAD_REF="${HEAD_REF:-}"
TIMEOUT="${TIMEOUT:-300}"

WORK_DIR=""

# Build a safe JSON result using printf instead of jq, so we have a valid
# fallback even if jq is not installed or fails early.
make_result() {
    printf '{"module":"%s","base":{"ref":"%s","passed":false,"error":"%s","skipped":%s},"head":{"ref":"%s","passed":false,"error":"%s","skipped":%s}}' \
        "$MODULE" "$BASE_REF" "$1" "$2" "$HEAD_REF" "$3" "$4"
}

RESULT=$(make_result "interrupted" "true" "interrupted" "true")

# Always emit whatever RESULT we have to stdout so Go can parse it.
# Fires on Ctrl+C (INT), kill (TERM), or any exit.
cleanup() {
    echo "" >&2
    echo "⚠️  Exiting — emitting results" >&2
    if [ -n "$WORK_DIR" ] && [ -d "$WORK_DIR" ]; then
        cd /
        rm -rf "$WORK_DIR"
        echo "🧹 Workspace cleaned" >&2
    fi
    printf '%s\n' "$RESULT"
}
trap cleanup INT TERM EXIT

# jq is optional now — we use it to update RESULT when available, else fall back
jq_update() {
    # $1 = jq filter, rest = extra --arg pairs
    # Returns updated RESULT or leaves it unchanged if jq missing/fails
    if command -v jq >/dev/null 2>&1; then
        _new=$(printf '%s' "$RESULT" | jq "$@" 2>/dev/null) && RESULT="$_new"
    fi
}

# Detect CPU cores
if command -v nproc >/dev/null 2>&1; then
    CORES=$(nproc)
elif command -v sysctl >/dev/null 2>&1; then
    CORES=$(sysctl -n hw.ncpu 2>/dev/null || echo 4)
else
    CORES=4
fi

# Detect GPU
HAS_CUDA=false
HAS_ROCM=false
if command -v nvidia-smi >/dev/null 2>&1; then
    HAS_CUDA=true
    GPU_NAME=$(nvidia-smi --query-gpu=name --format=csv,noheader 2>/dev/null | head -n1)
    GPU_COUNT=$(nvidia-smi --query-gpu=count --format=csv,noheader 2>/dev/null | head -n1)
    echo "✅ NVIDIA GPU detected: $GPU_NAME ($GPU_COUNT device(s))" >&2
elif command -v rocminfo >/dev/null 2>&1; then
    HAS_ROCM=true
    echo "✅ AMD GPU detected - enabling ROCm support" >&2
else
    echo "ℹ️ No GPU detected - CPU-only mode" >&2
fi

echo "" >&2
echo "════════════════════════════════════════════════════════════════════════════════" >&2
echo "🔬 TEST RUN: $MODULE" >&2
echo "   Repo:     $REPO" >&2
echo "   Base ref: $BASE_REF" >&2
echo "   Head ref: $HEAD_REF" >&2
echo "   Timeout:  ${TIMEOUT}s | Cores: $CORES" >&2
echo "   Started:  $(date)" >&2
echo "════════════════════════════════════════════════════════════════════════════════" >&2
echo "" >&2

# Validate required env vars
if [ -z "$MODULE" ] || [ -z "$REPO" ] || [ -z "$BASE_REF" ] || [ -z "$HEAD_REF" ]; then
    echo "❌ Missing required env vars (MODULE, REPO, BASE_REF, HEAD_REF)" >&2
    RESULT=$(make_result "missing env vars" "true" "missing env vars" "true")
    exit 1
fi

# Go env
export GOMAXPROCS=$CORES
export GOGC=100
export GO111MODULE=on

# GPU env
if [ "$HAS_CUDA" = true ]; then
    export CUDA_VISIBLE_DEVICES=all
    export TF_GPU_ALLOCATOR=cuda_malloc_async
    export TF_CPP_MIN_LOG_LEVEL=1
    export CGO_CFLAGS="-I/usr/local/cuda/include"
    export CGO_LDFLAGS="-L/usr/local/cuda/lib64 -lcuda -lcudart"
    export CUDA_CACHE_MAXSIZE=2147483648
    export CUDA_CACHE_DISABLE=0
elif [ "$HAS_ROCM" = true ]; then
    export ROCM_VISIBLE_DEVICES=all
    export HIP_VISIBLE_DEVICES=all
    export HCC_AMDGPU_TARGET=gfx900,gfx906,gfx908,gfx90a
fi

REPO_CLEAN=$(echo "$REPO" | sed 's|https://https://|https://|g' | sed 's|http://http://|http://|g' | sed 's|\.git$||')
REPO_MODULE=$(echo "$REPO_CLEAN" | sed 's|https://||' | sed 's|http://||' | sed 's|www\.||')

WORK_DIR="/work/$(echo "$MODULE" | tr '/' '_')_$(date +%s)"
mkdir -p "$WORK_DIR"
cd "$WORK_DIR"
echo "📁 Workspace: $WORK_DIR" >&2

# Build repo URL
case "$REPO_CLEAN" in
    http*) REPO_URL="$REPO_CLEAN" ;;
    *)     REPO_URL="https://$REPO_CLEAN" ;;
esac
case "$REPO_URL" in
    *.git) ;;
    *)     REPO_URL="${REPO_URL}.git" ;;
esac

# Clone dependency repo
echo "" >&2
echo "📦 Cloning dependency repo: $REPO_URL" >&2
if ! timeout "$TIMEOUT" git clone --depth 1 "$REPO_URL" dependency-repo 2>&1 >&2; then
    echo "❌ Failed to clone dependency repo" >&2
    RESULT=$(make_result "Clone failed or timed out" "true" "Clone failed or timed out" "true")
    exit 1
fi

# Clone dependent module
echo "📦 Cloning dependent module: $MODULE" >&2
if ! timeout "$TIMEOUT" git clone --depth 1 "https://${MODULE}.git" dependent-module 2>&1 >&2; then
    echo "❌ Failed to clone module: $MODULE" >&2
    RESULT=$(make_result "Module clone failed or timed out" "true" "Module clone failed or timed out" "true")
    exit 1
fi

# --- test_ref function ---
test_ref() {
    _ref="$1"
    _type="$2"  # "base" or "head"
    _tfile="$WORK_DIR/timeout_${_type}.txt"

    echo "" >&2
    echo "════════════════════════════════════════════════════════════════════════════" >&2
    echo "🔍 Testing $MODULE with dependency at ${_type}: ${_ref}" >&2
    echo "════════════════════════════════════════════════════════════════════════════" >&2

    cd "$WORK_DIR/dependency-repo"

    echo "   🔄 Fetching ${_ref}..." >&2
    if ! timeout "$TIMEOUT" git fetch --jobs="$CORES" origin "$_ref" 2>"$_tfile"; then
        _code=$?
        if [ $_code -eq 124 ]; then
            echo "   ⏰ Fetch timed out" >&2
            jq_update --arg t "$_type" '.[$t].skipped = true | .[$t].error = "Fetch timeout"'
        else
            echo "   ❌ Fetch failed" >&2
            jq_update --arg t "$_type" '.[$t].passed = false | .[$t].error = "Fetch failed: ref does not exist"'
        fi
        return 0
    fi

    echo "   🔄 Checking out FETCH_HEAD..." >&2
    if ! timeout "$TIMEOUT" git checkout FETCH_HEAD 2>"$_tfile"; then
        _code=$?
        if [ $_code -eq 124 ]; then
            echo "   ⏰ Checkout timed out" >&2
            jq_update --arg t "$_type" '.[$t].skipped = true | .[$t].error = "Checkout timeout"'
        else
            echo "   ❌ Checkout failed" >&2
            jq_update --arg t "$_type" '.[$t].passed = false | .[$t].error = "Checkout failed"'
        fi
        return 0
    fi

    echo "   ✅ At commit: $(git rev-parse --short HEAD)" >&2

    cd "$WORK_DIR/dependent-module"

    go mod edit -dropreplace="$REPO_MODULE" 2>/dev/null || true
    if ! go mod edit -replace "${REPO_MODULE}=${WORK_DIR}/dependency-repo" 2>/dev/null; then
        echo "   ❌ Failed to add replace directive" >&2
        jq_update --arg t "$_type" '.[$t].passed = false | .[$t].error = "Failed to add replace directive"'
        return 0
    fi

    [ -d "vendor" ] && rm -rf vendor && echo "   📁 Removed vendor dir" >&2

    echo "   📦 Downloading dependencies..." >&2
    export GOPROXY="${GOPROXY:-direct}"
    export GOSUMDB="${GOSUMDB:-off}"
    export GONOSUMDB="${GONOSUMDB:-*}"

    if ! timeout "$TIMEOUT" go mod download -json >&2 2>"$_tfile"; then
        _code=$?
        if [ $_code -eq 124 ]; then
            echo "   ⏰ Dependency download timed out" >&2
            jq_update --arg t "$_type" '.[$t].skipped = true | .[$t].error = "Dependency download timeout"'
            return 0
        fi
        echo "   ⚠️  go mod download had errors, continuing anyway..." >&2
    fi

    echo "   🔨 Building with $CORES cores..." >&2
    export GOCACHE="${WORK_DIR}/go-build"
    mkdir -p "$GOCACHE"

    _build_tags=""
    if [ "$HAS_CUDA" = true ]; then
        _build_tags="-tags=cuda"
        echo "   🚀 CUDA build enabled" >&2
    elif [ "$HAS_ROCM" = true ]; then
        _build_tags="-tags=rocm"
        echo "   🚀 ROCm build enabled" >&2
    fi

    # shellcheck disable=SC2086
    if ! timeout "$TIMEOUT" go build -p "$CORES" -mod=mod $_build_tags ./... >&2 2>build_error.txt; then
        _code=$?
        if [ $_code -eq 124 ]; then
            echo "   ⏰ Build timed out" >&2
            jq_update --arg t "$_type" '.[$t].skipped = true | .[$t].error = "Build timeout"'
        else
            _err=$(head -5 build_error.txt | tr '"' "'" | tr '\n' ' ')
            echo "   ❌ Build failed: $_err" >&2
            jq_update --arg t "$_type" --arg e "$_err" '.[$t].passed = false | .[$t].error = $e'
        fi
        return 0
    fi

    echo "   🧪 Running tests with $CORES cores..." >&2

    _test_tags=""
    if [ "$HAS_CUDA" = true ]; then
        _test_tags="-tags=cuda"
        export CUDA_LAUNCH_BLOCKING=1
        export TF_GPU_THREAD_MODE=gpu_private
    elif [ "$HAS_ROCM" = true ]; then
        _test_tags="-tags=rocm"
    fi

    # shellcheck disable=SC2086
    if ! timeout "$TIMEOUT" go test -p "$CORES" -parallel "$CORES" -vet=off -count=1 -mod=mod $_test_tags ./... >&2 2>test_error.txt; then
        _code=$?
        if [ $_code -eq 124 ]; then
            echo "   ⏰ Tests timed out" >&2
            jq_update --arg t "$_type" '.[$t].skipped = true | .[$t].error = "Test timeout"'
        else
            _err=$(head -5 test_error.txt | tr '"' "'" | tr '\n' ' ')
            echo "   ❌ Tests failed: $_err" >&2
            jq_update --arg t "$_type" --arg e "$_err" '.[$t].passed = false | .[$t].error = $e'
        fi
        return 0
    fi

    echo "   ✅ Tests passed" >&2
    jq_update --arg t "$_type" '.[$t].passed = true | .[$t].error = "" | .[$t].skipped = false'
}

# Run both refs
test_ref "$BASE_REF" "base"
test_ref "$HEAD_REF" "head"

# Manual cleanup (trap will see WORK_DIR="" and skip)
cd /
rm -rf "$WORK_DIR"
WORK_DIR=""
echo "🧹 Workspace cleaned" >&2

# Print summary to stderr
BASE_PASSED=$(printf '%s' "$RESULT" | jq -r '.base.passed' 2>/dev/null || echo "false")
BASE_SKIPPED=$(printf '%s' "$RESULT" | jq -r '.base.skipped' 2>/dev/null || echo "false")
BASE_ERROR=$(printf '%s' "$RESULT" | jq -r '.base.error' 2>/dev/null || echo "")
HEAD_PASSED=$(printf '%s' "$RESULT" | jq -r '.head.passed' 2>/dev/null || echo "false")
HEAD_SKIPPED=$(printf '%s' "$RESULT" | jq -r '.head.skipped' 2>/dev/null || echo "false")
HEAD_ERROR=$(printf '%s' "$RESULT" | jq -r '.head.error' 2>/dev/null || echo "")

echo "" >&2
echo "════════════════════════════════════════════════════════════════════════════════" >&2
echo "📊 FINAL RESULTS for $MODULE" >&2

_fmt_ref() {
    _label="$1" _ref="$2" _passed="$3" _skipped="$4" _err="$5"
    if [ "$_skipped" = "true" ]; then
        echo "   $_label ($_ref): ⏰ SKIPPED${_err:+ - $_err}" >&2
    elif [ "$_passed" = "true" ]; then
        echo "   $_label ($_ref): ✅ PASS" >&2
    else
        echo "   $_label ($_ref): ❌ FAIL${_err:+ - $_err}" >&2
    fi
}
_fmt_ref "Base" "$BASE_REF" "$BASE_PASSED" "$BASE_SKIPPED" "$BASE_ERROR"
_fmt_ref "Head" "$HEAD_REF" "$HEAD_PASSED" "$HEAD_SKIPPED" "$HEAD_ERROR"

if [ "$BASE_SKIPPED" = "true" ] || [ "$HEAD_SKIPPED" = "true" ]; then
    echo "   Overall: ⏸️  INCOMPLETE" >&2
elif [ "$BASE_PASSED" = "true" ] && [ "$HEAD_PASSED" = "true" ]; then
    echo "   Overall: ✅ PASS" >&2
elif [ "$BASE_PASSED" = "true" ]; then
    echo "   Overall: ⚠️  REGRESSION" >&2
elif [ "$HEAD_PASSED" = "true" ]; then
    echo "   Overall: 🎉 FIXED" >&2
else
    echo "   Overall: ❌ BROKEN" >&2
fi

if [ "$HAS_CUDA" = true ]; then
    GPU_UTIL=$(nvidia-smi --query-gpu=utilization.gpu --format=csv,noheader 2>/dev/null | head -n1)
    GPU_MEM=$(nvidia-smi --query-gpu=memory.used --format=csv,noheader 2>/dev/null | head -n1)
    echo "   GPU: util=$GPU_UTIL mem=$GPU_MEM" >&2
fi
echo "════════════════════════════════════════════════════════════════════════════════" >&2

# Disarm trap — we emit JSON ourselves cleanly below
trap - INT TERM EXIT

# THIS IS THE ONLY PLACE JSON GOES TO STDOUT
printf '%s\n' "$RESULT"