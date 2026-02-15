#!/bin/sh
set -e

MODULE="$MODULE"
REPO="$REPO"
BASE_REF="$BASE_REF"
HEAD_REF="$HEAD_REF"

# Initialize result structure
RESULT=$(jq -n \
    --arg module "$MODULE" \
    --arg base_ref "$BASE_REF" \
    --arg head_ref "$HEAD_REF" \
    '{
        "module": $module,
        "base": {
            "ref": $base_ref,
            "passed": false,
            "error": ""
        },
        "head": {
            "ref": $head_ref,
            "passed": false,
            "error": ""
        }
    }')

echo ""
echo "════════════════════════════════════════════════════════════════════════════════"
echo "🔬 TEST RUN: $MODULE"
echo "   Against repo: $REPO"
echo "   Base ref: $BASE_REF"
echo "   Head ref: $HEAD_REF"
echo "   Started at: $(date)"
echo "════════════════════════════════════════════════════════════════════════════════"
echo ""

REPO_CLEAN=$(echo "$REPO" | sed 's|https://https://|https://|g' | sed 's|http://http://|http://|g' | sed 's|\.git$||')
REPO_MODULE=$(echo "$REPO_CLEAN" | sed 's|https://||' | sed 's|http://||' | sed 's|www\.||')

WORK_DIR="/work/$(echo $MODULE | tr '/' '_')_$(date +%s)"
mkdir -p "$WORK_DIR"
cd "$WORK_DIR"
echo "📁 Workspace: $WORK_DIR"

# Clone dependency repo (the repo under test)
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
if ! git clone "$REPO_URL" dependency-repo; then
    echo "❌ Failed to clone dependency repo"
    RESULT=$(echo "$RESULT" | jq '.base.passed = false | .base.error = "Failed to clone dependency repo" | .head.passed = false | .head.error = "Failed to clone dependency repo"')
    echo "$RESULT"
    exit 0
fi

# Clone dependent module
echo ""
echo "📦 Cloning dependent module: $MODULE"
if ! git clone "https://$MODULE.git" dependent-module 2>/dev/null; then
    echo "❌ Failed to clone $MODULE"
    RESULT=$(echo "$RESULT" | jq '.base.passed = false | .base.error = "Failed to clone module" | .head.passed = false | .head.error = "Failed to clone module"')
    echo "$RESULT"
    exit 0
fi

# Function to test a specific ref
test_ref() {
    local ref="$1"
    local ref_type="$2"  # "base" or "head"
    
    echo ""
    echo "════════════════════════════════════════════════════════════════════════════"
    echo "🔍 Testing with dependency at $ref_type: $ref"
    echo "════════════════════════════════════════════════════════════════════════════"
    
    # Go to dependency repo
    cd "$WORK_DIR/dependency-repo"
    
    # Fetch and checkout
    echo "   🔄 Fetching ref: $ref"
    if git fetch origin "$ref" 2>/dev/null; then
        if git checkout FETCH_HEAD 2>/dev/null; then
            echo "   ✅ Dependency checkout successful"
            echo "   📍 Current commit: $(git rev-parse --short HEAD)"
        else
            echo "   ❌ Failed to checkout $ref"
            RESULT=$(echo "$RESULT" | jq --arg ref_type "$ref_type" '.[$ref_type].passed = false | .[$ref_type].error = "Checkout failed"')
            return
        fi
    else
        echo "   ❌ Failed to fetch ref $ref"
        RESULT=$(echo "$RESULT" | jq --arg ref_type "$ref_type" '.[$ref_type].passed = false | .[$ref_type].error = "Fetch failed: ref does not exist"')
        return
    fi
    
    # Go to dependent module
    cd "$WORK_DIR/dependent-module"
    
    # Replace with local dependency
    echo "   🔄 Using local dependency at $WORK_DIR/dependency-repo"
    go mod edit -dropreplace="$REPO_MODULE" 2>/dev/null || true
    
    if ! go mod edit -replace "$REPO_MODULE=$WORK_DIR/dependency-repo" 2>/dev/null; then
        echo "   ❌ Failed to add replace directive"
        RESULT=$(echo "$RESULT" | jq --arg ref_type "$ref_type" '.[$ref_type].passed = false | .[$ref_type].error = "Failed to add replace directive"')
        return
    fi
    
    # Remove vendor if exists
    if [ -d "vendor" ]; then
        echo "   📁 Removing vendor directory..."
        rm -rf vendor
    fi
    
    # Download dependencies
    echo "   📦 Downloading dependencies..."
    go mod download 2>/dev/null || true
    
    # Build
    echo "   🔨 Building..."
    if ! go build -mod=mod ./... 2>build_error.txt; then
        error_msg=$(cat build_error.txt | head -1 | sed 's/"/\\"/g')
        echo "   ❌ Build failed"
        RESULT=$(echo "$RESULT" | jq --arg ref_type "$ref_type" --arg error "$error_msg" '.[$ref_type].passed = false | .[$ref_type].error = $error')
        return
    fi
    
    # Test
    echo "   🧪 Testing..."
    if go test -mod=mod ./... 2>test_error.txt; then
        echo "   ✅ Tests passed"
        RESULT=$(echo "$RESULT" | jq --arg ref_type "$ref_type" '.[$ref_type].passed = true | .[$ref_type].error = ""')
    else
        error_msg=$(cat test_error.txt | head -1 | sed 's/"/\\"/g')
        echo "   ❌ Tests failed"
        RESULT=$(echo "$RESULT" | jq --arg ref_type "$ref_type" --arg error "$error_msg" '.[$ref_type].passed = false | .[$ref_type].error = $error')
    fi
}

# Test base ref
test_ref "$BASE_REF" "base"

# Test head ref
test_ref "$HEAD_REF" "head"

# Cleanup
cd /
rm -rf "$WORK_DIR"
echo "   🧹 Workspace cleaned"

# Extract results for summary
BASE_PASSED=$(echo "$RESULT" | jq -r '.base.passed')
BASE_ERROR=$(echo "$RESULT" | jq -r '.base.error')
HEAD_PASSED=$(echo "$RESULT" | jq -r '.head.passed')
HEAD_ERROR=$(echo "$RESULT" | jq -r '.head.error')

# Final output
echo ""
echo "════════════════════════════════════════════════════════════════════════════════"
echo "📊 FINAL RESULTS for $MODULE"
echo "   Base ($BASE_REF): $BASE_PASSED"
if [ "$BASE_PASSED" = "false" ] && [ -n "$BASE_ERROR" ] && [ "$BASE_ERROR" != "null" ]; then
    echo "       Error: $BASE_ERROR"
fi
echo "   Head ($HEAD_REF): $HEAD_PASSED"
if [ "$HEAD_PASSED" = "false" ] && [ -n "$HEAD_ERROR" ] && [ "$HEAD_ERROR" != "null" ]; then
    echo "       Error: $HEAD_ERROR"
fi

# Determine overall status
if [ "$BASE_PASSED" = "true" ] && [ "$HEAD_PASSED" = "true" ]; then
    echo "   Overall: ✅ PASS (both refs work)"
elif [ "$BASE_PASSED" = "true" ] && [ "$HEAD_PASSED" = "false" ]; then
    echo "   Overall: ⚠️  REGRESSION (base works, head fails)"
elif [ "$BASE_PASSED" = "false" ] && [ "$HEAD_PASSED" = "true" ]; then
    echo "   Overall: 🎉 FIXED (base fails, head works)"
else
    echo "   Overall: ❌ BROKEN (both refs fail)"
fi
echo "════════════════════════════════════════════════════════════════════════════════"

# Output the structured JSON result
echo "$RESULT"