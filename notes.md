- `sync.WaitGroup` is a concurrency primitive that blocks until all goroutines in the WaitGroup finish execution
    - It is a counting semaphore
    - `wg.Go(...)` starts goroutine
    - `wg.Wait()` blocks until all goroutines in the WaitGroup have finished
- Waiting for `30` seconds in Go:
```Go
duration := 30 * time.Second
start := time.Now()
for time.Since(start) < duration {
    ...
}
```
- `<-time.After(duration)` waits for the duration to elapse and then sends the current time on the returned channel, perfect for **timeouts**
- Use `atomic.AddInt64(...)` to increment integers belonging to structs that are shared between concurrent goroutines