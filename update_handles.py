#!/usr/bin/env python3
"""
Update Handle methods to use framework.ToolResult instead of string.
"""

import re
import sys


def update_file(filepath):
    with open(filepath, "r") as f:
        content = f.read()

    # Skip if already updated
    if (
        "framework.ToolResult" in content
        and "func (t *" in content
        and "Handle(ctx context.Context" in content
    ):
        print(f"  Already updated: {filepath}")
        return False

    # Pattern to match Handle method signature
    # func (t *Type) Handle(ctx context.Context, args map[string]interface{}) (string, error) {
    sig_pattern = r"(func \(t \*(\w+)\) Handle\(ctx context\.Context, args map\[string\]interface\{\}\) \()string, error\)"

    # Check if this file has Handle methods
    if not re.search(sig_pattern, content):
        print(f"  No Handle method found: {filepath}")
        return False

    # Update function signature
    content = re.sub(sig_pattern, r"\1framework.ToolResult, error)", content)

    # Update return statements
    # Pattern 1: return "", err -> return framework.ErrorResult(err)
    content = re.sub(r'return "", err\b', "return framework.ErrorResult(err)", content)

    # Pattern 2: return err -> return framework.ErrorResult(err)
    content = re.sub(
        r"\breturn err\b(?![\w])", "return framework.ErrorResult(err)", content
    )

    # Pattern 3: return "<literal>", nil -> return framework.TextResult("<literal>"), nil
    # This handles various string literals
    def replace_literal(match):
        literal = match.group(1)
        return f'return framework.TextResult("{literal}"), nil'

    content = re.sub(r'return "([^"]*)", nil', replace_literal, content)

    # Pattern 4: return string(b), nil -> return framework.TextResult(string(b)), nil
    content = re.sub(
        r"return string\(([^)]+)\), nil",
        r"return framework.TextResult(string(\1)), nil",
        content,
    )

    # Pattern 5: return b, nil (where b is a variable holding []byte)
    # We need to be careful here - let's handle common patterns
    # return json.Marshal(...) -> need to handle this specially

    # Pattern for json.Marshal with multi-line
    # Change return json.Marshal(...), nil pattern
    content = re.sub(
        r"return json\.Marshal\(([^)]+)\), nil",
        r"return framework.TextResult(func() string { b, _ := json.Marshal(\1); return string(b) }()), nil",
        content,
    )

    with open(filepath, "w") as f:
        f.write(content)

    print(f"  Updated: {filepath}")
    return True


if __name__ == "__main__":
    for filepath in sys.argv[1:]:
        update_file(filepath)
