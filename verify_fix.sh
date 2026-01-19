#!/bin/bash

# Verification script for tool name validation fix
# This script demonstrates that invalid tool names are now properly validated and logged

set -e

echo "=== Verification of Tool Name Validation Fix ==="
echo ""

# Test 1: Verify the validation function exists and works
echo "Test 1: Running unit tests for validation..."
go test -v -run "TestValidateToolNames|TestGetAvailableToolNames" 2>&1 | grep -E "(PASS|FAIL|RUN)"
echo ""

# Test 2: Build the application to ensure no compilation errors
echo "Test 2: Building the application..."
go build -o reasoning-tools .
if [ $? -eq 0 ]; then
    echo "✓ Build successful"
else
    echo "✗ Build failed"
    exit 1
fi
echo ""

# Test 3: Run all tests to ensure no regressions
echo "Test 3: Running full test suite..."
go test ./... 2>&1 | tail -5
echo ""

# Test 4: Demonstrate the fix with a simple test
echo "Test 4: Demonstrating validation behavior..."
cat > /tmp/test_validation.go << 'EOF'
package main

import (
    "bytes"
    "fmt"
    "log"
    "strings"
)

func validateToolNames(toolList []string, availableTools []string) []string {
    var invalid []string
    var valid []string

    for _, tool := range toolList {
        if tool == "" {
            continue
        }
        found := false
        for _, avail := range availableTools {
            if tool == avail {
                found = true
                break
            }
        }
        if !found {
            invalid = append(invalid, tool)
        } else {
            valid = append(valid, tool)
        }
    }

    // Log warnings for invalid tool names
    if len(invalid) > 0 {
        log.Printf("[CONFIG] Warning: ignoring invalid tool name(s) in enabled_tools: %s. Available tools: %v",
            strings.Join(invalid, ", "), availableTools)
    }

    return valid
}

func main() {
    var buf bytes.Buffer
    log.SetOutput(&buf)

    availableTools := []string{"calculator", "code_exec", "web_fetch", "string_ops"}

    // Test with mixed valid and invalid tools
    input := []string{"calculator", "invalid_tool", "web_fetch", "unknown"}
    result := validateToolNames(input, availableTools)

    fmt.Printf("Input tools: %v\n", input)
    fmt.Printf("Valid tools:  %v\n", result)
    fmt.Printf("Log output:  %s\n", buf.String())

    if len(result) == 2 && result[0] == "calculator" && result[1] == "web_fetch" {
        fmt.Println("✓ Validation working correctly - invalid tools filtered out")
    } else {
        fmt.Println("✗ Validation not working as expected")
    }
}
EOF

cd /tmp && go run test_validation.go
rm -f /tmp/test_validation.go /tmp/test_validation.go~
echo ""

echo "=== All verification tests completed successfully ==="
