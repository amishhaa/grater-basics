#!/bin/sh
set -e

MODULE="$MODULE"
REPO="$REPO"
BASE_REF="$BASE_REF"
HEAD_REF="$HEAD_REF"

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

echo ""
echo "📦 Cloning dependency repo: $REPO_CLEAN"
case "$REPO_CLEAN" in
    http*) REPO_URL="$REPO_CLEAN" ;;
    *) REPO_URL="https://$REPO_CLEAN" ;;
esac
case "$REPO_URL" in
    *.git) ;; *) REPO_URL="$REPO_URL.git" ;;
esac

if ! git clone --depth 1 "$REPO_URL" dependency-repo 2>/dev/null; then
    echo "⚠️  Shallow clone failed, trying full clone..."
    git clone "$REPO_URL" dependency-repo
fi

echo ""
echo "📦 Cloning dependent module: $MODULE"
if ! git clone "https://$MODULE.git" dependent-module 2>/dev/null; then
    echo "❌ Failed to clone $MODULE"
    echo "{\"module\":\"$MODULE\",\"base\":false,\"head\":false}"
    exit 0
fi

test_ref() {
    local ref="$1"
    local ref_name="$2"
    
    echo ""
    echo "════════════════════════════════════════════════════════════════════════════"
    echo "🔍 Testing with dependency at $ref_name: $ref"
    echo "════════════════════════════════════════════════════════════════════════════"
    
    cd "$WORK_DIR/dependency-repo"
    
    if ! git checkout "$ref" 2>/dev/null; then
        echo "   ⚠️  Could not checkout $ref directly, trying fetch..."
        git fetch origin "$ref" 2>/dev/null
        if ! git checkout FETCH_HEAD 2>/dev/null; then
            echo "   ❌ Failed to checkout $ref"
            echo "false"
            return
        fi
    fi
    echo "   ✅ Dependency checkout successful"
    
    cd "$WORK_DIR/dependent-module"
    
    echo "   🔄 Using local dependency at $WORK_DIR/dependency-repo"
    go mod edit -replace "$REPO_MODULE=$WORK_DIR/dependency-repo"
    
    if [ -d "vendor" ]; then
        echo "   📁 Removing vendor directory..."
        rm -rf vendor
    fi
    
    echo "   📦 Downloading other dependencies..."
    go mod download
    
    echo "   🔨 Building..."
    if ! go build -mod=mod ./...; then
        echo "   ❌ Build failed"
        echo "false"
        return
    fi
    
    echo "   🧪 Testing..."
    if go test -mod=mod ./...; then
        echo "   ✅ Tests passed"
        echo "true"
    else
        echo "   ❌ Tests failed"
        echo "false"
    fi
}

BASE_RESULT=$(test_ref "$BASE_REF" "base")

HEAD_RESULT=$(test_ref "$HEAD_REF" "head")

cd /
rm -rf "$WORK_DIR"
echo "   🧹 Workspace cleaned"

echo ""
echo "════════════════════════════════════════════════════════════════════════════════"
echo "📊 FINAL RESULTS for $MODULE"
echo "   Base (dependency at $BASE_REF): $BASE_RESULT"
echo "   Head (dependency at $HEAD_REF): $HEAD_RESULT"
echo "════════════════════════════════════════════════════════════════════════════════"

echo "{\"module\":\"$MODULE\",\"base\":$BASE_RESULT,\"head\":$HEAD_RESULT}"