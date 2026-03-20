#!/bin/bash

# ============================================
# Test Fixtures Generation Script - Minimal for Node Tests
# ============================================

# Handle debug flags
while [[ "$1" == -* ]]; do
  case "$1" in
    -x|-v) shift ;;
    *) break ;;
  esac
done

# Source library functions
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/lib.sh" || exit 1

# --- Setup ---
DEFAULT_OUTPUT_DIR="$SCRIPT_DIR/data"
DEFAULT_TEMP_DIR="$SCRIPT_DIR/tmp"
OUTPUT_DIR="${1:-$DEFAULT_OUTPUT_DIR}"
mkdir -p "$DEFAULT_TEMP_DIR"
TEMP_DIR=$(mktemp -d "$DEFAULT_TEMP_DIR/ipfs-test-data.XXXXXX")
echo "Generating test data in: $TEMP_DIR"
mkdir -p -- "${OUTPUT_DIR}"

# Initialize cleanup flag
CLEANUP_DONE=""
trap cleanup EXIT

# Check dependencies
check_dependencies || exit 1
check_ipfs_running || exit 1

# --- Test Parameters ---
# Test sizes include edge cases for chunk detection:
# - 240000: below chunk threshold (245760)
# - 256000: just above threshold, within typical range (262144)
# - 262144: typical chunk size
# - 524288: multi-chunk file
FILE_SIZES="240000 256000 262144 524288"
MISSING_BLOCKS="0 1"

# --- Generate Raw Data Tests ---
echo -e "\n=== Generating Raw Data Tests ==="
for SIZE in $FILE_SIZES; do
  for MISSING in $MISSING_BLOCKS; do
    echo "Creating test file of size ${SIZE} bytes (missing: ${MISSING})"
    FILE="$TEMP_DIR/data_${SIZE}_${MISSING}.data"
    create_file "$FILE" "$SIZE" "txt"

    if [ "$SIZE" -eq 0 ]; then
      echo "Created empty file"
      echo "File: $FILE, Size: $SIZE, Missing: $MISSING, CID: N/A" > "$OUTPUT_DIR/data_${SIZE}_${MISSING}.info"
      continue
    fi

    CID=$(add_to_ipfs "$FILE")
    if [ -z "$CID" ]; then
      echo "Error: Failed to generate CID for $FILE"
      continue
    fi

    CIDV1=$(ipfs cid format -v 1 -b base32 "$CID" 2>/dev/null || echo "$CID")

    BLOCK_DATA_FILE="$OUTPUT_DIR/data_${SIZE}_${MISSING}.block"
    if ! export_block "$CID" "$BLOCK_DATA_FILE"; then
      echo "Error: Failed to export block $CID"
      continue
    fi
    
    if [ "$MISSING" -eq 1 ]; then 
      remove_block "$CID"
    fi
    
    IS_PARTIAL=$(is_likely_chunk "$SIZE")

    echo "Generated: $FILE -> $CIDV1"
    create_info_file "${OUTPUT_DIR}/data_${SIZE}_${MISSING}.info.json" \
      --file "$FILE" \
      --size "$SIZE" \
      --raw_block_size "$SIZE" \
      --missing "$MISSING" \
      --cid "$CIDV1" \
      --is_partial "$IS_PARTIAL"
  done
done

# --- Build Protobuf Generator ---
echo -e "\n=== Building Protobuf Generator ==="
echo "Building protobuf generator..."
(cd "$SCRIPT_DIR/.." && go build -o "$TEMP_DIR/protobuf_generator" ./fixtures/protobuf_generator.go)

# --- Generate Protobuf Test Data ---
echo -e "\n=== Generating Protobuf Test Data ==="
for SIZE in $FILE_SIZES; do
  for MISSING in $MISSING_BLOCKS; do
    echo "Generating protobuf data (size: ${SIZE}, missing: ${MISSING})..."
    
    CID=$(OUTPUT_DIR="$OUTPUT_DIR" "$TEMP_DIR/protobuf_generator" -size "$SIZE" -partial "$MISSING")
    echo "Generated CID: $CID"
    
    BLOCK_DATA_FILE="${OUTPUT_DIR}/protobuf_${SIZE}_${MISSING}.block"
    mkdir -p -- "${OUTPUT_DIR}"
    ipfs block get "$CID" > "$BLOCK_DATA_FILE" 2>/dev/null || { echo "Failed to export block"; continue; }
    
    IS_PARTIAL=$(is_likely_chunk "$SIZE")

    info_file="${OUTPUT_DIR}/protobuf_${SIZE}_${MISSING}.info.json"
    if [[ ! -f "$info_file" ]]; then
      echo "Error: Protobuf info file not created: $info_file"
      exit 1
    fi

    if [ "$MISSING" -eq 1 ]; then
      ipfs block rm "$CID" >/dev/null 2>&1 || true
    fi
  done
done

# Cleanup
rm -f "$TEMP_DIR/protobuf_generator"

echo -e "\n=== Test data generation complete. ==="
echo "Output directory: $OUTPUT_DIR"
