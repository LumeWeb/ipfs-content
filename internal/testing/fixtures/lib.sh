#!/bin/bash

# ============================================
# Test Fixture Generation Library (Minimal for Node Tests)
# ============================================

# Check if IPFS is running
check_ipfs_running() {
  if ! ipfs id >/dev/null 2>&1; then
    echo "Error: IPFS daemon not running. Please start it first with 'ipfs daemon'"
    return 1
  fi
  return 0
}

# Check dependencies
check_dependencies() {
  if ! command -v jq &> /dev/null; then
    echo "Error: jq is required but not installed. Please install jq first."
    return 1
  fi
  return 0
}

# ============================================
# IPFS Operations
# ============================================

# Get block size from ipfs block stat output
get_block_size() {
  local CID="$1"
  if [ -z "$CID" ]; then
    echo "Error: Empty CID provided" >&2
    return 1
  fi
  
  local STATS
  STATS=$(ipfs block stat -- "$CID" 2>/dev/null)
  if [ $? -ne 0 ]; then
    echo "Error: Failed to get block stats for $CID" >&2
    return 1
  fi
  
  local SIZE
  SIZE=$(echo "$STATS" | grep 'Size:' | awk '{print $2}')
  if [ -z "$SIZE" ]; then
    echo "Error: Could not parse size from block stats" >&2
    return 1
  fi
  
  echo "$SIZE"
  return 0
}

# Add a file to IPFS and return the CID
add_to_ipfs() {
  local FILE="$1"
  if [ ! -e "$FILE" ]; then
    echo "Error: File '$FILE' does not exist" >&2
    return 1
  fi
  
  local OUTPUT
  
  # For node tests, we use raw codec for data files
  if [[ "$FILE" == *data_* ]]; then
    OUTPUT=$(ipfs dag put --store-codec=raw --input-codec=raw --allow-big-block "$FILE" 2>&1)
    if [ $? -ne 0 ]; then
      echo "Error: Failed to calculate raw CID for $FILE: $OUTPUT" >&2
      return 1
    fi
  else
    OUTPUT=$(ipfs add -Q --pin=false "$FILE" 2>&1)
    if [ $? -ne 0 ]; then
      echo "Error: Failed to add file $FILE to IPFS: $OUTPUT" >&2
      return 1
    fi
  fi
  echo "$OUTPUT"
}

# Export a block
export_block() {
  local CID="$1"
  local OUTPUT_FILE="$2"
  if [ -z "$CID" ]; then
    echo "Error: Empty CID provided" >&2
    return 1
  fi
  if ! ipfs block get "$CID" > "$OUTPUT_FILE" 2>/dev/null; then
    echo "Error: Failed to export block '$CID'" >&2
    return 1
  fi
  return 0
}

# Remove a block
remove_block() {
  local CID="$1"
  [ -z "$CID" ] && return 1
  echo "Removing block: $CID"
  
  if ! ipfs block rm "$CID" 2>/dev/null && ! ipfs dag rm "$CID" 2>/dev/null; then
    echo "Warning: Failed to remove block $CID (may not exist)"
    return 1
  fi
  return 0
}

# ============================================
# File Operations
# ============================================

# Determine if a size is likely a chunk
is_likely_chunk() {
  local size=$1
  local threshold=245760
  local typical=262144
  if [[ "$size" -ge "$threshold" && "$size" -le "$typical" ]]; then
    echo "true"
  else
    echo "false"
  fi
}

# Create a file with specific content
create_file() {
  local FILE="$1"
  local SIZE="$2"
  local TYPE="$3"

  case "$TYPE" in
    txt)
      # Create repeated pattern for deterministic output
      local pattern="IPFS test data. "
      local pattern_len=${#pattern}
      local pattern_count=$((SIZE / pattern_len))
      for ((i=0; i<pattern_count; i++)); do
        echo -n "$pattern" >> "$FILE"
      done
      local remaining=$((SIZE % pattern_len))
      [ $remaining -gt 0 ] && echo -n "${pattern:0:$remaining}" >> "$FILE"
      ;;
    *)  # Default to random data
      head -c "$SIZE" /dev/urandom > "$FILE"
      ;;
  esac
}

# ============================================
# Info File Operations
# ============================================

# Create standardized JSON info files
create_info_file() {
  local output_file="$1"
  shift
  
  if [[ -f "$output_file" && "$output_file" == *protobuf_* ]]; then
    return 0
  fi

  local jq_args=()
  local jq_query='{}'

  while [ $# -gt 0 ]; do
    case "$1" in
      --file)
        jq_query+=' | .file = $file'
        jq_args+=(--arg file "$2")
        shift 2
        ;;
      --dir)
        jq_query+=' | .dir = $dir'
        jq_args+=(--arg dir "$2")
        shift 2
        ;;
      --size)
        jq_query+=' | .size = ($size | tonumber)'
        jq_args+=(--arg size "$2")
        shift 2
        ;;
      --missing)
        jq_query+=' | .missing = ($missing | tonumber)'
        jq_args+=(--arg missing "$2")
        shift 2
        ;;
      --cid)
        jq_query+=' | .cid = $cid'
        jq_args+=(--arg cid "$2")
        shift 2
        ;;
      --is_partial)
        jq_query+=' | .is_partial = ($is_partial | test("true"))'
        jq_args+=(--arg is_partial "$2")
        shift 2
        ;;
      --type)
        jq_query+=' | .type = $type'
        jq_args+=(--arg type "$2")
        shift 2
        ;;
      --message_size)
        jq_query+=' | .message_size = ($message_size | tonumber)'
        jq_args+=(--arg message_size "$2")
        shift 2
        ;;
      --raw_block_size)
        jq_query+=' | .raw_block_size = ($raw_block_size | tonumber)'
        jq_args+=(--arg raw_block_size "$2")
        shift 2
        ;;
      *) shift ;;
    esac
  done

  mkdir -p -- "$(dirname "$output_file")"
  
  if [[ -f "$output_file" ]]; then
    local new_json
    new_json=$(jq -n "${jq_args[@]}" "$jq_query") || return 1
    # Merge with existing file, but new values override old values
    jq -n --argjson new "$new_json" 'input as $old | $old * $new' "$output_file" > "${output_file}.tmp" && mv "${output_file}.tmp" "$output_file"
  else
    jq -n "${jq_args[@]}" "$jq_query" > "$output_file"
  fi
}

# ============================================
# Cleanup Functions
# ============================================

# Cleanup handler
cleanup() {
  if [ -z "$CLEANUP_DONE" ]; then
    CLEANUP_DONE=1
    local exit_code=$?
    local TEMP_DIR="${1:-$TEMP_DIR}"
    echo -e "\nCleaning up temporary directory..."
    rm -rf "$TEMP_DIR"
    if [ $exit_code -ne 0 ]; then
      echo "Script failed with exit code $exit_code"
    fi
    exit $exit_code
  fi
}
