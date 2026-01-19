package main

import (
	"context"
	"fmt"
	"testing"
)

func TestCalculatorNegativeNumbers(t *testing.T) {
	tool := &CalculatorTool{}

	testCases := []struct {
		input    string
		expected string
	}{
		{"-5", "-5"},
		{"-10 + 3", "-7"},
		{"3 + -2", "1"},
		{"2 * -5", "-10"},
		{"-3.5", "-3.5"},
		{"5", "5"},
		{"2 + 3", "5"},
		{"-2 * -3", "6"},
		{"10 / -2", "-5"},
		{"-5 - 3", "-8"},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			result, err := tool.Execute(context.Background(), tc.input)
			if err != nil {
				t.Errorf("Input %q: unexpected error: %v", tc.input, err)
				return
			}
			if result != tc.expected {
				t.Errorf("Input %q: expected %q, got %q", tc.input, tc.expected, result)
			}
		})
	}
}

func TestCalculatorScientificNotation(t *testing.T) {
	tool := &CalculatorTool{}

	testCases := []struct {
		input    string
		expected string
	}{
		{"1e10", "10000000000"},
		{"1e10 + 1", "10000000001"},
		{"2 * 1e5", "200000"},
		{"-1e10", "-10000000000"},
		{"1e-5", "0.00001"},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			result, err := tool.Execute(context.Background(), tc.input)
			if err != nil {
				t.Errorf("Input %q: unexpected error: %v", tc.input, err)
				return
			}
			if result != tc.expected {
				t.Errorf("Input %q: expected %q, got %q", tc.input, tc.expected, result)
			}
		})
	}
}

// TestPythonCodeValidation tests that validatePythonCode correctly accepts
// valid Python code that was previously incorrectly rejected.
func TestPythonCodeValidation(t *testing.T) {
	testCases := []struct {
		name        string
		code        string
		shouldError bool
		description string
	}{
		{
			name:        "semicolon_statement_separator",
			code:        "x = 1; y = 2; print(x, y)",
			shouldError: false,
			description: "Semicolon is a valid Python statement separator",
		},
		{
			name:        "multiple_semicolons",
			code:        "a = 1; b = 2; c = 3; result = a + b + c; print(result)",
			shouldError: false,
			description: "Multiple semicolons are valid in Python",
		},
		{
			name:        "semicolon_with_comment",
			code:        "x = 1; # set x\ny = 2; # set y\nprint(x + y)",
			shouldError: false,
			description: "Semicolon followed by comment is valid",
		},
		{
			name:        "simple_print",
			code:        "print('Hello, World!')",
			shouldError: false,
			description: "Simple print statement should pass",
		},
		{
			name:        "math_calculation",
			code:        "result = 2 + 2\nprint(result)",
			shouldError: false,
			description: "Simple math calculation should pass",
		},
		{
			name:        "dangerous_os_import",
			code:        "import os\nos.system('ls')",
			shouldError: true,
			description: "Dangerous os import should still be blocked",
		},
		{
			name:        "dangerous_subprocess_import",
			code:        "import subprocess\nsubprocess.run(['ls'])",
			shouldError: true,
			description: "Dangerous subprocess import should still be blocked",
		},
		{
			name:        "dangerous_eval",
			code:        "eval('print(1)')",
			shouldError: true,
			description: "Dangerous eval function should still be blocked",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validatePythonCode(context.Background(), tc.code)
			if tc.shouldError && err == nil {
				t.Errorf("%s: expected error for dangerous code, but got none (description: %s)", tc.name, tc.description)
			}
			if !tc.shouldError && err != nil {
				t.Errorf("%s: unexpected error for valid code: %v (description: %s)", tc.name, err, tc.description)
			}
		})
	}
}

// TestPythonCodeValidationAdvanced tests advanced security patterns
// that the enhanced validation should detect and block.
func TestPythonCodeValidationAdvanced(t *testing.T) {
	testCases := []struct {
		name        string
		code        string
		shouldError bool
		description string
	}{
		// Basic dangerous imports (string-based checks)
		{
			name:        "os_import_blocked",
			code:        "import os",
			shouldError: true,
			description: "os module import should be blocked",
		},
		{
			name:        "subprocess_import_blocked",
			code:        "import subprocess",
			shouldError: true,
			description: "subprocess module import should be blocked",
		},
		{
			name:        "sys_import_blocked",
			code:        "import sys",
			shouldError: true,
			description: "sys module import should be blocked",
		},
		// From imports
		{
			name:        "from_os_import_blocked",
			code:        "from os import system",
			shouldError: true,
			description: "from os import should be blocked",
		},
		{
			name:        "from_subprocess_import_blocked",
			code:        "from subprocess import run",
			shouldError: true,
			description: "from subprocess import should be blocked",
		},
		// Dangerous functions
		{
			name:        "eval_blocked",
			code:        "eval('1+1')",
			shouldError: true,
			description: "eval() function should be blocked",
		},
		{
			name:        "exec_blocked",
			code:        "exec('print(1)')",
			shouldError: true,
			description: "exec() function should be blocked",
		},
		{
			name:        "compile_blocked",
			code:        "compile('print(1)', '<string>', 'exec')",
			shouldError: true,
			description: "compile() function should be blocked",
		},
		{
			name:        "open_blocked",
			code:        "open('/etc/passwd')",
			shouldError: true,
			description: "open() function should be blocked",
		},
		{
			name:        "globals_blocked",
			code:        "print(globals())",
			shouldError: true,
			description: "globals() function should be blocked",
		},
		{
			name:        "locals_blocked",
			code:        "print(locals())",
			shouldError: true,
			description: "locals() function should be blocked",
		},
		{
			name:        "getattr_blocked",
			code:        "getattr(obj, 'attr')",
			shouldError: true,
			description: "getattr() function should be blocked",
		},
		{
			name:        "setattr_blocked",
			code:        "setattr(obj, 'attr', value)",
			shouldError: true,
			description: "setattr() function should be blocked",
		},
		// Dangerous dunder attributes
		{
			name:        "__class__blocked",
			code:        "x.__class__",
			shouldError: true,
			description: "__class__ attribute access should be blocked",
		},
		{
			name:        "__subclasses__blocked",
			code:        "x.__subclasses__()",
			shouldError: true,
			description: "__subclasses__ attribute access should be blocked",
		},
		{
			name:        "__mro__blocked",
			code:        "cls.__mro__",
			shouldError: true,
			description: "__mro__ attribute access should be blocked",
		},
		{
			name:        "__code__blocked",
			code:        "func.__code__",
			shouldError: true,
			description: "__code__ attribute access should be blocked",
		},
		{
			name:        "__globals__blocked",
			code:        "func.__globals__",
			shouldError: true,
			description: "__globals__ attribute access should be blocked",
		},
		{
			name:        "__dict__blocked",
			code:        "obj.__dict__",
			shouldError: true,
			description: "__dict__ attribute access should be blocked",
		},
		// New dangerous modules added in enhancement
		{
			name:        "multiprocessing_import_blocked",
			code:        "import multiprocessing",
			shouldError: true,
			description: "multiprocessing module import should be blocked",
		},
		{
			name:        "threading_import_blocked",
			code:        "import threading",
			shouldError: true,
			description: "threading module import should be blocked",
		},
		{
			name:        "signal_import_blocked",
			code:        "import signal",
			shouldError: true,
			description: "signal module import should be blocked",
		},
		{
			name:        "fcntl_import_blocked",
			code:        "import fcntl",
			shouldError: true,
			description: "fcntl module import should be blocked",
		},
		{
			name:        "resource_import_blocked",
			code:        "import resource",
			shouldError: true,
			description: "resource module import should be blocked",
		},
		{
			name:        "pty_import_blocked",
			code:        "import pty",
			shouldError: true,
			description: "pty module import should be blocked",
		},
		{
			name:        "tty_import_blocked",
			code:        "import tty",
			shouldError: true,
			description: "tty module import should be blocked",
		},
		// Additional bypass patterns
		{
			name:        "bytearray_blocked",
			code:        "bytearray('test', 'utf-8')",
			shouldError: true,
			description: "bytearray should be blocked (potential code obfuscation)",
		},
		{
			name:        "bytes_fromhex_blocked",
			code:        "bytes.fromhex('48656c6c6f')",
			shouldError: true,
			description: "bytes.fromhex should be blocked (potential code encoding)",
		},
		{
			name:        "bytes_decode_safe",
			code:        "b'test'.decode()",
			shouldError: false,
			description: "bytes.decode is a common operation and not inherently dangerous",
		},
		{
			name:        "type_function_blocked",
			code:        "type(obj)",
			shouldError: true,
			description: "type() function should be blocked",
		},
		{
			name:        "super_blocked",
			code:        "super().__init__()",
			shouldError: true,
			description: "super() function should be blocked",
		},
		{
			name:        "obfuscated_import_listcomp",
			code:        "[__import__('os')]",
			shouldError: true,
			description: "obfuscated import in list comprehension should be blocked",
		},
		{
			name:        "obfuscated_import_parens",
			code:        "(__import__('os'))",
			shouldError: true,
			description: "obfuscated import in parentheses should be blocked",
		},
		{
			name:        "lambda_with_import",
			code:        "f = lambda: __import__('os')",
			shouldError: true,
			description: "lambda with __import__ should be blocked",
		},
		{
			name:        "lambda_with_exec",
			code:        "f = lambda: exec('print(1)')",
			shouldError: true,
			description: "lambda with exec should be blocked",
		},
		{
			name:        "pickle_import_blocked",
			code:        "import pickle",
			shouldError: true,
			description: "pickle module import should be blocked",
		},
		{
			name:        "marshal_import_blocked",
			code:        "import marshal",
			shouldError: true,
			description: "marshal module import should be blocked",
		},
		{
			name:        "yaml_import_blocked",
			code:        "import yaml",
			shouldError: true,
			description: "yaml module import should be blocked",
		},
		{
			name:        "shelve_import_blocked",
			code:        "import shelve",
			shouldError: true,
			description: "shelve module import should be blocked",
		},
		{
			name:        "base64_import_blocked",
			code:        "import base64",
			shouldError: true,
			description: "base64 module import should be blocked",
		},
		{
			name:        "b64decode_blocked",
			code:        "base64.b64decode('test')",
			shouldError: true,
			description: "base64.b64decode should be blocked",
		},
		{
			name:        "escape_sequences_blocked",
			code:        "x = '\\x48\\x65\\x6c\\x6c\\x6f'",
			shouldError: true,
			description: "escape sequences should be blocked",
		},
		{
			name:        "unicode_escape_blocked",
			code:        "x = '\\u0048\\u0065\\u006c\\u006c\\u006f'",
			shouldError: true,
			description: "unicode escape sequences should be blocked",
		},
		{
			name:        "with_open_blocked",
			code:        "with open('/etc/passwd') as f:\n    print(f.read())",
			shouldError: true,
			description: "with open() should be blocked",
		},
		// Valid code that should pass
		{
			name:        "simple_arithmetic",
			code:        "x = 1 + 2\nprint(x)",
			shouldError: false,
			description: "Simple arithmetic should pass",
		},
		{
			name:        "for_loop",
			code:        "for i in range(10):\n    print(i)",
			shouldError: false,
			description: "For loop should pass",
		},
		{
			name:        "list_comprehension",
			code:        "x = [i*2 for i in range(10)]\nprint(x)",
			shouldError: false,
			description: "List comprehension should pass",
		},
		{
			name:        "function_definition",
			code:        "def add(a, b):\n    return a + b\nprint(add(1, 2))",
			shouldError: false,
			description: "Function definition should pass",
		},
		{
			name:        "class_definition",
			code:        "class Counter:\n    def __init__(self):\n        self.count = 0\nc = Counter()\nprint(c.count)",
			shouldError: false,
			description: "Class definition should pass",
		},
		{
			name:        "dictionary_operations",
			code:        "d = {'a': 1, 'b': 2}\nprint(d['a'])",
			shouldError: false,
			description: "Dictionary operations should pass",
		},
		{
			name:        "string_operations",
			code:        "s = 'Hello, World!'\nprint(s.upper())",
			shouldError: false,
			description: "String operations should pass",
		},
		{
			name:        "f_string",
			code:        "x = 42\nprint(f'The answer is {x}')",
			shouldError: false,
			description: "F-strings should pass",
		},
		{
			name:        "lambda_safe",
			code:        "f = lambda x: x * 2\nprint(f(5))",
			shouldError: false,
			description: "Safe lambda should pass",
		},
		{
			name:        "try_except",
			code:        "try:\n    x = 1 / 0\nexcept ZeroDivisionError:\n    print('Cannot divide by zero')",
			shouldError: false,
			description: "Try-except block should pass",
		},
		{
			name:        "if_statement",
			code:        "x = 5\nif x > 0:\n    print('positive')",
			shouldError: false,
			description: "If statement should pass",
		},
		{
			name:        "math_import_safe",
			code:        "import math\nprint(math.sqrt(16))",
			shouldError: false,
			description: "math module is safe and commonly used for calculations",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validatePythonCode(context.Background(), tc.code)
			if tc.shouldError && err == nil {
				t.Errorf("%s: expected error for dangerous code, but got none (description: %s)", tc.name, tc.description)
			}
			if !tc.shouldError && err != nil {
				t.Errorf("%s: unexpected error for valid code: %v (description: %s)", tc.name, err, tc.description)
			}
		})
	}
}

// TestDeadlockPrevention verifies that validatePythonCode doesn't deadlock
// when the Python process times out or exits early (regression test for deadlock bug)
func TestDeadlockPrevention(t *testing.T) {
	// This test verifies that validatePythonCode returns within a reasonable time
	// even when the Python process might exit early due to timeout or error.
	// The deadlock occurred when CombinedOutput() blocked waiting for process completion,
	// while the write goroutine blocked on stdin to a process that already exited.

	testCode := `
# This code will cause Python to process, but the key test is that
# validatePythonCode returns even if Python exits early/times out
print("test result")
`

	// Call validatePythonCode - with the fix, it should return immediately
	// without deadlock regardless of Python's exit state
	err := validatePythonCode(context.Background(), testCode)

	// We don't assert a specific result because Python might not be available
	// The important thing is that it returns at all (no deadlock)
	// If there was a deadlock, this test would hang indefinitely
	if err != nil {
		// Error is acceptable (Python might not be available, timeout, etc.)
		// The fix is verified by the fact that we get here without hanging
		t.Logf("validatePythonCode returned (with error): %v", err)
	} else {
		t.Logf("validatePythonCode returned successfully (no error)")
	}
}

// TestMultipleRapidValidations verifies that multiple rapid calls don't cause
// issues or accumulate goroutines (potential leak scenario)
func TestMultipleRapidValidations(t *testing.T) {
	// Call validatePythonCode multiple times in rapid succession
	// If there's a goroutine leak or deadlock, this will eventually fail
	for i := 0; i < 10; i++ {
		code := fmt.Sprintf("x = %d\nprint(x)", i)
		validatePythonCode(context.Background(), code)
	}
	// If we get here without hanging or resource exhaustion, the fix is working
	t.Logf("Successfully completed 10 rapid validation calls")
}
