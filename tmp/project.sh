#!/bin/sh

set -eu

DIR=${1:-.}
OUT=${2:-test.md}

# Find all regular files under DIR_ABS (excluding directories), process null-delimited to be safe with spaces/newlines
find "$DIR" -type f | while IFS= read -r file; do
  # write header line with leading dash and absolute-from-dir path
  printf -- "\n- /%s\n" "$file" >>"$OUT"

  # open fenced code block with 'go' language tag
  printf -- '```go\n' >>"$OUT"

  if [ -r "$file" ]; then
    cat "$file" >>"$OUT"
  else
    printf -- "// cannot read file: %s\n" "$file" >>"$OUT"
  fi

  # close fenced code block
  printf -- '\n```\n' >>"$OUT"
done

echo "Done. Output saved to $OUT"
