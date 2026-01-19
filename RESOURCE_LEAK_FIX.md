# Resource Leak Fix: HTTP Response Bodies in Retry Logic

## Issue Description

**Bug**: Resource leak in `OpenAIProvider.Chat()` method when HTTP request retry is attempted.

**File**: `provider.go`
**Method**: `OpenAIProvider.Chat()` (lines 228-339)

## Root Cause

The retry logic used `defer resp.Body.Close()` to close the HTTP response body. However, in Go, `defer` statements execute when the **function** returns, not when a loop iteration ends with `continue`.

### Original Code (Buggy)

```go
var lastErr error
for attempt := 0; attempt < 3; attempt++ {
    // ... request creation ...
    
    resp, err := p.client.Do(req)
    if err != nil {
        lastErr = err
        // Retry on connection reset or timeout
        if isTransientError(err) {
            continue  // ← BUG: defer NOT executed here!
        }
        return "", fmt.Errorf("request failed: %w", err)
    }
    defer resp.Body.Close()  // ← Only executed when Chat() function returns
    
    body, err := io.ReadAll(resp.Body)
    if err != nil {
        lastErr = err
        continue  // ← BUG: defer NOT executed here either!
    }
    
    if resp.StatusCode != http.StatusOK {
        if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
            lastErr = fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
            continue  // ← BUG: defer NOT executed!
        }
        return "", fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
    }
    
    // ... success case ...
    return chatResp.Choices[0].Message.Content, nil
}
return "", fmt.Errorf("request failed after retries: %w", lastErr)
```

### Why This Causes a Leak

1. When `continue` is executed (lines 278, 292, 300 in original code), the loop proceeds to the next iteration
2. The `defer resp.Body.Close()` statement does NOT execute when `continue` is called
3. The HTTP response body from the previous iteration remains **unclosed**
4. Each unclosed response body consumes a file descriptor and HTTP connection
5. Under retry scenarios, multiple file descriptors are leaked simultaneously
6. Eventually, this leads to "too many open files" errors

### Impact

- **Severity**: High (resource leak)
- **Frequency**: Occurs whenever HTTP request fails and retry is attempted
- **Consequences**: 
  - File descriptor exhaustion
  - Connection pool exhaustion
  - "Too many open files" errors under load
  - Memory pressure from unclosed connections

## Solution

Replace `defer` with explicit `resp.Body.Close()` calls before each `continue` and `return` statement.

### Fixed Code

```go
var lastErr error
for attempt := 0; attempt < 3; attempt++ {
    // ... request creation ...
    
    resp, err := p.client.Do(req)
    if err != nil {
        lastErr = err
        // Retry on connection reset or timeout
        if isTransientError(err) {
            // Close connection before retrying
            if resp != nil {
                resp.Body.Close()  // ← Explicit close before continue
            }
            continue
        }
        return "", fmt.Errorf("request failed: %w", err)
    }
    // defer removed - see inline closes
    
    body, err := io.ReadAll(resp.Body)
    if err != nil {
        lastErr = err
        // Close response body before retrying
        resp.Body.Close()  // ← Explicit close before continue
        continue
    }
    
    if resp.StatusCode != http.StatusOK {
        if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
            lastErr = fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
            // Close response body before retrying
            resp.Body.Close()  // ← Explicit close before continue
            continue
        }
        // Close response body before returning error
        resp.Body.Close()  // ← Explicit close before return
        return "", fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
    }
    
    // ... success case ...
    // Close response body on success path
    resp.Body.Close()  // ← Explicit close before return
    return chatResp.Choices[0].Message.Content, nil
}

// Note: response body is already closed above on all paths
return "", fmt.Errorf("request failed after retries: %w", lastErr)
```

## Changes Made

1. **Removed** `defer resp.Body.Close()` statement (line 282 in original)
2. **Added** explicit `resp.Body.Close()` before each `continue`:
   - Line 279-281: After `isTransientError()` check
   - Line 290-291: After `io.ReadAll()` error
   - Line 298-299: After status code retry (429/5xx)
3. **Added** explicit `resp.Body.Close()` before each error return:
   - Line 303: Before status error return
   - Line 323-324: Before API error return
4. **Added** explicit `resp.Body.Close()` before success return:
   - Line 332-333: Before success return
5. **Added** comment explaining that response body is closed on all paths

## Verification

### Build Test
```bash
go build -o /tmp/reasoning-tools-check .
# Result: No compilation errors
```

### Unit Test
```bash
go test ./...
# Result: All tests pass
# ok  	reasoning-tools	1.231s
# ok  	reasoning-tools/utils	(cached)
```

### Manual Review
- ✅ All code paths that create `resp` also close it
- ✅ No `defer` statements left that could execute at wrong time
- ✅ Response body is closed before `continue` in all retry cases
- ✅ Response body is closed before `return` in all error cases
- ✅ Response body is closed before success return
- ✅ All error returns still maintain proper error wrapping

## Affected Code Paths

1. **Network error retry**: When `p.client.Do()` returns a transient error (e.g., connection reset)
2. **Read error retry**: When `io.ReadAll()` fails to read response body
3. **Rate limit retry**: When server returns 429 Too Many Requests
4. **Server error retry**: When server returns 5xx status code
5. **Error returns**: Non-retryable errors before returning
6. **Success return**: Successful response before returning result

## Why Use Explicit Close Instead of Defer?

In retry loops with `continue` statements, `defer` is problematic because:

- `defer` executes when the **entire function** returns
- `continue` does **not** trigger `defer` execution
- Response body must be closed **before** retrying to prevent leak
- Explicit `resp.Body.Close()` ensures proper timing

## Lessons Learned

1. **Never use defer in retry loops** with `continue` statements
2. **Explicitly close resources** before any early exit (`continue`, `break`, `return`)
3. **Test resource cleanup** under failure/retry scenarios
4. **Consider file descriptor limits** when writing retry logic
5. **Use tools like linters** to detect potential resource leaks (e.g., `staticcheck`, `errcheck`)

## Related Resources

- [Go defer mechanics](https://go.dev/tour/flowcontrol/12)
- [Resource leak patterns in Go](https://github.com/golang/go/wiki/CommonMistakes#using-defer-to-close-a-file)
- [HTTP client best practices](https://pkg.go.dev/net/http#Client)

---

**Fix Applied**: 2026-01-19  
**Status**: ✅ Fixed and verified  
**Test Status**: ✅ All tests passing
