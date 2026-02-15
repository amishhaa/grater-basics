#!/bin/sh
set -e

MODULE="$MODULE"
REPO="$REPO"
BASE_REF="$BASE_REF"
HEAD_REF="$HEAD_REF"
TIMEOUT="${TIMEOUT:-300}"

RESULT=$(jq -n \
    --arg module "$MODULE" \
    --arg base_ref "$BASE_REF" \
    --arg head_ref "$HEAD_REF" \
    '{
        "module": $module,
        "base": {
            "ref": $base_ref,
            "passed": false,
            "error": "",
            "skipped": false
        },
        "head": {
            "ref": $head_ref,
            "passed": false,
            "error": "",
            "skipped": false
        }
    }')

echo ""
echo "════════════════════════════════════════════════════════════════════════════════"
echo "🔬 TEST RUN: $MODULE"
echo "   Against repo: $REPO"
echo "   Base ref: $BASE_REF"
echo "   Head ref: $HEAD_REF"
echo "   Timeout: ${TIMEOUT}s"
echo "   Started at: $(date)"
echo "════════════════════════════════════════════════════════════════════════════════"
echo ""

REPO_CLEAN=$(echo "$REPO" | sed 's|https://https://|https://|g' | sed 's|http://http://|http://|g' | sed 's|\.git$||')
REPO_MODULE=$(echo "$REPO_CLEAN" | sed 's|https://||' | sed 's|http://||' | sed 's|www\.||')

WORK_DIR="/work/$(echo $MODULE | tr '/' '_')_$(date +%s)"
mkdir -p "$WORK_DIR"
cd "$WORK_DIR"
echo "📁 Workspace: $WORK_DIR"

echo ""
echo "📦 Cloning dependency repo: $REPO_CLEAN"
case "$REPO_CLEAN" in
    http*) REPO_URL="$REPO_CLEAN" ;;
    *) REPO_URL="https://$REPO_CLEAN" ;;
esac
case "$REPO_URL" in
    *.git) ;; *) REPO_URL="$REPO_URL.git" ;;
esac

echo "   Cloning from: $REPO_URL"
if ! timeout $TIMEOUT git clone "$REPO_URL" dependency-repo; then
    echo "❌ Timeout or failed to clone dependency repo"
    RESULT=$(echo "$RESULT" | jq '.base.skipped = true | .base.error = "Clone timeout or failed" | .head.skipped = true | .head.error = "Clone timeout or failed"')
    echo "$RESULT"
    exit 0
fi

echo ""
echo "📦 Cloning dependent module: $MODULE"
if ! timeout $TIMEOUT git clone "https://$MODULE.git" dependent-module 2>/dev/null; then
    echo "❌ Timeout or failed to clone $MODULE"
    RESULT=$(echo "$RESULT" | jq '.base.skipped = true | .base.error = "Module clone timeout or failed" | .head.skipped = true | .head.error = "Module clone timeout or failed"')
    echo "$RESULT"
    exit 0
fi

test_ref() {
    local ref="$1"
    local ref_type="$2"
    local timeout_file="$WORK_DIR/timeout_$ref_type.txt"
    
    echo ""
    echo "════════════════════════════════════════════════════════════════════════════"
    echo "🔍 Testing with dependency at $ref_type: $ref"
    echo "════════════════════════════════════════════════════════════════════════════"

    cd "$WORK_DIR/dependency-repo"

    echo "   🔄 Fetching ref: $ref"
    if ! timeout $TIMEOUT git fetch origin "$ref" 2>"$timeout_file"; then
        exit_code=$?
        if [ $exit_code -eq 124 ]; then
            echo "   ⏰ Timeout fetching ref $ref"
            RESULT=$(echo "$RESULT" | jq --arg ref_type "$ref_type" '.[$ref_type].skipped = true | .[$ref_type].error = "Fetch timeout"')
        else
            echo "   ❌ Failed to fetch ref $ref"
            RESULT=$(echo "$RESULT" | jq --arg ref_type "$ref_type" '.[$ref_type].passed = false | .[$ref_type].error = "Fetch failed: ref does not exist"')
        fi
        return
    fi

    echo "   🔄 Checking out ref: $ref"
    if ! timeout $TIMEOUT git checkout FETCH_HEAD 2>"$timeout_file"; then
        exit_code=$?
        if [ $exit_code -eq 124 ]; then
            echo "   ⏰ Timeout checking out $ref"
            RESULT=$(echo "$RESULT" | jq --arg ref_type "$ref_type" '.[$ref_type].skipped = true | .[$ref_type].error = "Checkout timeout"')
        else
            echo "   ❌ Failed to checkout $ref"
            RESULT=$(echo "$RESULT" | jq --arg ref_type "$ref_type" '.[$ref_type].passed = false | .[$ref_type].error = "Checkout failed"')
        fi
        return
    fi
    
    echo "   ✅ Dependency checkout successful"
    echo "   📍 Current commit: $(git rev-parse --short HEAD)"

    cd "$WORK_DIR/dependent-module"

    echo "   🔄 Using local dependency at $WORK_DIR/dependency-repo"
    go mod edit -dropreplace="$REPO_MODULE" 2>/dev/null || true
    
    if ! go mod edit -replace "$REPO_MODULE=$WORK_DIR/dependency-repo" 2>/dev/null; then
        echo "   ❌ Failed to add replace directive"
        RESULT=$(echo "$RESULT" | jq --arg ref_type "$ref_type" '.[$ref_type].passed = false | .[$ref_type].error = "Failed to add replace directive"')
        return
    fi

    if [ -d "vendor" ]; then
        echo "   📁 Removing vendor directory..."
        rm -rf vendor
    fi

    echo "   📦 Downloading dependencies..."
    if ! timeout $TIMEOUT go mod download 2>"$timeout_file"; then
        exit_code=$?
        if [ $exit_code -eq 124 ]; then
            echo "   ⏰ Timeout downloading dependencies"
            RESULT=$(echo "$RESULT" | jq --arg ref_type "$ref_type" '.[$ref_type].skipped = true | .[$ref_type].error = "Dependency download timeout"')
            return
        fi
    fi
    
    echo "   🔨 Building..."
    if ! timeout $TIMEOUT go build -mod=mod ./... 2>build_error.txt; then
        exit_code=$?
        if [ $exit_code -eq 124 ]; then
            echo "   ⏰ Timeout during build"
            RESULT=$(echo "$RESULT" | jq --arg ref_type "$ref_type" '.[$ref_type].skipped = true | .[$ref_type].error = "Build timeout"')
            return
        else
            error_msg=$(cat build_error.txt | head -1 | sed 's/"/\\"/g')
            echo "   ❌ Build failed"
            RESULT=$(echo "$RESULT" | jq --arg ref_type "$ref_type" --arg error "$error_msg" '.[$ref_type].passed = false | .[$ref_type].error = $error')
            return
        fi
    fi
    
    echo "   🧪 Testing..."
    if ! timeout $TIMEOUT go test -mod=mod ./... 2>test_error.txt; then
        exit_code=$?
        if [ $exit_code -eq 124 ]; then
            echo "   ⏰ Timeout during tests"
            RESULT=$(echo "$RESULT" | jq --arg ref_type "$ref_type" '.[$ref_type].skipped = true | .[$ref_type].error = "Test timeout"')
            return
        else
            error_msg=$(cat test_error.txt | head -1 | sed 's/"/\\"/g')
            echo "   ❌ Tests failed"
            RESULT=$(echo "$RESULT" | jq --arg ref_type "$ref_type" --arg error "$error_msg" '.[$ref_type].passed = false | .[$ref_type].error = $error')
            return
        fi
    fi
    
    echo "   ✅ Tests passed"
    RESULT=$(echo "$RESULT" | jq --arg ref_type "$ref_type" '.[$ref_type].passed = true | .[$ref_type].error = "" | .[$ref_type].skipped = false')
}

test_ref "$BASE_REF" "base"

test_ref "$HEAD_REF" "head"

cd /
rm -rf "$WORK_DIR"
echo "   🧹 Workspace cleaned"

BASE_PASSED=$(echo "$RESULT" | jq -r '.base.passed')
BASE_ERROR=$(echo "$RESULT" | jq -r '.base.error')
BASE_SKIPPED=$(echo "$RESULT" | jq -r '.base.skipped')
HEAD_PASSED=$(echo "$RESULT" | jq -r '.head.passed')
HEAD_ERROR=$(echo "$RESULT" | jq -r '.head.error')
HEAD_SKIPPED=$(echo "$RESULT" | jq -r '.head.skipped')

echo ""
echo "════════════════════════════════════════════════════════════════════════════════"
echo "📊 FINAL RESULTS for $MODULE"
if [ "$BASE_SKIPPED" = "true" ]; then
    echo "   Base ($BASE_REF): ⏰ SKIPPED - $BASE_ERROR"
else
    echo "   Base ($BASE_REF): $BASE_PASSED"
    if [ "$BASE_PASSED" = "false" ] && [ -n "$BASE_ERROR" ] && [ "$BASE_ERROR" != "null" ]; then
        echo "       Error: $BASE_ERROR"
    fi
fi

if [ "$HEAD_SKIPPED" = "true" ]; then
    echo "   Head ($HEAD_REF): ⏰ SKIPPED - $HEAD_ERROR"
else
    echo "   Head ($HEAD_REF): $HEAD_PASSED"
    if [ "$HEAD_PASSED" = "false" ] && [ -n "$HEAD_ERROR" ] && [ "$HEAD_ERROR" != "null" ]; then
        echo "       Error: $HEAD_ERROR"
    fi
fi

if [ "$BASE_SKIPPED" = "true" ] || [ "$HEAD_SKIPPED" = "true" ]; then
    echo "   Overall: ⏸️  INCOMPLETE (some tests skipped due to timeout)"
elif [ "$BASE_PASSED" = "true" ] && [ "$HEAD_PASSED" = "true" ]; then
    echo "   Overall: ✅ PASS (both refs work)"
elif [ "$BASE_PASSED" = "true" ] && [ "$HEAD_PASSED" = "false" ]; then
    echo "   Overall: ⚠️  REGRESSION (base works, head fails)"
elif [ "$BASE_PASSED" = "false" ] && [ "$HEAD_PASSED" = "true" ]; then
    echo "   Overall: 🎉 FIXED (base fails, head works)"
else
    echo "   Overall: ❌ BROKEN (both refs fail)"
fi
echo "════════════════════════════════════════════════════════════════════════════════"

echo "$RESULT"